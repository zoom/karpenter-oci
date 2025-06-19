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

package cloudprovider

import (
	"context"
	"github.com/awslabs/operatorpkg/object"
	"github.com/awslabs/operatorpkg/status"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/fake"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/controllers/provisioning"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	"sigs.k8s.io/karpenter/pkg/events"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var stop context.CancelFunc
var env *coretest.Environment
var ociEnv *test.Environment
var prov *provisioning.Provisioner
var cloudProvider *CloudProvider
var cluster *state.Cluster
var fakeClock *clock.FakeClock
var recorder events.Recorder

func TestProvider(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "cloudProvider/OCI")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	ctx, stop = context.WithCancel(ctx)
	ociEnv = test.NewEnvironment(ctx, env)
	fakeClock = clock.NewFakeClock(time.Now())
	recorder = events.NewRecorder(&record.FakeRecorder{})
	cloudProvider = New(ociEnv.InstanceTypesProvider, ociEnv.InstanceProvider, recorder,
		env.Client, ociEnv.AMIProvider)
	cluster = state.NewCluster(fakeClock, env.Client, cloudProvider)
	prov = provisioning.NewProvisioner(env.Client, recorder, cloudProvider, cluster, fakeClock)
})

var _ = AfterSuite(func() {
	stop()
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	option := test.Options()
	option.AvailableDomains = []string{"JPqd:US-ASHBURN-AD-1", "JPqd:US-ASHBURN-AD-2", "JPqd:US-ASHBURN-AD-3"}
	ctx = options.ToContext(ctx, option)

	cluster.Reset()
	ociEnv.Reset()

	ociEnv.LaunchTemplateProvider.ClusterEndpoint = "https://test-cluster"
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("CloudProvider", func() {
	var nodeClass *v1alpha1.OciNodeClass
	var nodePool *karpv1.NodePool
	var nodeClaim *karpv1.NodeClaim

	var _ = BeforeEach(func() {
		nodeClass = test.OciNodeClass()
		nodeClass.StatusConditions().SetTrue(status.ConditionReady)
		nodePool = coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Template: karpv1.NodeClaimTemplate{
					Spec: karpv1.NodeClaimTemplateSpec{
						NodeClassRef: &karpv1.NodeClassReference{
							Group: object.GVK(nodeClass).Group,
							Kind:  object.GVK(nodeClass).Kind,
							Name:  nodeClass.Name,
						},
						Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
							{NodeSelectorRequirement: v1.NodeSelectorRequirement{Key: karpv1.CapacityTypeLabelKey, Operator: v1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand}}},
						},
					},
				},
			},
		})
		nodeClaim = coretest.NodeClaim(karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					karpv1.NodePoolLabelKey:    nodePool.Name,
					v1.LabelInstanceTypeStable: "shape-1",
				},
			},
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
			},
		})
		_, err := ociEnv.SubnetProvider.List(ctx, nodeClass) // Hydrate the subnet cache
		ociEnv.InstanceTypeCache.Flush()
		ociEnv.UnavailableOfferingsCache.Flush()
		Expect(err).To(BeNil())

	})
	It("should not proceed with instance creation if NodeClass is unknown", func() {
		nodeClass.StatusConditions().SetUnknown(status.ConditionReady)
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		_, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(err).To(HaveOccurred())
		Expect(cloudprovider.IsNodeClassNotReadyError(err)).To(BeFalse())
	})
	It("should return NodeClassNotReady error on creation if NodeClass is not ready", func() {
		nodeClass.StatusConditions().SetFalse(status.ConditionReady, "NodeClassNotReady", "NodeClass not ready")
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		_, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(err).To(HaveOccurred())
		Expect(cloudprovider.IsNodeClassNotReadyError(err)).To(BeTrue())
	})
	It("should return an ICE error when there are no instance types to launch", func() {
		// Specify no instance types and expect to receive a capacity error
		nodeClaim.Spec.Requirements = []karpv1.NodeSelectorRequirementWithMinValues{
			{
				NodeSelectorRequirement: v1.NodeSelectorRequirement{
					Key:      v1.LabelInstanceTypeStable,
					Operator: v1.NodeSelectorOpIn,
					Values:   []string{"test-instance-type"},
				},
			},
		}
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		cloudProviderNodeClaim, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(cloudprovider.IsInsufficientCapacityError(err)).To(BeTrue())
		Expect(cloudProviderNodeClaim).To(BeNil())
	})
	It("should set ImageID in the status field of the nodeClaim", func() {
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		cloudProviderNodeClaim, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(err).To(BeNil())
		Expect(cloudProviderNodeClaim).ToNot(BeNil())
		Expect(cloudProviderNodeClaim.Status.ImageID).ToNot(BeEmpty())
	})
	It("should return NodeClass Hash on the nodeClaim", func() {
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		cloudProviderNodeClaim, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(err).To(BeNil())
		Expect(cloudProviderNodeClaim).ToNot(BeNil())
		_, ok := cloudProviderNodeClaim.Annotations[v1alpha1.AnnotationOciNodeClassHash]
		Expect(ok).To(BeTrue())
	})
	It("should return NodeClass Hash Version on the nodeClaim", func() {
		ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
		cloudProviderNodeClaim, err := cloudProvider.Create(ctx, nodeClaim)
		Expect(err).To(BeNil())
		Expect(cloudProviderNodeClaim).ToNot(BeNil())
		v, ok := cloudProviderNodeClaim.Annotations[v1alpha1.AnnotationOciNodeClassHashVersion]
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal(v1alpha1.OciNodeClassHashVersion))
	})

	Context("NodeClaim Drift", func() {

		It("should return drift If NodeClass image is different from the nodeClaim image", func() {
			instanceTypes, _ := cloudProvider.GetInstanceTypes(ctx, nodePool)

			// Filter down to a single instance type
			instanceTypes = lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool { return i.Name == "shape-1" })

			// Since all the capacity pools are ICEd. This should return back an ICE error
			instance, _ := ociEnv.InstanceProvider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

			// update nodeclass image to shape-2
			nodeClass.Status.Images = []*v1alpha1.Image{
				{
					Id:            "ocid1.image.oc1.iad.shape-2",
					Name:          "shape-2",
					CompartmentId: "ocid1.compartment.oc1..aaaaaaaa",
				},
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
			reason, err := cloudProvider.isImageDrifted(ctx, nodeClaim, nodePool, instance, nodeClass)
			Expect(reason).NotTo(BeEmpty())
			Expect(err).To(BeNil())
		})
		It("should return no drift If NodeClass image is same from the nodeClaim image", func() {
			instanceTypes, _ := cloudProvider.GetInstanceTypes(ctx, nodePool)

			// Filter down to a single instance type
			instanceTypes = lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool { return i.Name == "shape-1" })

			// Since all the capacity pools are ICEd. This should return back an ICE error
			instance, _ := ociEnv.InstanceProvider.Create(ctx, nodeClass, nodeClaim, instanceTypes)

			nodeClass.Status.Images = []*v1alpha1.Image{
				{
					Id:            "ocid1.image.oc1.iad.aaaaaaaa",
					Name:          "shape-1",
					CompartmentId: "ocid1.compartment.oc1..aaaaaaaa",
				},
			}
			ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
			reason, err := cloudProvider.isImageDrifted(ctx, nodeClaim, nodePool, instance, nodeClass)
			Expect(reason).To(BeEmpty())
			Expect(err).To(BeNil())
		})
		It("should return drift If NodeClass subnets does not contain the nodeClaim's", func() {

			nodeClass.Status.Subnets = []*v1alpha1.Subnet{
				&v1alpha1.Subnet{
					Id: "subnets-1",
				},
				&v1alpha1.Subnet{
					Id: "subnets-2",
				},
				&v1alpha1.Subnet{
					Id: "subnets-3",
				},
			}

			ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)

			subnets := []core.Subnet{
				core.Subnet{
					Id: common.String("subnets-10"),
				},
				core.Subnet{
					Id: common.String("subnets-2"),
				},
				core.Subnet{
					Id: common.String("subnets-3"),
				},
			}
			reason, err := cloudProvider.isSubnetDrifted(subnets, nodeClass)
			Expect(reason).NotTo(BeEmpty())
			Expect(err).To(BeNil())

		})
		It("should return no drift If NodeClass subnets contains the nodeClaim's", func() {
			nodeClass.Status.Subnets = []*v1alpha1.Subnet{
				&v1alpha1.Subnet{
					Id: "subnets-1",
				},
				&v1alpha1.Subnet{
					Id: "subnets-2",
				},
				&v1alpha1.Subnet{
					Id: "subnets-3",
				},
			}

			//ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)

			subnets := []core.Subnet{
				core.Subnet{
					Id: common.String("subnets-1"),
				},
				core.Subnet{
					Id: common.String("subnets-2"),
				},
				core.Subnet{
					Id: common.String("subnets-3"),
				},
			}
			reason, err := cloudProvider.isSubnetDrifted(subnets, nodeClass)
			Expect(reason).To(BeEmpty())
			Expect(err).To(BeNil())

		})
		It("should return drift If NodeClass security groups does not contain the nodeClaim sgs", func() {

			sgs := []core.NetworkSecurityGroup{
				core.NetworkSecurityGroup{Id: common.String("sg-1")},
				core.NetworkSecurityGroup{Id: common.String("sg-2")},
				core.NetworkSecurityGroup{Id: common.String("sg-3")},
			}

			nodeClass.Status.SecurityGroups = []*v1alpha1.SecurityGroup{
				&v1alpha1.SecurityGroup{Id: "sg-10"},
				&v1alpha1.SecurityGroup{Id: "sg-20"},
				&v1alpha1.SecurityGroup{Id: "sg-30"},
			}
			reason, err := cloudProvider.areSecurityGroupsDrifted(sgs, nodeClass)
			Expect(reason).NotTo(BeEmpty())
			Expect(err).To(BeNil())
		})
		It("should return no drift If NodeClass security groups contain the nodeClaim sgs", func() {
			sgs := []core.NetworkSecurityGroup{
				core.NetworkSecurityGroup{Id: common.String("sg-1")},
				core.NetworkSecurityGroup{Id: common.String("sg-2")},
				core.NetworkSecurityGroup{Id: common.String("sg-3")},
			}

			nodeClass.Status.SecurityGroups = []*v1alpha1.SecurityGroup{
				&v1alpha1.SecurityGroup{Id: "sg-1"},
				&v1alpha1.SecurityGroup{Id: "sg-2"},
				&v1alpha1.SecurityGroup{Id: "sg-3"},
			}
			reason, err := cloudProvider.areSecurityGroupsDrifted(sgs, nodeClass)
			Expect(reason).To(BeEmpty())
			Expect(err).To(BeNil())
		})

		It("no drift", func() {
			CreateOciTestResource(nodePool, nodeClass, nodeClaim)
			reason, err := cloudProvider.IsDrifted(ctx, nodeClaim)
			Expect(reason).To(BeEmpty())
			Expect(err).To(BeNil())

		})

		It("drift if sg update", func() {
			CreateOciTestResource(nodePool, nodeClass, nodeClaim)

			nodeClass.Status.SecurityGroups = []*v1alpha1.SecurityGroup{
				&v1alpha1.SecurityGroup{Id: *fake.DefaultSecurityGroup[0].Id},
				&v1alpha1.SecurityGroup{Id: *fake.DefaultSecurityGroup[1].Id},
			}

			ExpectApplied(ctx, env.Client, nodeClass)

			reason, err := cloudProvider.IsDrifted(ctx, nodeClaim)
			Expect(reason).NotTo(BeEmpty())
			Expect(err).To(BeNil())

		})
	})
})

func CreateOciTestResource(nodePool *karpv1.NodePool, nodeClass *v1alpha1.OciNodeClass, nodeClaim *karpv1.NodeClaim) {
	// create nodeClass
	nodeClass.Status.Images = []*v1alpha1.Image{
		{
			Id:            "ocid1.image.oc1.iad.aaaaaaaa",
			Name:          "shape-1",
			CompartmentId: "ocid1.compartment.oc1..aaaaaaaa",
		},
	}
	nodeClass.Status.Subnets = []*v1alpha1.Subnet{
		&v1alpha1.Subnet{
			Id: *fake.DefaultSubnets[0].Id,
		},
		&v1alpha1.Subnet{
			Id: *fake.DefaultSubnets[1].Id,
		},
		&v1alpha1.Subnet{
			Id: *fake.DefaultSubnets[2].Id,
		},
	}

	nodeClass.Status.SecurityGroups = []*v1alpha1.SecurityGroup{
		&v1alpha1.SecurityGroup{Id: *fake.DefaultSecurityGroup[0].Id},
		&v1alpha1.SecurityGroup{Id: *fake.DefaultSecurityGroup[1].Id},
		&v1alpha1.SecurityGroup{Id: *fake.DefaultSecurityGroup[2].Id},
	}

	// create instance
	instanceTypes, _ := cloudProvider.GetInstanceTypes(ctx, nodePool)

	// Filter down to a single instance type
	instanceTypes = lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool { return i.Name == "shape-1" })

	// Since all the capacity pools are ICEd. This should return back an ICE error
	instance, _ := ociEnv.InstanceProvider.Create(ctx, nodeClass, nodeClaim, instanceTypes)
	nodeClaim.Status.ProviderID = *instance.Id
	ExpectApplied(ctx, env.Client, nodePool, nodeClass, nodeClaim)
}

func CleanResource(nodePool *karpv1.NodePool, nodeClass *v1alpha1.OciNodeClass, nodeClaim *karpv1.NodeClaim, instance *core.Instance) {
	_ = env.Client.Delete(ctx, nodeClaim)
	_ = env.Client.Delete(ctx, nodeClass)
	// create nodeClaim
	_ = cloudProvider.Delete(ctx, nodeClaim)
	if instance != nil {
		_ = ociEnv.InstanceProvider.Delete(ctx, *instance.Id)
	}
}
