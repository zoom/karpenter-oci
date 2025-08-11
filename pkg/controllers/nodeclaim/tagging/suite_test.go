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

package tagging_test

import (
	"context"
	"github.com/awslabs/operatorpkg/object"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis"
	oci_v1alpha1 "github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclaim/tagging"
	"github.com/zoom/karpenter-oci/pkg/fake"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"testing"

	"sigs.k8s.io/karpenter/pkg/events"
	"sigs.k8s.io/karpenter/pkg/test/v1alpha1"

	"github.com/samber/lo"
	corev1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var ociEnv *test.Environment
var env *coretest.Environment
var taggingController *tagging.Controller

func TestAPIs(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "TaggingController")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(v1alpha1.CRDs...))
	ctx = coreoptions.ToContext(ctx, coretest.Options(coretest.OptionsFields{FeatureGates: coretest.FeatureGates{ReservedCapacity: lo.ToPtr(true)}}))
	ctx = options.ToContext(ctx, test.Options())
	ociEnv = test.NewEnvironment(ctx, env)
	cloudProvider := cloudprovider.New(ociEnv.InstanceTypesProvider, ociEnv.InstanceProvider, events.NewRecorder(&record.FakeRecorder{}),
		env.Client, ociEnv.AMIProvider)
	taggingController = tagging.NewController(env.Client, cloudProvider, ociEnv.InstanceProvider)
})
var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ociEnv.Reset()
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("TaggingController", func() {
	var ociInstance *core.Instance
	var nodeClass *oci_v1alpha1.OciNodeClass

	BeforeEach(func() {
		ociInstance = &core.Instance{
			LifecycleState: core.InstanceLifecycleStateRunning,
			Id:             lo.ToPtr(fake.InstanceID()),
		}

		ociEnv.CmpCli.Instances.Store(lo.FromPtr(ociInstance.Id), ociInstance)
		nodeClass = test.OciNodeClass()
		nodeClass.Spec.FreeFormTags = map[string]string{"custom_key": "value", "custom_ke2": "value2"}
		ExpectApplied(ctx, env.Client, nodeClass)
	})

	It("shouldn't tag instances without a Node", func() {
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: lo.FromPtr(ociInstance.Id),
			},
		})

		ExpectApplied(ctx, env.Client, nodeClaim)
		ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
		Expect(nodeClaim.Annotations).To(Not(HaveKey(oci_v1alpha1.AnnotationInstanceTagged)))
		Expect(ociInstance.FreeformTags["custom_key"]).To(BeEmpty())
	})

	It("shouldn't tag nodeclaim with a malformed provderID", func() {
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: "Bad providerID",
				NodeName:   "default",
			},
		})

		ExpectApplied(ctx, env.Client, nodeClaim)
		ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
		Expect(nodeClaim.Annotations).To(Not(HaveKey(oci_v1alpha1.AnnotationInstanceTagged)))
		Expect(ociInstance.FreeformTags["custom_key"]).To(BeEmpty())
	})

	It("should gracefully handle missing NodeClaim", func() {
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: lo.FromPtr(ociInstance.Id),
				NodeName:   "default",
			},
		})

		ExpectApplied(ctx, env.Client, nodeClaim)
		ExpectDeleted(ctx, env.Client, nodeClaim)
		ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
	})

	It("should gracefully handle missing instance", func() {
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: lo.FromPtr(ociInstance.Id),
				NodeName:   "default",
			},
		})

		ExpectApplied(ctx, env.Client, nodeClaim)
		ociEnv.CmpCli.Instances.Delete(lo.FromPtr(ociInstance.Id))
		ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
		Expect(nodeClaim.Annotations).To(Not(HaveKey(oci_v1alpha1.AnnotationInstanceTagged)))
	})

	It("shouldn't tag nodeclaim with deletion timestamp set", func() {
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
			Status: karpv1.NodeClaimStatus{
				ProviderID: lo.FromPtr(ociInstance.Id),
				NodeName:   "default",
			},
			ObjectMeta: corev1.ObjectMeta{
				Finalizers: []string{"testing/finalizer"},
			},
		})

		ExpectApplied(ctx, env.Client, nodeClaim)
		Expect(env.Client.Delete(ctx, nodeClaim)).To(Succeed())
		ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
		Expect(nodeClaim.Annotations).To(Not(HaveKey(oci_v1alpha1.AnnotationInstanceTagged)))
		Expect(ociInstance.FreeformTags["custom_key"]).To(BeEmpty())
	})

	DescribeTable(
		"should tag taggable instances",
		func(expectedTags map[string]string) {
			nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
				Spec: karpv1.NodeClaimSpec{
					NodeClassRef: &karpv1.NodeClassReference{
						Group: object.GVK(nodeClass).Group,
						Kind:  object.GVK(nodeClass).Kind,
						Name:  nodeClass.Name,
					},
				},
				Status: karpv1.NodeClaimStatus{
					ProviderID: lo.FromPtr(ociInstance.Id),
					NodeName:   "default",
				},
			})
			for k, v := range expectedTags {
				nodeClass.Spec.FreeFormTags[k] = v
			}

			ociEnv.CmpCli.Instances.Store(lo.ToPtr(ociInstance.Id), ociInstance)

			ExpectApplied(ctx, env.Client, nodeClass, nodeClaim)
			ExpectObjectReconciled(ctx, env.Client, taggingController, nodeClaim)
			nodeClaim = ExpectExists(ctx, env.Client, nodeClaim)
			Expect(nodeClaim.Annotations).To(HaveKey(oci_v1alpha1.AnnotationInstanceTagged))

			ociInstance := lo.Must(ociEnv.CmpCli.Instances.Load(lo.FromPtr(ociInstance.Id))).(*core.Instance)
			instanceTags := ociInstance.FreeformTags

			for tag, value := range expectedTags {
				Expect(instanceTags).To(HaveKeyWithValue(tag, value))
			}
		},
		Entry("with the custom tag", map[string]string{"CustomKey": "CustomValue"}),
	)
})
