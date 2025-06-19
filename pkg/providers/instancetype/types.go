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
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"github.com/zoom/karpenter-oci/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/ptr"
	"math"
	corev1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
	"strconv"
	"strings"
)

const (
	Gi              = 1024 * 1024 * 1024
	MemoryAvailable = "memory.available"
	NodeFSAvailable = "nodefs.available"
)

// https://docs.oracle.com/en-us/iaas/Content/Compute/References/arm.htm
var ArmShapes = []string{"BM.Standard.A1.160", "VM.Standard.A1.Flex", "VM.Standard.A2.Flex"}

// TaxBrackets implements a simple bracketed tax structure.
type TaxBrackets []struct {
	// UpperBound is the largest value this bracket is applied to.
	// The first bracket's lower bound is always 0.
	UpperBound float64

	LowerBound float64

	Recommended float64

	// Rate is the percent rate of tax expressed as a float i.e. .5 for 50%.
	Rate float64
}

var (
	// reservedMemoryTaxGi denotes the tax brackets for memory in Gi.
	reservedMemoryTaxGi = TaxBrackets{
		{
			LowerBound:  2,
			Recommended: 1,
		},
		{
			LowerBound:  4,
			Recommended: 1,
		},
		{
			LowerBound:  8,
			Recommended: 1,
		},
		{
			LowerBound:  16,
			Recommended: 2,
		},
		{
			LowerBound:  128,
			Recommended: 9,
			Rate:        .02,
		},
	}

	//reservedCPUTaxVCPU denotes the tax brackets for Virtual CPU cores.
	reservedCPUTaxVCPU = TaxBrackets{
		{
			LowerBound:  1,
			Recommended: 0.06,
		},
		{
			LowerBound:  2,
			Recommended: 0.07,
		},
		{
			LowerBound:  3,
			Recommended: 0.08,
		},
		{
			LowerBound:  4,
			Recommended: 0.085,
		},
		{
			LowerBound:  5,
			Recommended: 0.09,
			Rate:        .0025,
		},
	}
)

// Calculate expects Memory in Gi and CPU in cores.
func (t TaxBrackets) Calculate(amount float64) float64 {
	var tax float64

	for _, bracket := range t {
		if bracket.LowerBound > amount {
			break
		}
		tax = bracket.Recommended + bracket.Rate*(amount-bracket.LowerBound)
	}

	return tax
}

func NewInstanceType(ctx context.Context, shape *internalmodel.WrapShape, nodeClass *v1alpha1.OciNodeClass,
	region string, zones []string, offerings cloudprovider.Offerings) *cloudprovider.InstanceType {
	kc := &v1alpha1.KubeletConfiguration{}
	if nodeClass.Spec.Kubelet != nil {
		kc = nodeClass.Spec.Kubelet
	}
	return &cloudprovider.InstanceType{
		Name:         *shape.Shape.Shape,
		Requirements: computeRequirements(ctx, shape, offerings, zones, region),
		Offerings:    offerings,
		Capacity:     computeCapacity(ctx, shape, kc, nodeClass),
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      KubeReservedResources(kc, cpu(shape.CalcCpu), resources.Quantity(fmt.Sprintf("%dGi", shape.CalMemInGBs))),
			SystemReserved:    SystemReservedResources(kc),
			EvictionThreshold: EvictionThreshold(resources.Quantity(fmt.Sprintf("%dGi", shape.CalMemInGBs)), resources.Quantity(fmt.Sprintf("%dGi", nodeClass.Spec.BootConfig.BootVolumeSizeInGBs)), kc),
		},
	}
}

func computeRequirements(ctx context.Context, shape *internalmodel.WrapShape, offerings cloudprovider.Offerings, zones []string, region string) scheduling.Requirements {
	arch := "amd64"
	if lo.Contains(ArmShapes, *shape.Shape.Shape) {
		arch = "arm64"
	}
	requirements := scheduling.NewRequirements(
		// Well Known Upstream
		scheduling.NewRequirement(v1.LabelInstanceTypeStable, v1.NodeSelectorOpIn, *shape.Shape.Shape),
		scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpIn, arch),
		scheduling.NewRequirement(v1.LabelOSStable, v1.NodeSelectorOpIn, string(v1.Linux)),
		//scheduling.NewRequirement(v1.LabelTopologyZone, v1.NodeSelectorOpIn, lo.Map(offerings.Available(), func(o cloudprovider.Offering, _ int) string { return o.Zone })...),
		scheduling.NewRequirement(v1.LabelTopologyZone, v1.NodeSelectorOpIn, zones...),
		scheduling.NewRequirement(v1.LabelTopologyRegion, v1.NodeSelectorOpIn, region),
		// Well Known to Karpenter
		scheduling.NewRequirement(corev1.CapacityTypeLabelKey, v1.NodeSelectorOpIn, lo.Map(offerings.Available(), func(o *cloudprovider.Offering, _ int) string {
			return o.Requirements.Get(corev1.CapacityTypeLabelKey).Any()
		})...),
		// Well Known to OCI
		scheduling.NewRequirement(v1alpha1.LabelInstanceShapeName, v1.NodeSelectorOpIn, *shape.Shape.Shape),
		scheduling.NewRequirement(v1alpha1.LabelInstanceCPU, v1.NodeSelectorOpIn, fmt.Sprint(shape.CalcCpu)),
		scheduling.NewRequirement(v1alpha1.LabelIsFlexible, v1.NodeSelectorOpIn, fmt.Sprint(lo.FromPtr(shape.IsFlexible))),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPU, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceGPUDescription, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceMemory, v1.NodeSelectorOpIn, fmt.Sprint(shape.CalMemInGBs*1024)),
		scheduling.NewRequirement(v1alpha1.LabelInstanceNetworkBandwidth, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelInstanceMaxVNICs, v1.NodeSelectorOpDoesNotExist),
	)
	// insert actual value if exist
	if shape.NetworkingBandwidthInGbps != nil {
		requirements[v1alpha1.LabelInstanceNetworkBandwidth].Insert(fmt.Sprint(shape.CalMaxBandwidthInGbps * 1024))
	}
	if shape.MaxVnicAttachments != nil {
		requirements[v1alpha1.LabelInstanceMaxVNICs].Insert(fmt.Sprint(shape.CalMaxVnic))
	}
	if shape.Gpus != nil && lo.FromPtr(shape.Gpus) != 0 {
		requirements[v1alpha1.LabelInstanceGPU].Insert(fmt.Sprint(nvidiaGPUs(shape.Shape).Value()))
		qualifiedDesc := utils.SanitizeLabelValue(lo.FromPtr(shape.GpuDescription))
		requirements[v1alpha1.LabelInstanceGPUDescription].Insert(qualifiedDesc)
	}

	return requirements
}

func computeCapacity(ctx context.Context, shape *internalmodel.WrapShape, kc *v1alpha1.KubeletConfiguration, nodeclass *v1alpha1.OciNodeClass) v1.ResourceList {

	resourceList := v1.ResourceList{
		v1.ResourceCPU:                    *cpu(shape.CalcCpu),
		v1.ResourceMemory:                 *memory(ctx, shape.CalMemInGBs),
		v1.ResourceEphemeralStorage:       *ephemeralStorage(nodeclass),
		v1.ResourcePods:                   *pods(shape, kc),
		v1.ResourceName("nvidia.com/gpu"): *nvidiaGPUs(shape.Shape),
	}
	return resourceList
}

func cpu(cpu int64) *resource.Quantity {
	return resources.Quantity(fmt.Sprint(cpu))
}

func memory(ctx context.Context, memoryInGBs int64) *resource.Quantity {
	mem := resources.Quantity(fmt.Sprintf("%dGi", memoryInGBs))
	// Account for VM overhead in calculation
	mem.Sub(resource.MustParse(fmt.Sprintf("%dMi", int64(math.Ceil(float64(mem.Value())*options.FromContext(ctx).VMMemoryOverheadPercent/1024/1024)))))
	return mem
}

// Setting ephemeral-storage to be either the default value, what is defined in blockDeviceMappings, or the combined size of local store volumes.
func ephemeralStorage(nodeclass *v1alpha1.OciNodeClass) *resource.Quantity {
	return resources.Quantity(fmt.Sprintf("%dGi", nodeclass.Spec.BootConfig.BootVolumeSizeInGBs))
}

func nvidiaGPUs(shape core.Shape) *resource.Quantity {
	count := int64(0)
	if shape.Gpus != nil {
		count = int64(*shape.Gpus)
	}
	return resources.Quantity(fmt.Sprint(count))
}

// TODO fixme, we need to consider the maxVnic only when using native-cni
func pods(shape *internalmodel.WrapShape, kc *v1alpha1.KubeletConfiguration) *resource.Quantity {
	var count int64
	switch {
	case kc != nil && kc.MaxPods != nil:
		count = int64(ptr.Int32Value(kc.MaxPods))
	default:
		// The limit of 110 is imposed by Kubernetes.
		count = 110
	}
	if kc != nil && kc.PodsPerCore != nil {
		count = lo.Min([]int64{int64(ptr.Int32Value(kc.PodsPerCore)) * shape.CalcCpu, count})
	}
	// Maximum number of Pods per node = MIN( (Number of VNICs - 1) * 31 ), 110)
	return resources.Quantity(fmt.Sprint(min(count, (shape.CalMaxVnic-1)*31)))
}

func SystemReservedResources(kc *v1alpha1.KubeletConfiguration) v1.ResourceList {
	if kc != nil && kc.SystemReserved != nil {
		return lo.MapEntries(kc.SystemReserved, func(k string, v string) (v1.ResourceName, resource.Quantity) {
			return v1.ResourceName(k), resource.MustParse(v)
		})
	}
	return v1.ResourceList{
		v1.ResourceCPU:    *resource.NewScaledQuantity(100, resource.Milli),
		v1.ResourceMemory: *resource.NewQuantity(100*1024*1024, resource.BinarySI),
	}
}

// https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengbestpractices_topic-Cluster-Management-best-practices.htm#contengbestpractices_topic-Cluster-Management-best-practices__ManagingOKEClusters-Reserveresourcesforkubernetesandossystemdaemons
func KubeReservedResources(kc *v1alpha1.KubeletConfiguration, cpu *resource.Quantity, memory *resource.Quantity) v1.ResourceList {
	if kc != nil && len(kc.KubeReserved) != 0 {
		return lo.MapEntries(kc.KubeReserved, func(k string, v string) (v1.ResourceName, resource.Quantity) {
			return v1.ResourceName(k), resource.MustParse(v)
		})
	}
	reservedMemoryMi := int64(1024 * reservedMemoryTaxGi.Calculate(float64(memory.Value()/Gi)))
	reservedCPUMilli := int64(1000 * reservedCPUTaxVCPU.Calculate(float64(cpu.Value())))

	resourceList := v1.ResourceList{
		v1.ResourceCPU:              *resource.NewScaledQuantity(reservedCPUMilli, resource.Milli),
		v1.ResourceMemory:           *resource.NewQuantity(reservedMemoryMi*1024*1024, resource.BinarySI),
		v1.ResourceEphemeralStorage: resource.MustParse("1Gi"), // default kube-reserved ephemeral-storage
	}

	return resourceList
}

func EvictionThreshold(memory *resource.Quantity, storage *resource.Quantity, kc *v1alpha1.KubeletConfiguration) v1.ResourceList {
	overhead := v1.ResourceList{
		v1.ResourceMemory:           resource.MustParse("100Mi"),
		v1.ResourceEphemeralStorage: resource.MustParse(fmt.Sprint(math.Ceil(float64(storage.Value()) / 100 * 10))),
	}

	override := v1.ResourceList{}
	var evictionSignals []map[string]string
	if kc != nil && kc.EvictionHard != nil {
		evictionSignals = append(evictionSignals, kc.EvictionHard)
	}

	for _, m := range evictionSignals {
		temp := v1.ResourceList{}
		if v, ok := m[MemoryAvailable]; ok {
			temp[v1.ResourceMemory] = computeEvictionSignal(*memory, v)
		}
		if v, ok := m[NodeFSAvailable]; ok {
			temp[v1.ResourceEphemeralStorage] = computeEvictionSignal(*storage, v)
		}
		override = resources.MaxResources(override, temp)
	}
	// Assign merges maps from left to right so overrides will always be taken last
	return lo.Assign(overhead, override)
}

// computeEvictionSignal computes the resource quantity value for an eviction signal value, computed off the
// base capacity value if the signal value is a percentage or as a resource quantity if the signal value isn't a percentage
func computeEvictionSignal(capacity resource.Quantity, signalValue string) resource.Quantity {
	if strings.HasSuffix(signalValue, "%") {
		p := mustParsePercentage(signalValue)

		// Calculation is node.capacity * signalValue if percentage
		// From https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/#eviction-signals
		return resource.MustParse(fmt.Sprint(math.Ceil(capacity.AsApproximateFloat64() / 100 * p)))
	}
	return resource.MustParse(signalValue)
}

func mustParsePercentage(v string) float64 {
	p, err := strconv.ParseFloat(strings.Trim(v, "%"), 64)
	if err != nil {
		panic(fmt.Sprintf("expected percentage value to be a float but got %s, %v", v, err))
	}
	// Setting percentage value to 100% is considered disabling the threshold according to
	// https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/
	if p == 100 {
		p = 0
	}
	return p
}
