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

package instancetype_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/awslabs/operatorpkg/status"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/fake"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/instancetype"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"github.com/zoom/karpenter-oci/pkg/test"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"
	"knative.dev/pkg/ptr"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"sort"
	"testing"

	"sigs.k8s.io/karpenter/pkg/controllers/provisioning"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	"sigs.k8s.io/karpenter/pkg/events"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var env *coretest.Environment
var ociEnv *test.Environment
var fakeClock *clock.FakeClock
var prov *provisioning.Provisioner
var cluster *state.Cluster
var cloudProvider *cloudprovider.CloudProvider

func TestInstanceType(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "InstanceTypeProvider")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...), coretest.WithFieldIndexers(test.OciNodeClassFieldIndexer(ctx)))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options(test.OptionsFields{AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1", "JPqd:US-ASHBURN-AD-2", "JPqd:US-ASHBURN-AD-3"}}))
	ociEnv = test.NewEnvironment(ctx, env)
	fakeClock = &clock.FakeClock{}
	cloudProvider = cloudprovider.New(ociEnv.InstanceTypesProvider, ociEnv.InstanceProvider, events.NewRecorder(&record.FakeRecorder{}),
		env.Client, ociEnv.AMIProvider)
	cluster = state.NewCluster(fakeClock, env.Client)
	prov = provisioning.NewProvisioner(env.Client, events.NewRecorder(&record.FakeRecorder{}), cloudProvider, cluster, fakeClock)
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options(test.OptionsFields{AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1", "JPqd:US-ASHBURN-AD-2", "JPqd:US-ASHBURN-AD-3"}}))
	cluster.Reset()
	ociEnv.Reset()
	ociEnv.LaunchTemplateProvider.ClusterEndpoint = "https://test-cluster"
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("InstanceTypeProvider", func() {
	var nodeClass *v1alpha1.OciNodeClass
	var nodePool *karpv1.NodePool
	BeforeEach(func() {
		nodeClass = test.OciNodeClass()
		nodeClass.StatusConditions().SetTrue(status.ConditionReady)
		nodePool = coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Template: karpv1.NodeClaimTemplate{
					Spec: karpv1.NodeClaimTemplateSpec{
						Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
							{
								NodeSelectorRequirement: v1.NodeSelectorRequirement{
									Key:      karpv1.CapacityTypeLabelKey,
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{karpv1.CapacityTypeOnDemand},
								},
							},
						},
						NodeClassRef: &karpv1.NodeClassReference{
							Name: nodeClass.Name,
						},
					},
				},
			},
		})
	})

	It("should support individual instance type labels", func() {
		ExpectApplied(ctx, env.Client, nodePool, nodeClass)

		nodeSelector := map[string]string{
			// Well known
			karpv1.NodePoolLabelKey:         nodePool.Name,
			v1.LabelTopologyZone:            "US-ASHBURN-AD-1",
			v1.LabelTopologyRegion:          "us-ashburn-1",
			v1.LabelInstanceTypeStable:      "shape-gpu",
			v1.LabelOSStable:                "linux",
			v1.LabelArchStable:              "amd64",
			v1.LabelFailureDomainBetaZone:   "US-ASHBURN-AD-1",
			v1.LabelFailureDomainBetaRegion: "us-ashburn-1",
			"beta.kubernetes.io/arch":       "amd64",
			"beta.kubernetes.io/os":         "linux",
			v1.LabelInstanceType:            "shape-gpu",

			karpv1.CapacityTypeLabelKey: "on-demand",
			// Well Known to OCI
			v1alpha1.LabelInstanceShapeName:        "shape-gpu",
			v1alpha1.LabelInstanceCPU:              "2",
			v1alpha1.LabelInstanceMemory:           "8192",
			v1alpha1.LabelInstanceNetworkBandwidth: "10",
			v1alpha1.LabelInstanceMaxVNICs:         "2",
			v1alpha1.LabelIsFlexible:               "false",
			v1alpha1.LabelInstanceGPU:              "1",
			v1alpha1.LabelInstanceGPUDescription:   "A100",
		}

		// Ensure that we're exercising all well known labels
		keys := lo.Keys(karpv1.NormalizedLabels)
		Expect(lo.Keys(nodeSelector)).To(ContainElements(append(karpv1.WellKnownLabels.Difference(sets.New(
			v1.LabelWindowsBuild)).UnsortedList(), keys...)))

		var pods []*v1.Pod
		for key, value := range nodeSelector {
			pods = append(pods, coretest.UnschedulablePod(coretest.PodOptions{NodeSelector: map[string]string{key: value}}))
		}
		ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
		for _, pod := range pods {
			ExpectScheduled(ctx, env.Client, pod)
		}
	})
	It("should order the instance types by price and only consider the cheapest ones", func() {
		instances := fake.MakeInstances()
		lo.ForEach(instances, func(item *internalmodel.WrapShape, index int) {
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(item)
		})

		ExpectApplied(ctx, env.Client, nodePool, nodeClass)
		pod := coretest.UnschedulablePod(coretest.PodOptions{
			ResourceRequirements: v1.ResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
				Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
			},
		})
		ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
		ExpectScheduled(ctx, env.Client, pod)
		its, err := cloudProvider.GetInstanceTypes(ctx, nodePool)
		Expect(err).To(BeNil())
		// Order all the instances by their price
		// We need some way to deterministically order them if their prices match
		reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodePool.Spec.Template.Spec.Requirements...)
		sort.Slice(its, func(i, j int) bool {
			iPrice := its[i].Offerings.Compatible(reqs).Cheapest().Price
			jPrice := its[j].Offerings.Compatible(reqs).Cheapest().Price
			if iPrice == jPrice {
				return its[i].Name < its[j].Name
			}
			return iPrice < jPrice
		})
		// Expect that the launch template overrides gives the 100 cheapest instance types
		expected := sets.NewString(lo.Map(its[:3], func(i *corecloudprovider.InstanceType, _ int) string {
			return i.Name
		})...)
		Expect(ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Len()).To(Equal(1))
		call := ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Pop()

		Expect(expected.Has(*call.Shape)).To(BeTrue(), fmt.Sprintf("expected %s to exist in set", *call.Shape))
	})
	It("should not launch instances w/ instance storage for ephemeral storage resource requests when exceeding blockDeviceMapping", func() {
		ExpectApplied(ctx, env.Client, nodePool, nodeClass)
		pod := coretest.UnschedulablePod(coretest.PodOptions{
			ResourceRequirements: v1.ResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceEphemeralStorage: resource.MustParse("5000Gi")},
			},
		})
		ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
		ExpectNotScheduled(ctx, env.Client, pod)
	})
	It("calculate max-pods by max MaxVnicAttachments", func() {
		instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
		Expect(err).To(BeNil())
		for _, info := range instanceInfo {
			it := instancetype.NewInstanceType(ctx,
				info,
				nodeClass,
				nodeClass.Spec.Kubelet,
				"us-ashburn-1",
				[]string{"us-east-1"},
				ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
			)
			Expect(it.Capacity.Pods().Value()).To(BeNumerically("==", 31))
		}
	})
	It("calculate max-pods by max MaxVnicAttachments for flex instance type", func() {
		ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("VM.Standard.E4.Flex"),
			IsFlexible: common.Bool(true), Ocpus: common.Float32(2), MemoryInGBs: common.Float32(8),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2),
			MaxVnicAttachmentOptions: &core.ShapeMaxVnicAttachmentOptions{
				Min:            common.Int(2),
				Max:            common.Float32(24),
				DefaultPerOcpu: common.Float32(1),
			},
			OcpuOptions: &core.ShapeOcpuOptions{
				Min: common.Float32(1),
				Max: common.Float32(16),
			},
			MemoryOptions: &core.ShapeMemoryOptions{
				MinInGBs: common.Float32(2),
				MaxInGBs: common.Float32(4096),
			},
		}})
		customMaxPod := int32(100)
		nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
			MaxPods: &customMaxPod,
		}
		instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
		Expect(err).To(BeNil())
		for _, info := range instanceInfo {
			it := instancetype.NewInstanceType(ctx,
				info,
				nodeClass,
				nodeClass.Spec.Kubelet,
				"us-ashburn-1",
				[]string{"us-east-1"},
				ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
			)
			Expect(it.Capacity.Pods().Value()).To(BeNumerically("==", min(int64(customMaxPod), (info.CalMaxVnic-1)*31)))
		}
	})
	Context("Metrics", func() {
		It("should expose vcpu metrics for instance types", func() {
			instanceTypes, err := ociEnv.InstanceTypesProvider.List(ctx, nodeClass.Spec.Kubelet, nodeClass)
			Expect(err).To(BeNil())
			Expect(len(instanceTypes)).To(BeNumerically(">", 0))
			for _, it := range instanceTypes {
				metric, ok := FindMetricWithLabelValues("karpenter_cloudprovider_instance_type_cpu_cores", map[string]string{
					"instance_type": it.Name,
				})
				Expect(ok).To(BeTrue())
				Expect(metric).To(Not(BeNil()))
				value := metric.GetGauge().Value
				Expect(*value).To(BeNumerically(">", 0))
			}
		})
		It("should expose memory metrics for instance types", func() {
			instanceTypes, err := ociEnv.InstanceTypesProvider.List(ctx, nodeClass.Spec.Kubelet, nodeClass)
			Expect(err).To(BeNil())
			Expect(len(instanceTypes)).To(BeNumerically(">", 0))
			for _, it := range instanceTypes {
				metric, ok := FindMetricWithLabelValues("karpenter_cloudprovider_instance_type_memory_bytes", map[string]string{
					"instance_type": it.Name,
				})
				Expect(ok).To(BeTrue())
				Expect(metric).To(Not(BeNil()))
				value := metric.GetGauge().Value
				Expect(*value).To(BeNumerically(">", 0))
			}
		})
		It("should expose availability metrics for instance types", func() {
			instanceTypes, err := ociEnv.InstanceTypesProvider.List(ctx, nodeClass.Spec.Kubelet, nodeClass)
			Expect(err).To(BeNil())
			Expect(len(instanceTypes)).To(BeNumerically(">", 0))
			for _, it := range instanceTypes {
				for _, of := range it.Offerings {
					metric, ok := FindMetricWithLabelValues("karpenter_cloudprovider_instance_type_offering_available", map[string]string{
						"instance_type": it.Name,
						"capacity_type": of.Requirements.Get(karpv1.CapacityTypeLabelKey).Any(),
						"zone":          of.Requirements.Get(v1.LabelTopologyZone).Any(),
					})
					Expect(ok).To(BeTrue())
					Expect(metric).To(Not(BeNil()))
					value := metric.GetGauge().Value
					Expect(*value).To(BeNumerically("==", lo.Ternary(of.Available, 1, 0)))
				}
			}
		})
	})

	Context("Overhead", func() {
		var info *internalmodel.WrapShape

		BeforeEach(func() {
			ctx = options.ToContext(ctx, test.Options(test.OptionsFields{
				ClusterName: lo.ToPtr("karpenter-cluster"), AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1"},
			}))

			var ok bool
			instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
			Expect(err).To(BeNil())
			for _, val := range instanceInfo {
				if *val.Shape.Shape == "shape-1" {
					info = val
					ok = true
				}
			}
			Expect(ok).To(BeTrue())
		})

		Context("System Reserved Resources", func() {
			It("should use defaults when no kubelet is specified", func() {
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Overhead.SystemReserved.Cpu().String()).To(Equal("100m"))
				Expect(it.Overhead.SystemReserved.Memory().String()).To(Equal("100Mi"))
			})
			It("should override system reserved cpus when specified", func() {
				nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
					SystemReserved: map[string]string{
						string(v1.ResourceCPU):              "2",
						string(v1.ResourceMemory):           "20Gi",
						string(v1.ResourceEphemeralStorage): "10Gi",
					},
				}
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Overhead.SystemReserved.Cpu().String()).To(Equal("2"))
				Expect(it.Overhead.SystemReserved.Memory().String()).To(Equal("20Gi"))
				Expect(it.Overhead.SystemReserved.StorageEphemeral().String()).To(Equal("10Gi"))
			})
		})
		Context("Kube Reserved Resources", func() {
			It("should use defaults when no kubelet is specified", func() {
				nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{}
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Overhead.KubeReserved.Cpu().String()).To(Equal("70m"))
				Expect(it.Overhead.KubeReserved.Memory().String()).To(Equal("1Gi"))
				Expect(it.Overhead.KubeReserved.StorageEphemeral().String()).To(Equal("1Gi"))
			})
			It("should override kube reserved when specified", func() {
				nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
					SystemReserved: map[string]string{
						string(v1.ResourceCPU):              "1",
						string(v1.ResourceMemory):           "20Gi",
						string(v1.ResourceEphemeralStorage): "1Gi",
					},
					KubeReserved: map[string]string{
						string(v1.ResourceCPU):              "2",
						string(v1.ResourceMemory):           "10Gi",
						string(v1.ResourceEphemeralStorage): "2Gi",
					},
				}
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Overhead.KubeReserved.Cpu().String()).To(Equal("2"))
				Expect(it.Overhead.KubeReserved.Memory().String()).To(Equal("10Gi"))
				Expect(it.Overhead.KubeReserved.StorageEphemeral().String()).To(Equal("2Gi"))
			})
		})
		Context("Eviction Thresholds", func() {
			BeforeEach(func() {
				ctx = options.ToContext(ctx, test.Options(test.OptionsFields{
					VMMemoryOverheadPercent: lo.ToPtr[float64](0.075), AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1"},
				}))
			})
			It("should take the default eviction threshold when none is specified", func() {
				nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{}
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Overhead.EvictionThreshold.Memory().String()).To(Equal("100Mi"))
			})
		})
		It("should set max-pods to user-defined value if specified", func() {
			instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
			Expect(err).To(BeNil())
			nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
				MaxPods: ptr.Int32(10),
			}
			for _, info := range instanceInfo {
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Capacity.Pods().Value()).To(BeNumerically("==", 10))
			}
		})
		It("should override pods-per-core value", func() {
			instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
			Expect(err).To(BeNil())
			nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
				PodsPerCore: ptr.Int32(1),
			}
			for _, info := range instanceInfo {
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Capacity.Pods().Value()).To(BeNumerically("==", info.CalcCpu))
			}
		})
		It("should take the minimum of pods-per-core and max-pods", func() {
			instanceInfo, err := ociEnv.InstanceTypesProvider.ListInstanceType(ctx)
			Expect(err).To(BeNil())
			nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
				PodsPerCore: ptr.Int32(4),
				MaxPods:     ptr.Int32(20),
			}
			for _, info := range instanceInfo {
				it := instancetype.NewInstanceType(
					ctx,
					info,
					nodeClass,
					nodeClass.Spec.Kubelet,
					"us-ashburn-1",
					[]string{"us-east-1"},
					ociEnv.InstanceTypesProvider.CreateOfferings(info, sets.New[string]("us-east-1")),
				)
				Expect(it.Capacity.Pods().Value()).To(BeNumerically("==", lo.Min([]int64{20, info.CalcCpu * 4})))
			}
		})
		It("shouldn't report more resources than are actually available on instances", func() {

			// reset cache
			ociEnv.Reset()
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("t4g.small"),
				IsFlexible: common.Bool(false), Ocpus: common.Float32(1), MemoryInGBs: common.Float32(4),
				NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)}})
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("t4g.medium"),
				IsFlexible: common.Bool(false), Ocpus: common.Float32(2), MemoryInGBs: common.Float32(8),
				NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)}})
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("t4g.xlarge"),
				IsFlexible: common.Bool(false), Ocpus: common.Float32(4), MemoryInGBs: common.Float32(16),
				NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)}})
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("m5.large"),
				IsFlexible: common.Bool(false), Ocpus: common.Float32(2), MemoryInGBs: common.Float32(8),
				NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)}})

			nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
				EvictionHard: map[string]string{"memory.available": "750Mi"},
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			its, err := cloudProvider.GetInstanceTypes(ctx, nodePool)
			Expect(err).To(BeNil())

			instanceTypes := map[string]*corecloudprovider.InstanceType{}
			for _, it := range its {
				instanceTypes[it.Name] = it
			}

			for _, tc := range []struct {
				InstanceType string
				// this need to verified in actual oci cluster
				// Actual allocatable values as reported by the node from kubelet. You find these
				// by launching the node and inspecting the node status allocatable.
				Memory resource.Quantity
				CPU    resource.Quantity
			}{
				{
					InstanceType: "t4g.small",
					Memory:       resource.MustParse("2062131Ki"),
					CPU:          resource.MustParse("1830m"),
				},
				{
					InstanceType: "t4g.medium",
					Memory:       resource.MustParse("5840486Ki"),
					CPU:          resource.MustParse("3815m"),
				},
				{
					InstanceType: "t4g.xlarge",
					Memory:       resource.MustParse("12551372Ki"),
					CPU:          resource.MustParse("7803m"),
				},
				{
					InstanceType: "m5.large",
					Memory:       resource.MustParse("5840486Ki"),
					CPU:          resource.MustParse("3815m"),
				},
			} {
				it, ok := instanceTypes[tc.InstanceType]
				Expect(ok).To(BeTrue(), fmt.Sprintf("didn't find instance type %q, add to instanceTypeTestData in ./hack/codegen.sh", tc.InstanceType))

				allocatable := it.Allocatable()
				// We need to ensure that our estimate of the allocatable resources <= the value that kubelet reports.  If it's greater,
				// we can launch nodes that can't actually run the pods.
				Expect(allocatable.Memory().AsApproximateFloat64()).To(BeNumerically("<=", tc.Memory.AsApproximateFloat64()),
					fmt.Sprintf("memory estimate for %s was too large, had %s vs %s", tc.InstanceType, allocatable.Memory().String(), tc.Memory.String()))
				Expect(allocatable.Cpu().AsApproximateFloat64()).To(BeNumerically("<=", tc.CPU.AsApproximateFloat64()),
					fmt.Sprintf("CPU estimate for %s was too large, had %s vs %s", tc.InstanceType, allocatable.Cpu().String(), tc.CPU.String()))
			}
		})
	})
	Context("Insufficient Capacity Error Cache", func() {
		It("should launch instances of different type on second reconciliation attempt with Insufficient Capacity Error Cache fallback", func() {
			ociEnv.CmpCli.InsufficientCapacityPools.Set([]fake.CapacityPool{{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-2", Zone: "US-ASHBURN-AD-1"}})
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pods := []*v1.Pod{
				coretest.UnschedulablePod(coretest.PodOptions{
					NodeSelector: map[string]string{v1.LabelTopologyZone: "US-ASHBURN-AD-1"},
					ResourceRequirements: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
						Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
					},
				}),
				coretest.UnschedulablePod(coretest.PodOptions{
					NodeSelector: map[string]string{v1.LabelTopologyZone: "US-ASHBURN-AD-1"},
					ResourceRequirements: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
						Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
					},
				}),
			}
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			// it should've tried to pack them on a single shape-3 then hit an insufficient capacity error
			for _, pod := range pods {
				ExpectNotScheduled(ctx, env.Client, pod)
			}
			nodeNames := sets.NewString()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			for _, pod := range pods {
				node := ExpectScheduled(ctx, env.Client, pod)
				nodeNames.Insert(node.Name)
			}
			Expect(nodeNames.Len()).To(Equal(1))
		})
		It("should launch instances in a different zone on second reconciliation attempt with Insufficient Capacity Error Cache fallback", func() {
			ociEnv.CmpCli.InsufficientCapacityPools.Set([]fake.CapacityPool{{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-2", Zone: "US-ASHBURN-AD-1"}})
			pod := coretest.UnschedulablePod(coretest.PodOptions{
				NodeSelector: map[string]string{v1.LabelInstanceTypeStable: "shape-2"},
				ResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
					Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
				},
			})
			pod.Spec.Affinity = &v1.Affinity{NodeAffinity: &v1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
				{
					Weight: 1, Preference: v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
						{Key: v1.LabelTopologyZone, Operator: v1.NodeSelectorOpIn, Values: []string{"US-ASHBURN-AD-1"}},
					}},
				},
			}}}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			// it should've tried to pack them in test-zone-1a on a p3.8xlarge then hit insufficient capacity, the next attempt will try test-zone-1b
			ExpectNotScheduled(ctx, env.Client, pod)

			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			node := ExpectScheduled(ctx, env.Client, pod)
			Expect(node.Labels).To(SatisfyAll(
				HaveKeyWithValue(v1.LabelInstanceTypeStable, "shape-2"),
				SatisfyAny(
					HaveKeyWithValue(v1.LabelTopologyZone, "US-ASHBURN-AD-3"),
					HaveKeyWithValue(v1.LabelTopologyZone, "US-ASHBURN-AD-2"),
				)))
		})
		It("should launch smaller instances than optimal if larger instance launch results in Insufficient Capacity Error", func() {
			ociEnv.CmpCli.InsufficientCapacityPools.Set([]fake.CapacityPool{
				{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-2", Zone: "US-ASHBURN-AD-1"},
			})
			nodePool.Spec.Template.Spec.Requirements = append(nodePool.Spec.Template.Spec.Requirements, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: v1.NodeSelectorRequirement{
					Key:      v1.LabelInstanceType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"shape-1", "shape-2"},
				},
			})
			pods := []*v1.Pod{}
			for i := 0; i < 2; i++ {
				pods = append(pods, coretest.UnschedulablePod(coretest.PodOptions{
					ResourceRequirements: v1.ResourceRequirements{
						Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
					},
					NodeSelector: map[string]string{
						v1.LabelTopologyZone: "US-ASHBURN-AD-1",
					},
				}))
			}
			// Provisions 2 shape-2 instances since shape-3 was insufficient
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			for _, pod := range pods {
				ExpectNotScheduled(ctx, env.Client, pod)
			}
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			for _, pod := range pods {
				node := ExpectScheduled(ctx, env.Client, pod)
				Expect(node.Labels[v1.LabelInstanceTypeStable]).To(Equal("shape-1"))
			}
		})
		It("should return all instance types, even though with no offerings due to Insufficient Capacity Error", func() {
			ociEnv.CmpCli.InsufficientCapacityPools.Set([]fake.CapacityPool{
				{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-3", Zone: "US-ASHBURN-AD-1"},
				{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-3", Zone: "US-ASHBURN-AD-2"},
				{CapacityType: karpv1.CapacityTypeOnDemand, InstanceType: "shape-3", Zone: "US-ASHBURN-AD-3"},
			})
			nodePool.Spec.Template.Spec.Requirements = nil
			nodePool.Spec.Template.Spec.Requirements = append(nodePool.Spec.Template.Spec.Requirements, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: v1.NodeSelectorRequirement{
					Key:      v1.LabelInstanceType,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"shape-3"},
				},
			},
			)
			nodePool.Spec.Template.Spec.Requirements = append(nodePool.Spec.Template.Spec.Requirements, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: v1.NodeSelectorRequirement{
					Key:      karpv1.CapacityTypeLabelKey,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"on-demand"},
				},
			})

			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			for _, ct := range []string{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeSpot} {
				for _, zone := range []string{"US-ASHBURN-AD-1", "US-ASHBURN-AD-2", "US-ASHBURN-AD-3"} {
					ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov,
						coretest.UnschedulablePod(coretest.PodOptions{
							ResourceRequirements: v1.ResourceRequirements{
								Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1")},
							},
							NodeSelector: map[string]string{
								karpv1.CapacityTypeLabelKey: ct,
								v1.LabelTopologyZone:        zone,
							},
						}))
				}
			}

			ociEnv.InstanceTypeCache.Flush()
			instanceTypes, err := cloudProvider.GetInstanceTypes(ctx, nodePool)
			Expect(err).To(BeNil())
			instanceTypeNames := sets.NewString()
			for _, it := range instanceTypes {
				instanceTypeNames.Insert(it.Name)
				if it.Name == "shape-3" {
					// should have no valid offerings
					Expect(it.Offerings.Available()).To(HaveLen(0))
				}
			}
			Expect(instanceTypeNames.Has("shape-3"))
		})
	})
	Context("CapacityType", func() {
		It("should default to on-demand", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			node := ExpectScheduled(ctx, env.Client, pod)
			Expect(node.Labels).To(HaveKeyWithValue(karpv1.CapacityTypeLabelKey, karpv1.CapacityTypeOnDemand))
		})
	})
	Context("Ephemeral Storage", func() {
		BeforeEach(func() {
			nodeClass.Spec.BlockDevices = []*v1alpha1.VolumeAttributes{{
				SizeInGBs: 100,
				VpusPerGB: 20},
			}
		})
		It("should default to EBS defaults when volumeSize is not defined in blockDeviceMappings for AL2 Root volume", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			node := ExpectScheduled(ctx, env.Client, pod)
			Expect(*node.Status.Capacity.StorageEphemeral()).To(Equal(resource.MustParse("100Gi")))
			Expect(ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Len()).To(BeNumerically(">=", 1))
			ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.ForEach(func(ltInput *core.LaunchInstanceRequest) {
				Expect(ltInput.LaunchVolumeAttachments).To(HaveLen(1))
				data, err := json.Marshal(ltInput.LaunchVolumeAttachments[0].GetLaunchCreateVolumeDetails())
				Expect(err).To(BeNil())
				volumeAttr := &core.LaunchCreateVolumeFromAttributes{}
				err = json.Unmarshal(data, volumeAttr)
				Expect(err).To(BeNil())
				Expect(lo.FromPtr(volumeAttr.SizeInGBs)).To(Equal(int64(100)))
				Expect(lo.FromPtr(volumeAttr.VpusPerGB)).To(Equal(int64(20)))
			})
		})
	})
	Context("Flex instance type", func() {
		BeforeEach(func() {
			ctx = options.ToContext(ctx, test.Options(test.OptionsFields{
				FlexCpuMemRatios: common.String("2,4"), FlexCpuConstrainList: common.String("1,2"), AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1"},
			}))
		})
		It("pod should schedule one suitable flex instance type", func() {
			ociEnv.CmpCli.DescribeInstanceTypesOutput.Add(&internalmodel.WrapShape{Shape: core.Shape{Shape: common.String("flex_instance"),
				IsFlexible: common.Bool(true), OcpuOptions: &core.ShapeOcpuOptions{
					Min: common.Float32(1),
					Max: common.Float32(16),
				}, MemoryOptions: &core.ShapeMemoryOptions{
					MinInGBs: common.Float32(2),
					MaxInGBs: common.Float32(128),
				},
				NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)}})
			pod := coretest.UnschedulablePod(coretest.PodOptions{
				NodeSelector: map[string]string{v1.LabelInstanceTypeStable: "flex_instance"},
				ResourceRequirements: v1.ResourceRequirements{
					Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1"), v1.ResourceMemory: resource.MustParse("5Gi")},
				},
			})
			pod.Spec.Affinity = &v1.Affinity{NodeAffinity: &v1.NodeAffinity{PreferredDuringSchedulingIgnoredDuringExecution: []v1.PreferredSchedulingTerm{
				{
					Weight: 1, Preference: v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{
						{Key: v1.LabelTopologyZone, Operator: v1.NodeSelectorOpIn, Values: []string{"US-ASHBURN-AD-1"}},
					}},
				},
			}}}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			node := ExpectScheduled(ctx, env.Client, pod)
			Expect(node.Labels).To(SatisfyAll(
				HaveKeyWithValue(v1.LabelInstanceTypeStable, "flex_instance"),
				HaveKeyWithValue(v1alpha1.LabelInstanceCPU, "2"),
				HaveKeyWithValue(v1alpha1.LabelInstanceMemory, "8192")))
		})
	})
})
