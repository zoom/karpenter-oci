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

package imagefamily

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"strings"
	"sync"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/patrickmn/go-cache"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
)

type Provider struct {
	sync.Mutex
	cache  *cache.Cache
	client api.ComputeClient
}

func NewProvider(client api.ComputeClient, cache *cache.Cache) *Provider {
	return &Provider{
		client: client,
		cache:  cache,
	}
}

func (p *Provider) List(ctx context.Context, nodeclass *v1alpha1.OciNodeClass) ([]internalmodel.WrapImage, error) {
	hash, err := hashstructure.Hash(nodeclass.Spec.ImageSelector, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return nil, err
	}

	p.Lock()
	defer p.Unlock()

	if images, ok := p.cache.Get(fmt.Sprintf("%d", hash)); ok {
		// shallow-copy of the slice
		return append([]internalmodel.WrapImage{}, images.([]internalmodel.WrapImage)...), nil
	}
	images := make(map[string]internalmodel.WrapImage, 0)
	for _, selector := range nodeclass.Spec.ImageSelector {
		if selector.Id == "" {
			req := core.ListImagesRequest{
				CompartmentId:  common.String(selector.CompartmentId),
				DisplayName:    common.String(selector.Name),
				LifecycleState: core.ImageLifecycleStateAvailable}

			// Send the request using the service client
			resp, err := p.client.ListImages(ctx, req)
			if err != nil {
				return nil, err
			}
			for _, img := range resp.Items {
				images[lo.FromPtr(img.Id)] = internalmodel.WrapImage{Image: img, Requirements: requirementsForImage(nodeclass.Spec.ImageFamily, img)}
			}
		} else {
			req := core.GetImageRequest{
				ImageId: common.String(selector.Id),
			}
			resp, err := p.client.GetImage(ctx, req)
			if err != nil {
				return nil, err
			}
			images[lo.FromPtr(resp.Id)] = internalmodel.WrapImage{Image: resp.Image, Requirements: requirementsForImage(nodeclass.Spec.ImageFamily, resp.Image)}
		}
	}
	p.cache.SetDefault(fmt.Sprintf("%d", hash), lo.Values(images))
	return lo.Values(images), nil
}

// parse the image arch info from name
// gpu Oracle-Linux-8.10-Gen2-GPU-2025.05.19-0-OKE-1.31.1-764
// arm64 Oracle-Linux-8.10-aarch64-2025.05.19-0-OKE-1.31.1-764
// x86 Oracle-Linux-8.10-2025.05.19-0-OKE-1.31.1-764
func requirementsForImage(imageFamily string, image core.Image) scheduling.Requirements {
	if imageFamily != v1alpha1.OracleOKELinuxImageFamily {
		return scheduling.NewRequirements()
	}
	arch := karpv1.ArchitectureAmd64
	if strings.Contains(strings.ToLower(lo.FromPtr(image.DisplayName)), "aarch64") {
		arch = karpv1.ArchitectureAmd64
	}
	requires := scheduling.NewRequirements(scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, arch))
	if strings.Contains(strings.ToLower(lo.FromPtr(image.DisplayName)), "gpu") {
		requires.Add(scheduling.NewRequirement(v1alpha1.LabelInstanceGPU, corev1.NodeSelectorOpExists))
	}
	return requires
}
