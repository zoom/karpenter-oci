/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instancetype

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	ocicache "github.com/zoom/karpenter-oci/pkg/cache"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"github.com/zoom/karpenter-oci/pkg/providers/pricing"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"strconv"
	"strings"
	"sync"
)

const (
	InstanceTypesCacheKey = "types"
)

type Provider struct {
	region               string
	compClient           api.ComputeClient
	mu                   sync.Mutex
	cache                *cache.Cache
	unavailableOfferings *ocicache.UnavailableOfferings
	priceSyncer          *pricing.PriceListSyncer
}

func NewProvider(region string, compClient api.ComputeClient, cache *cache.Cache, unavailableOfferings *ocicache.UnavailableOfferings, priceSyncer *pricing.PriceListSyncer) *Provider {
	return &Provider{region: region, compClient: compClient, cache: cache, unavailableOfferings: unavailableOfferings, priceSyncer: priceSyncer}
}

func (p *Provider) List(ctx context.Context, kc *v1alpha1.KubeletConfiguration, nodeClass *v1alpha1.OciNodeClass) ([]*cloudprovider.InstanceType, error) {

	wrapShapes, err := p.ListInstanceType(ctx)
	if err != nil {
		return nil, err
	}
	instanceTypes := make([]*cloudprovider.InstanceType, 0)
	for _, wrapped := range wrapShapes {
		// todo offers
		instanceTypes = append(instanceTypes, NewInstanceType(ctx, wrapped, nodeClass, kc, p.region, wrapped.AvailableDomains, p.CreateOfferings(wrapped, sets.New(wrapped.AvailableDomains...))))
	}
	return instanceTypes, nil

}

func (p *Provider) CreateOfferings(shape *internalmodel.WrapShape, zones sets.Set[string]) []cloudprovider.Offering {
	var offerings []cloudprovider.Offering

	var priceCatalog *pricing.PriceCatalog
	if p.priceSyncer != nil {
		priceCatalog = &p.priceSyncer.PriceCatalog
	}

	// only on-demand support
	for zone := range zones {
		// exclude any offerings that have recently seen an insufficient capacity error
		isUnavailable := p.unavailableOfferings.IsUnavailable(*shape.Shape.Shape, zone, v1.CapacityTypeOnDemand) // todo support pricing calculate

		price := float64(pricing.Calculate(shape, priceCatalog))
		offerings = append(offerings, cloudprovider.Offering{
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
			),
			Price:     price,
			Available: !isUnavailable,
		})
		// metric
		instanceTypeOfferingAvailable.With(prometheus.Labels{
			instanceTypeLabel: *shape.Shape.Shape,
			capacityTypeLabel: v1.CapacityTypeOnDemand,
			zoneLabel:         zone,
		}).Set(float64(lo.Ternary(!isUnavailable, 1, 0)))

		instanceTypeOfferingPriceEstimate.With(prometheus.Labels{
			instanceTypeLabel: fmt.Sprintf("%s_%d_%d", *shape.Shape.Shape, shape.CalcCpu/2, shape.CalMemInGBs),
			capacityTypeLabel: v1.CapacityTypeOnDemand,
			zoneLabel:         zone,
		}).Set(price)
	}
	return offerings
}

func (p *Provider) ListInstanceType(ctx context.Context) (map[string]*internalmodel.WrapShape, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cached, ok := p.cache.Get(InstanceTypesCacheKey); ok {
		return cached.(map[string]*internalmodel.WrapShape), nil
	}
	adShapesMap := make(map[string][]*internalmodel.WrapShape, 0)
	for _, availableDomain := range options.FromContext(ctx).AvailableDomains {
		shapes := make([]core.Shape, 0)
		nextPage := "0"
		for nextPage != "" {
			if nextPage == "0" {
				nextPage = ""
			}
			req := core.ListShapesRequest{
				Limit:              common.Int(50),
				Page:               common.String(nextPage),
				AvailabilityDomain: common.String(availableDomain),
				CompartmentId:      common.String(options.FromContext(ctx).CompartmentId)}

			// Send the request using the service client
			resp, err := p.compClient.ListShapes(ctx, req)
			if err != nil {
				return nil, err
			}
			shapes = append(shapes, resp.Items...)
			if resp.OpcNextPage != nil {
				nextPage = *resp.OpcNextPage
			} else {
				nextPage = ""
			}
		}

		ad := strings.Split(availableDomain, ":")[1]

		if old, ok := adShapesMap[ad]; ok {
			adShapesMap[ad] = append(old, toWrapShape(ctx, shapes, ad)...)
		} else {
			adShapesMap[ad] = toWrapShape(ctx, shapes, ad)
		}

	}
	// combine zones
	wrapShapes := make(map[string]*internalmodel.WrapShape, 0)
	for ad, shapes := range adShapesMap {
		for _, shape := range shapes {
			// metric
			instanceTypeVCPU.With(prometheus.Labels{instanceTypeLabel: *shape.Shape.Shape}).Set(float64(shape.CalcCpu))
			instanceTypeMemory.With(prometheus.Labels{instanceTypeLabel: *shape.Shape.Shape}).Set(float64(lo.FromPtr(shape.MemoryInGBs)) * 1024 * 1024 * 1024)

			if wrapped, ok := wrapShapes[fmt.Sprintf("%s-%d-%d", *shape.Shape.Shape, shape.CalcCpu, shape.CalMemInGBs)]; !ok {
				wrapShapes[fmt.Sprintf("%s-%d-%d", *shape.Shape.Shape, shape.CalcCpu, shape.CalMemInGBs)] = shape
			} else {
				wrapped.AvailableDomains = append(wrapped.AvailableDomains, ad)
			}
		}
	}

	p.cache.SetDefault(InstanceTypesCacheKey, wrapShapes)
	return wrapShapes, nil
}

func toWrapShape(ctx context.Context, shapes []core.Shape, ad string) []*internalmodel.WrapShape {
	wrapShapes := make([]*internalmodel.WrapShape, 0)
	for _, shape := range shapes {
		if *shape.IsFlexible {
			wrapShapes = append(wrapShapes, splitFlexCpuMem(ctx, shape, ad)...)
		} else {
			wrapShapes = append(wrapShapes, &internalmodel.WrapShape{
				Shape: shape,
				// ocpus is twice vcpu
				CalcCpu:          int64(*shape.Ocpus) * 2,
				CalMemInGBs:      int64(*shape.MemoryInGBs),
				AvailableDomains: []string{ad},
				CalMaxVnic:       int64(*shape.MaxVnicAttachments),
			})
		}
	}
	return wrapShapes
}

func splitFlexCpuMem(ctx context.Context, shape core.Shape, ad string) []*internalmodel.WrapShape {
	flexCpuMemRatios := strings.Split(options.FromContext(ctx).FlexCpuMemRatios, ",")
	constrainCpus := strings.Split(options.FromContext(ctx).FlexCpuConstrainList, ",")
	wrapShapes := make([]*internalmodel.WrapShape, 0)
	for i := 0; i < len(constrainCpus); i++ {
		for _, ratio := range flexCpuMemRatios {
			ratioInt, covErr := strconv.Atoi(ratio)
			if covErr != nil {
				continue
			}
			cpus, covErr := strconv.Atoi(constrainCpus[i])
			if covErr != nil {
				continue
			}
			memInGBs := cpus * 2 * ratioInt
			if cpus < int(*shape.OcpuOptions.Min) || memInGBs < int(*shape.MemoryOptions.MinInGBs) {
				continue
			}
			if cpus > int(*shape.OcpuOptions.Max) || memInGBs > int(*shape.MemoryOptions.MaxInGBs) {
				continue
			}
			var calMaxVnic int64
			// https://docs.oracle.com/en-us/iaas/Content/Compute/References/computeshapes.htm
			if shape.MaxVnicAttachmentOptions != nil && shape.MaxVnicAttachmentOptions.DefaultPerOcpu != nil {
				if cpus == 1 {
					calMaxVnic = 2
				} else {
					calMaxVnic = int64(*shape.MaxVnicAttachmentOptions.DefaultPerOcpu) * int64(cpus)
				}
				calMaxVnic = min(24, calMaxVnic)
			} else {
				calMaxVnic = int64(*shape.MaxVnicAttachments)
			}
			wrapShapes = append(wrapShapes, &internalmodel.WrapShape{
				Shape:            shape,
				CalcCpu:          int64(cpus) * 2,
				CalMemInGBs:      int64(memInGBs),
				AvailableDomains: []string{ad},
				CalMaxVnic:       calMaxVnic,
			})
		}
	}
	return wrapShapes
}
