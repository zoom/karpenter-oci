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

package garbagecollection_test

import (
	"context"
	"github.com/awslabs/operatorpkg/object"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclaim/garbagecollection"
	"github.com/zoom/karpenter-oci/pkg/fake"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"github.com/zoom/karpenter-oci/pkg/utils"
	v1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/karpenter/pkg/events"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var ociEnv *test.Environment
var env *coretest.Environment
var garbageCollectionController *garbagecollection.Controller
var cloudProvider *cloudprovider.CloudProvider

func TestGarbageCollection(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GarbageCollection")
}

var _ = BeforeSuite(func() {
	ctx = options.ToContext(ctx, test.Options())
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...))
	ociEnv = test.NewEnvironment(ctx, env)
	cloudProvider = cloudprovider.New(ociEnv.InstanceTypesProvider, ociEnv.InstanceProvider, events.NewRecorder(&record.FakeRecorder{}),
		env.Client, ociEnv.AMIProvider)
	garbageCollectionController = garbagecollection.NewController(env.Client, cloudProvider)
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ociEnv.Reset()
})

var _ = Describe("GarbageCollection", func() {
	var instance *core.Instance
	var nodeClass *v1alpha1.OciNodeClass
	var providerID string

	BeforeEach(func() {
		instanceID := fake.InstanceID()
		providerID = instanceID
		nodeClass = test.OciNodeClass()
		nodePool := coretest.NodePool(karpv1.NodePool{
			Spec: karpv1.NodePoolSpec{
				Template: karpv1.NodeClaimTemplate{
					Spec: karpv1.NodeClaimTemplateSpec{
						NodeClassRef: &karpv1.NodeClassReference{
							Group: object.GVK(nodeClass).Group,
							Kind:  object.GVK(nodeClass).Kind,
							Name:  nodeClass.Name,
						},
					},
				},
			},
		})
		instance = &core.Instance{
			Id:                 common.String(instanceID),
			Shape:              common.String("VM.Standard.E4.Flex"),
			DisplayName:        common.String("dev-common-k8s-oci-cluster-karpenter"),
			FaultDomain:        common.String("FAULT-DOMAIN-1"),
			AvailabilityDomain: common.String("JPqd:US-ASHBURN-AD-1"),
			TimeCreated:        &common.SDKTime{Time: time.Now()},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: common.String("ocid1.image.oc1.iad.aaaaaaaa"),
			},
			DefinedTags: map[string]map[string]interface{}{options.FromContext(ctx).TagNamespace: {
				utils.SafeTagKey(karpv1.NodePoolLabelKey):         nodePool.Name,
				utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey): options.FromContext(ctx).ClusterName,
				utils.SafeTagKey(v1alpha1.LabelNodeClass):         nodeClass.Name}},
		}
	})
	AfterEach(func() {
		ExpectCleanedUp(ctx, env.Client)
	})

	It("should delete an instance if there is no NodeClaim owner", func() {
		// Launch time was 1m ago
		instance.TimeCreated = &common.SDKTime{Time: time.Now().Add(-time.Minute)}
		ociEnv.CmpCli.Instances.Store(*instance.Id, instance)

		ExpectSingletonReconciled(ctx, garbageCollectionController)
		_, err := cloudProvider.Get(ctx, providerID)
		Expect(err).To(HaveOccurred())
		Expect(corecloudprovider.IsNodeClaimNotFoundError(err)).To(BeTrue())
	})
	It("should delete an instance along with the node if there is no NodeClaim owner (to quicken scheduling)", func() {
		// Launch time was 1m ago
		instance.TimeCreated = &common.SDKTime{Time: time.Now().Add(-time.Minute)}
		ociEnv.CmpCli.Instances.Store(*instance.Id, instance)

		node := coretest.Node(coretest.NodeOptions{
			ProviderID: providerID,
		})
		ExpectApplied(ctx, env.Client, node)

		ExpectSingletonReconciled(ctx, garbageCollectionController)
		_, err := cloudProvider.Get(ctx, providerID)
		Expect(err).To(HaveOccurred())
		Expect(corecloudprovider.IsNodeClaimNotFoundError(err)).To(BeTrue())

		ExpectNotFound(ctx, env.Client, node)
	})
	It("should delete many instances if they all don't have NodeClaim owners", func() {
		// Generate 100 instances that have different instanceIDs
		var ids []string
		for i := 0; i < 100; i++ {
			instanceID := fake.InstanceID()
			ociEnv.CmpCli.Instances.Store(
				instanceID,
				&core.Instance{
					Id:                 common.String(instanceID),
					Shape:              common.String("VM.Standard.E4.Flex"),
					DisplayName:        common.String("dev-common-k8s-oci-cluster-karpenter"),
					FaultDomain:        common.String("FAULT-DOMAIN-1"),
					AvailabilityDomain: common.String("JPqd:US-ASHBURN-AD-1"),
					SourceDetails: core.InstanceSourceViaImageDetails{
						ImageId: common.String("ocid1.image.oc1.iad.aaaaaaaa"),
					},
					DefinedTags: map[string]map[string]interface{}{options.FromContext(ctx).TagNamespace: {
						utils.SafeTagKey(karpv1.NodePoolLabelKey):         "default",
						utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey): options.FromContext(ctx).ClusterName,
						utils.SafeTagKey(v1alpha1.LabelNodeClass):         "default"}},
					TimeCreated: &common.SDKTime{Time: time.Now().Add(-time.Minute)},
				},
			)
			ids = append(ids, instanceID)
		}
		ExpectSingletonReconciled(ctx, garbageCollectionController)

		wg := sync.WaitGroup{}
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer GinkgoRecover()
				defer wg.Done()

				_, err := cloudProvider.Get(ctx, id)
				Expect(err).To(HaveOccurred())
				Expect(corecloudprovider.IsNodeClaimNotFoundError(err)).To(BeTrue())
			}(id)
		}
		wg.Wait()
	})
	It("should not delete all instances if they all have NodeClaim owners", func() {
		// Generate 100 instances that have different instanceIDs
		var ids []string
		var nodeClaims []*karpv1.NodeClaim
		for i := 0; i < 100; i++ {
			instanceID := fake.InstanceID()
			ociEnv.CmpCli.Instances.Store(
				instanceID,
				&core.Instance{
					Id:                 common.String(instanceID),
					Shape:              common.String("VM.Standard.E4.Flex"),
					DisplayName:        common.String("dev-common-k8s-oci-cluster-karpenter"),
					FaultDomain:        common.String("FAULT-DOMAIN-1"),
					AvailabilityDomain: common.String("JPqd:US-ASHBURN-AD-1"),
					SourceDetails: core.InstanceSourceViaImageDetails{
						ImageId: common.String("ocid1.image.oc1.iad.aaaaaaaa"),
					},

					DefinedTags: map[string]map[string]interface{}{options.FromContext(ctx).TagNamespace: {
						utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey): options.FromContext(ctx).ClusterName}},
					TimeCreated: &common.SDKTime{Time: time.Now().Add(-time.Minute)},
				},
			)
			nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
				Spec: karpv1.NodeClaimSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: object.GVK(nodeClass).Group,
						Kind:  object.GVK(nodeClass).Kind,
						Name:  nodeClass.Name,
					},
				},
				Status: karpv1.NodeClaimStatus{
					ProviderID: instanceID,
				},
			})
			ExpectApplied(ctx, env.Client, nodeClaim)
			nodeClaims = append(nodeClaims, nodeClaim)
			ids = append(ids, instanceID)
		}
		ExpectSingletonReconciled(ctx, garbageCollectionController)

		wg := sync.WaitGroup{}
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer GinkgoRecover()
				defer wg.Done()

				_, err := cloudProvider.Get(ctx, id)
				Expect(err).ToNot(HaveOccurred())
			}(id)
		}
		wg.Wait()

		for _, nodeClaim := range nodeClaims {
			ExpectExists(ctx, env.Client, nodeClaim)
		}
	})
	It("should not delete an instance if it is within the NodeClaim resolution window (1m)", func() {
		// Launch time just happened
		instance.TimeCreated = &common.SDKTime{Time: time.Now()}
		ociEnv.CmpCli.Instances.Store(*instance.Id, instance)

		ExpectSingletonReconciled(ctx, garbageCollectionController)
		_, err := cloudProvider.Get(ctx, providerID)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should not delete an instance if it was not launched by a NodeClaim", func() {
		// Remove the "karpenter.sh/managed-by" tag (this isn't launched by a machine)
		delete(instance.DefinedTags[options.FromContext(ctx).TagNamespace], utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey))

		// Launch time was 1m ago
		instance.TimeCreated = &common.SDKTime{Time: time.Now().Add(-time.Minute)}
		ociEnv.CmpCli.Instances.Store(*instance.Id, instance)

		ExpectSingletonReconciled(ctx, garbageCollectionController)
		_, err := cloudProvider.Get(ctx, providerID)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should not delete the instance or node if it already has a NodeClaim that matches it", func() {
		// Launch time was 1m ago
		instance.TimeCreated = &common.SDKTime{Time: time.Now().Add(-time.Minute)}
		ociEnv.CmpCli.Instances.Store(*instance.Id, instance)

		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: providerID,
			},
		})
		node := coretest.Node(coretest.NodeOptions{
			ProviderID: providerID,
		})
		ExpectApplied(ctx, env.Client, nodeClaim, node)

		ExpectSingletonReconciled(ctx, garbageCollectionController)
		_, err := cloudProvider.Get(ctx, providerID)
		Expect(err).ToNot(HaveOccurred())
		ExpectExists(ctx, env.Client, node)
	})
	It("should not delete many instances or nodes if they already have NodeClaim owners that match it", func() {
		// Generate 100 instances that have different instanceIDs that have NodeClaims
		var ids []string
		var nodes []*v1.Node
		for i := 0; i < 100; i++ {
			instanceID := fake.InstanceID()
			ociEnv.CmpCli.Instances.Store(
				instanceID,
				&core.Instance{
					Id:                 common.String(instanceID),
					Shape:              common.String("VM.Standard.E4.Flex"),
					DisplayName:        common.String("dev-common-k8s-oci-cluster-karpenter"),
					FaultDomain:        common.String("FAULT-DOMAIN-1"),
					AvailabilityDomain: common.String("JPqd:US-ASHBURN-AD-1"),
					SourceDetails: core.InstanceSourceViaImageDetails{
						ImageId: common.String("ocid1.image.oc1.iad.aaaaaaaa"),
					},
					DefinedTags: map[string]map[string]interface{}{options.FromContext(ctx).TagNamespace: {
						utils.SafeTagKey(karpv1.NodePoolLabelKey):         "default",
						utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey): options.FromContext(ctx).ClusterName,
						utils.SafeTagKey(v1alpha1.LabelNodeClass):         nodeClass.Name}},
					TimeCreated: &common.SDKTime{Time: time.Now().Add(-time.Minute)},
				},
			)
			nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
				Spec: karpv1.NodeClaimSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: object.GVK(nodeClass).Group,
						Kind:  object.GVK(nodeClass).Kind,
						Name:  nodeClass.Name,
					},
				},
				Status: karpv1.NodeClaimStatus{
					ProviderID: instanceID,
				},
			})
			node := coretest.Node(coretest.NodeOptions{
				ProviderID: instanceID,
			})
			ExpectApplied(ctx, env.Client, nodeClaim, node)
			ids = append(ids, instanceID)
			nodes = append(nodes, node)
		}
		ExpectSingletonReconciled(ctx, garbageCollectionController)

		wg := sync.WaitGroup{}
		for i := range ids {
			wg.Add(1)
			go func(id string, node *v1.Node) {
				defer GinkgoRecover()
				defer wg.Done()

				_, err := cloudProvider.Get(ctx, id)
				Expect(err).ToNot(HaveOccurred())
				ExpectExists(ctx, env.Client, node)
			}(ids[i], nodes[i])
		}
		wg.Wait()
	})
})
