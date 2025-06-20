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

package nodeclaim_test

import (
	"fmt"
	"github.com/awslabs/operatorpkg/object"
	. "github.com/onsi/ginkgo/v2"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/test"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
	"time"

	. "github.com/onsi/gomega"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
)

var _ = Describe("StandaloneNodeClaim", func() {
	It("should create a standard NodeClaim within the flex shape", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		nodeClaim = env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		Expect(node.Labels).To(HaveKeyWithValue(v1alpha1.LabelInstanceShapeName, "VM.Standard.E4.Flex"))
		env.EventuallyExpectNodeClaimsReady(nodeClaim)
	})
	It("should create a standard NodeClaim based on resource requests", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{"BM.Standard.A1.160", "VM.Standard.A1.Flex", "VM.Standard.A2.Flex"},
						},
					},
				},
				Resources: karpv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("64Gi"),
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		nodeClaim = env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		Expect(resources.Fits(nodeClaim.Spec.Resources.Requests, node.Status.Allocatable))
		env.EventuallyExpectNodeClaimsReady(nodeClaim)
	})
	It("should create a NodeClaim propagating all the NodeClaim spec details", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"custom-annotation": "custom-value",
				},
				Labels: map[string]string{
					"custom-label": "custom-value",
				},
			},
			Spec: karpv1.NodeClaimSpec{
				Taints: []corev1.Taint{
					{
						Key:    "custom-taint",
						Effect: corev1.TaintEffectNoSchedule,
						Value:  "custom-value",
					},
					{
						Key:    "other-custom-taint",
						Effect: corev1.TaintEffectNoExecute,
						Value:  "other-custom-value",
					},
				},
				Resources: karpv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		Expect(node.Annotations).To(HaveKeyWithValue("custom-annotation", "custom-value"))
		Expect(node.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
		Expect(node.Spec.Taints).To(ContainElements(
			corev1.Taint{
				Key:    "custom-taint",
				Effect: corev1.TaintEffectNoSchedule,
				Value:  "custom-value",
			},
			corev1.Taint{
				Key:    "other-custom-taint",
				Effect: corev1.TaintEffectNoExecute,
				Value:  "other-custom-value",
			},
		))
		Expect(node.OwnerReferences).To(ContainElement(
			metav1.OwnerReference{
				APIVersion:         object.GVK(nodeClaim).GroupVersion().String(),
				Kind:               "NodeClaim",
				Name:               nodeClaim.Name,
				UID:                nodeClaim.UID,
				BlockOwnerDeletion: lo.ToPtr(true),
			},
		))
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nodeClaim)
	})
	It("should remove the cloudProvider NodeClaim when the cluster NodeClaim is deleted", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		nodeClaim = env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]

		instanceID := env.ExpectParsedProviderID(node.Spec.ProviderID)
		env.GetInstance(node.Name)

		// Node is deleted and now should be not found
		env.ExpectDeleted(nodeClaim)
		env.EventuallyExpectNotFound(nodeClaim, node)

		Eventually(func(g Gomega) {
			g.Expect(env.GetInstanceByID(instanceID).LifecycleState).To(BeElementOf(core.InstanceLifecycleStateTerminating, core.InstanceLifecycleStateTerminated))
		}, time.Second*10).Should(Succeed())
	})
	It("should delete a NodeClaim from the node termination finalizer", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		nodeClaim = env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]

		instanceID := env.ExpectParsedProviderID(node.Spec.ProviderID)
		env.GetInstance(node.Name)

		// Delete the node and expect both the node and nodeClaim to be gone as well as the instance to be shutting-down
		env.ExpectDeleted(node)
		env.EventuallyExpectNotFound(nodeClaim, node)

		Eventually(func(g Gomega) {
			g.Expect(env.GetInstanceByID(instanceID).LifecycleState).To(BeElementOf(core.InstanceLifecycleStateTerminating, core.InstanceLifecycleStateTerminated))
		}, time.Second*10).Should(Succeed())
	})
	It("should create a NodeClaim with custom labels passed through the userData", func() {
		nodeClass.Spec.ImageFamily = v1alpha1.CustomImageFamily
		rawContent, err := os.ReadFile("testdata/custom_userdata_label.sh")
		Expect(err).ToNot(HaveOccurred())
		url, _ := url.Parse(env.ClusterEndpoint)
		nodeClass.Spec.UserData = lo.ToPtr(fmt.Sprintf(string(rawContent), url.Hostname(), env.ClusterDns, env.ExpectCABundle()))

		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      corev1.LabelArchStable,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"amd64"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		env.ExpectCreated(nodeClass, nodeClaim)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		Expect(node.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
		Expect(node.Labels).To(HaveKeyWithValue("custom-label2", "custom-value2"))

		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nodeClaim)
	})
	It("should delete a NodeClaim after the registration timeout when the node doesn't register", func() {

		// Create userData that adds custom labels through the --node-labels
		nodeClass.Spec.ImageFamily = v1alpha1.CustomImageFamily
		rawContent, err := os.ReadFile("testdata/custom_userdata_input.sh")
		Expect(err).ToNot(HaveOccurred())
		nodeClass.Spec.UserData = lo.ToPtr(fmt.Sprintf(string(rawContent), "badEndpoint.com", env.ClusterDns, env.ExpectCABundle()))

		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      corev1.LabelArchStable,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"amd64"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})

		env.ExpectCreated(nodeClass, nodeClaim)
		nodeClaim = env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]

		// Expect that the nodeClaim eventually launches and has false Registration/Initialization
		Eventually(func(g Gomega) {
			temp := &karpv1.NodeClaim{}
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), temp)).To(Succeed())
			g.Expect(temp.StatusConditions().Get(karpv1.ConditionTypeLaunched).IsTrue()).To(BeTrue())
			g.Expect(temp.StatusConditions().Get(karpv1.ConditionTypeRegistered).IsUnknown()).To(BeTrue())
			g.Expect(temp.StatusConditions().Get(karpv1.ConditionTypeInitialized).IsUnknown()).To(BeTrue())
		}).Should(Succeed())

		// Expect that the nodeClaim is eventually de-provisioned due to the registration timeout
		Eventually(func(g Gomega) {
			g.Expect(errors.IsNotFound(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), nodeClaim))).To(BeTrue())
		}).WithTimeout(time.Minute * 20).Should(Succeed())
	})
	It("should delete a NodeClaim if it references a NodeClass that doesn't exist", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		// Don't create the NodeClass and expect that the NodeClaim fails and gets deleted
		env.ExpectCreated(nodeClaim)
		env.EventuallyExpectNotFound(nodeClaim)
	})
	It("should delete a NodeClaim if it references a NodeClass that isn't Ready", func() {
		nodeClaim := test.NodeClaim(karpv1.NodeClaim{
			Spec: karpv1.NodeClaimSpec{
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      karpv1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{karpv1.CapacityTypeOnDemand},
						},
					},
				},
				NodeClassRef: &karpv1.NodeClassReference{
					Group: object.GVK(nodeClass).Group,
					Kind:  object.GVK(nodeClass).Kind,
					Name:  nodeClass.Name,
				},
			},
		})
		// Point to an AMI that doesn't exist so that the NodeClass goes NotReady
		nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Id: "ami-123456789"}}
		env.ExpectCreated(nodeClass, nodeClaim)
		env.EventuallyExpectNotFound(nodeClaim)
	})
})
