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

package launchtemplate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/awslabs/operatorpkg/status"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/karpenter/pkg/controllers/provisioning"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	"sigs.k8s.io/karpenter/pkg/events"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"

	. "sigs.k8s.io/karpenter/pkg/test/expectations"
)

var ctx context.Context
var stop context.CancelFunc
var env *coretest.Environment
var ociEnv *test.Environment
var fakeClock *clock.FakeClock
var prov *provisioning.Provisioner
var cluster *state.Cluster
var cloudProvider *cloudprovider.CloudProvider

func TestLaunchTemplate(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "LaunchTemplateProvider")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...), coretest.WithFieldIndexers(test.OciNodeClassFieldIndexer(ctx)))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options(test.OptionsFields{AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1", "JPqd:US-ASHBURN-AD-2", "JPqd:US-ASHBURN-AD-3"}}))
	ctx, stop = context.WithCancel(ctx)
	ociEnv = test.NewEnvironment(ctx, env)

	fakeClock = &clock.FakeClock{}
	cloudProvider = cloudprovider.New(ociEnv.InstanceTypesProvider, ociEnv.InstanceProvider, events.NewRecorder(&record.FakeRecorder{}),
		env.Client, ociEnv.AMIProvider)
	cluster = state.NewCluster(fakeClock, env.Client)
	prov = provisioning.NewProvisioner(env.Client, events.NewRecorder(&record.FakeRecorder{}), cloudProvider, cluster, fakeClock)
})

var _ = AfterSuite(func() {
	stop()
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options(test.OptionsFields{AvailableDomains: []string{"JPqd:US-ASHBURN-AD-1", "JPqd:US-ASHBURN-AD-2", "JPqd:US-ASHBURN-AD-3"}}))
	cluster.Reset()
	ociEnv.Reset()

	ociEnv.LaunchTemplateProvider.ClusterEndpoint = "https://test-cluster"
	ociEnv.LaunchTemplateProvider.CABundle = lo.ToPtr("ca-bundle")
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("LaunchTemplate Provider", func() {
	var nodePool *karpv1.NodePool
	var nodeClass *v1alpha1.OciNodeClass
	BeforeEach(func() {
		nodeClass = test.OciNodeClass()
		nodeClass.StatusConditions().SetTrue(status.ConditionReady)
		nodePool = coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Template: karpv1.NodeClaimTemplate{
					ObjectMeta: karpv1.ObjectMeta{
						Labels: map[string]string{coretest.DiscoveryLabel: "unspecified"},
					},
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

	Context("Labels", func() {
		It("should apply labels to the node", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			node := ExpectScheduled(ctx, env.Client, pod)
			Expect(node.Labels).To(HaveKey(v1.LabelOSStable))
			Expect(node.Labels).To(HaveKey(v1.LabelArchStable))
			Expect(node.Labels).To(HaveKey(v1.LabelInstanceTypeStable))
		})
	})
	Context("Tags", func() {
		It("should combine custom tags and static tags", func() {
			nodeClass.Spec.Tags = map[string]string{
				"tag1": "tag1value",
				"tag2": "tag2value",
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectScheduled(ctx, env.Client, pod)
			Expect(ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Len()).To(Equal(1))
			createFleetInput := ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Pop()
			Expect(createFleetInput.DefinedTags[options.FromContext(ctx).TagNamespace]).To(HaveLen(5))

			// tags should be included in instance
			ExpectTags(createFleetInput.DefinedTags[options.FromContext(ctx).TagNamespace], nodeClass.Spec.Tags)
		})
	})
	Context("Block Device Mappings", func() {
		It("should use custom block device mapping", func() {
			nodeClass.Spec.BlockDevices = []*v1alpha1.VolumeAttributes{
				{
					SizeInGBs: 100,
					VpusPerGB: 20,
				},
				{
					SizeInGBs: 50,
					VpusPerGB: 10,
				},
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectScheduled(ctx, env.Client, pod)
			Expect(ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.Len()).To(BeNumerically(">=", 1))
			ociEnv.CmpCli.LaunchInstanceBehavior.CalledWithInput.ForEach(func(ltInput *core.LaunchInstanceRequest) {
				volumeAttrs, err := GetVolumeDetails(ltInput)
				Expect(err).To(BeNil())
				Expect(lo.FromPtr(volumeAttrs[0].SizeInGBs)).To(Equal(int64(100)))
				Expect(lo.FromPtr(volumeAttrs[1].SizeInGBs)).To(Equal(int64(50)))
			})
		})
	})
	Context("Ephemeral Storage", func() {
		It("should pack pods when a daemonset has an ephemeral-storage request", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass, test.DaemonSet(
				test.DaemonSetOptions{
					PodOptions: coretest.PodOptions{
						ResourceRequirements: v1.ResourceRequirements{
							Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1"),
								v1.ResourceMemory:           resource.MustParse("1Gi"),
								v1.ResourceEphemeralStorage: resource.MustParse("1Gi")}},
					}},
			))
			pod := coretest.UnschedulablePod()
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectScheduled(ctx, env.Client, pod)
		})
		It("should pack pods with any ephemeral-storage request", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceEphemeralStorage: resource.MustParse("1G"),
				}}})
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectScheduled(ctx, env.Client, pod)
		})
		It("should pack pods with large ephemeral-storage request", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
				}}})
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectScheduled(ctx, env.Client, pod)
		})
		It("should not pack pods if the sum of pod ephemeral-storage and overhead exceeds node capacity", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceEphemeralStorage: resource.MustParse("109Gi"),
				}}})
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectNotScheduled(ctx, env.Client, pod)
		})
		It("should launch multiple nodes if sum of pod ephemeral-storage requests exceeds a single nodes capacity", func() {
			var nodes []*v1.Node
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pods := []*v1.Pod{
				coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceEphemeralStorage: resource.MustParse("55Gi"),
					},
				},
				}),
				coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceEphemeralStorage: resource.MustParse("55Gi"),
					},
				},
				}),
			}
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			for _, pod := range pods {
				nodes = append(nodes, ExpectScheduled(ctx, env.Client, pod))
			}
			Expect(nodes).To(HaveLen(2))
		})
		It("should only pack pods with ephemeral-storage requests that will fit on an available node", func() {
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pods := []*v1.Pod{
				coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
					},
				},
				}),
				coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceEphemeralStorage: resource.MustParse("150Gi"),
					},
				},
				}),
			}
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pods...)
			ExpectScheduled(ctx, env.Client, pods[0])
			ExpectNotScheduled(ctx, env.Client, pods[1])
		})
		It("should not pack pod if no available instance types have enough storage", func() {
			ExpectApplied(ctx, env.Client, nodePool)
			pod := coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceEphemeralStorage: resource.MustParse("150Gi"),
				},
			},
			})
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)
			ExpectNotScheduled(ctx, env.Client, pod)
		})
		It("should pack pods using the blockdevicemappings from the provider spec when defined", func() {
			nodeClass.Spec.BlockDevices = []*v1alpha1.VolumeAttributes{
				{
					SizeInGBs: 100,
					VpusPerGB: 20,
				},
				{
					SizeInGBs: 50,
					VpusPerGB: 10,
				},
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass)
			pod := coretest.UnschedulablePod(coretest.PodOptions{ResourceRequirements: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceEphemeralStorage: resource.MustParse("25Gi"),
				},
			},
			})
			ExpectProvisioned(ctx, env.Client, cluster, cloudProvider, prov, pod)

			// capacity isn't recorded on the node any longer, but we know the pod should schedule
			ExpectScheduled(ctx, env.Client, pod)
		})
	})
	// todo userdata test
})

// ExpectTags verifies that the expected tags are a subset of the tags found
func ExpectTags(existingTags map[string]interface{}, expected map[string]string) {
	GinkgoHelper()
	for expKey, expValue := range expected {
		foundValue, ok := existingTags[expKey]
		Expect(ok).To(BeTrue(), fmt.Sprintf("expected to find tag %s in %s", expKey, existingTags))
		Expect(foundValue.(string)).To(Equal(expValue))
	}
}

func GetVolumeDetails(request *core.LaunchInstanceRequest) ([]*core.LaunchCreateVolumeFromAttributes, error) {
	res := make([]*core.LaunchCreateVolumeFromAttributes, 0)
	for _, item := range request.LaunchVolumeAttachments {
		data, err := json.Marshal(item.GetLaunchCreateVolumeDetails())
		if err != nil {
			return nil, err
		}
		volumeAttr := &core.LaunchCreateVolumeFromAttributes{}
		err = json.Unmarshal(data, volumeAttr)
		if err != nil {
			return nil, err
		}
		res = append(res, volumeAttr)
	}
	return res, nil
}
