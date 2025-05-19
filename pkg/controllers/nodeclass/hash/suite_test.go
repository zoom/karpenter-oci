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

package hash

import (
	"context"
	"github.com/awslabs/operatorpkg/object"
	"github.com/imdario/mergo"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"github.com/zoom/karpenter-oci/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var env *coretest.Environment
var ociEnv *test.Environment
var hashController *Controller

func TestAPIs(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "OciNodeClass")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...), coretest.WithFieldIndexers(test.OciNodeClassFieldIndexer(ctx)))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	ociEnv = test.NewEnvironment(ctx, env)

	hashController = NewController(env.Client)
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ociEnv.Reset()
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("NodeClass Hash Controller", func() {
	var nodeClass *v1alpha1.OciNodeClass
	var nodePool *karpv1.NodePool
	BeforeEach(func() {
		nodeClass = test.OciNodeClass(v1alpha1.OciNodeClass{
			Spec: v1alpha1.OciNodeClassSpec{
				SubnetSelector: []v1alpha1.SubnetSelectorTerm{{
					Name: "private-1",
				}},
				ImageSelector: []v1alpha1.ImageSelectorTerm{{Name: "Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679"}},
			},
		})
		nodePool = coretest.NodePool(karpv1.NodePool{
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
	})
	DescribeTable("should update the drift hash when static field is updated", func(changes *v1alpha1.OciNodeClass) {
		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, hashController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)

		expectedHash := nodeClass.Hash()
		Expect(nodeClass.ObjectMeta.Annotations[v1alpha1.AnnotationOciNodeClassHash]).To(Equal(expectedHash))

		Expect(mergo.Merge(nodeClass, changes, mergo.WithOverride)).To(Succeed())

		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, hashController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)

		expectedHashTwo := nodeClass.Hash()
		Expect(nodeClass.Annotations[v1alpha1.AnnotationOciNodeClassHash]).To(Equal(expectedHashTwo))
		Expect(expectedHash).ToNot(Equal(expectedHashTwo))

	},
		Entry("UserData Drift", &v1alpha1.OciNodeClass{Spec: v1alpha1.OciNodeClassSpec{UserData: utils.String("userdata-test-2")}}),
		Entry("Tags Drift", &v1alpha1.OciNodeClass{Spec: v1alpha1.OciNodeClassSpec{Tags: map[string]string{"keyTag-test-3": "valueTag-test-3"}}}),
		Entry("BlockDeviceMappings Drift", &v1alpha1.OciNodeClass{Spec: v1alpha1.OciNodeClassSpec{BlockDevices: []*v1alpha1.VolumeAttributes{{SizeInGBs: 1, VpusPerGB: 1}}}}),
	)
	It("should not update ec2nodeclass-hash on all NodeClaims when the ec2nodeclass-hash-version matches the controller hash version", func() {
		nodeClass.Annotations = map[string]string{
			v1alpha1.AnnotationOciNodeClassHash:        "abceduefed",
			v1alpha1.AnnotationOciNodeClassHashVersion: "test-version",
		}
		nodeClaim := coretest.NodeClaim(karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{karpv1.NodePoolLabelKey: nodePool.Name},
				Annotations: map[string]string{
					v1alpha1.AnnotationOciNodeClassHash:        "1234564654",
					v1alpha1.AnnotationOciNodeClassHashVersion: v1alpha1.OciNodeClassHashVersion,
				},
			},
			Spec: karpv1.NodeClaimSpec{
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		ExpectApplied(ctx, env.Client, nodeClass, nodeClaim, nodePool)

		ExpectObjectReconciled(ctx, env.Client, hashController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		nodeClaim = ExpectExists(ctx, env.Client, nodeClaim)

		expectedHash := nodeClass.Hash()

		// Expect nodeclass-hash on the NodeClass to be updated
		Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, expectedHash))
		Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
		// Expect nodeclass-hash on the NodeClaims to stay the same
		Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, "1234564654"))
		Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
	})

})
