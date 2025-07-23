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

package integration_test

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validation", func() {
	Context("NodePool", func() {
		It("should error when a restricted label is used in labels (karpenter.sh/nodepool)", func() {
			nodePool.Spec.Template.Labels = map[string]string{
				karpv1.NodePoolLabelKey: "my-custom-nodepool",
			}
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when a restricted label is used in labels (kubernetes.io/custom-label)", func() {
			nodePool.Spec.Template.Labels = map[string]string{
				"kubernetes.io/custom-label": "custom-value",
			}
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should allow a restricted label exception to be used in labels (node-restriction.kubernetes.io/custom-label)", func() {
			nodePool.Spec.Template.Labels = map[string]string{
				corev1.LabelNamespaceNodeRestriction + "/custom-label": "custom-value",
			}
			Expect(env.Client.Create(env.Context, nodePool)).To(Succeed())
		})
		It("should allow a restricted label exception to be used in labels ([*].node-restriction.kubernetes.io/custom-label)", func() {
			nodePool.Spec.Template.Labels = map[string]string{
				"subdomain" + corev1.LabelNamespaceNodeRestriction + "/custom-label": "custom-value",
			}
			Expect(env.Client.Create(env.Context, nodePool)).To(Succeed())
		})
		It("should error when a requirement references a restricted label (karpenter.sh/nodepool)", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      karpv1.NodePoolLabelKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"default"},
				}})
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when a requirement uses In but has no values", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{},
				}})
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when a requirement uses an unknown operator", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      karpv1.CapacityTypeLabelKey,
					Operator: "within",
					Values:   []string{karpv1.CapacityTypeSpot},
				}})
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when Gt is used with multiple integer values", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      v1alpha1.LabelInstanceMemory,
					Operator: corev1.NodeSelectorOpGt,
					Values:   []string{"1000000", "2000000"},
				}})
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when Lt is used with multiple integer values", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      v1alpha1.LabelInstanceMemory,
					Operator: corev1.NodeSelectorOpLt,
					Values:   []string{"1000000", "2000000"},
				}})
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when consolidateAfter is negative", func() {
			nodePool.Spec.Disruption.ConsolidationPolicy = karpv1.ConsolidationPolicyWhenEmpty
			nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("-1s")
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should succeed when ConsolidationPolicy=WhenEmptyOrUnderutilized is used with consolidateAfter", func() {
			nodePool.Spec.Disruption.ConsolidationPolicy = karpv1.ConsolidationPolicyWhenEmptyOrUnderutilized
			nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("1m")
			Expect(env.Client.Create(env.Context, nodePool)).To(Succeed())
		})
		It("should error when minValues for a requirement key is negative", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"insance-type-1", "insance-type-2"},
				},
				MinValues: lo.ToPtr(-1)},
			)
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when minValues for a requirement key is zero", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"insance-type-1", "insance-type-2"},
				},
				MinValues: lo.ToPtr(0)},
			)
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when minValues for a requirement key is more than 50", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"insance-type-1", "insance-type-2"},
				},
				MinValues: lo.ToPtr(51)},
			)
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
		It("should error when minValues for a requirement key is greater than the values specified within In operator", func() {
			nodePool = coretest.ReplaceRequirements(nodePool, karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"insance-type-1", "insance-type-2"},
				},
				MinValues: lo.ToPtr(3)},
			)
			Expect(env.Client.Create(env.Context, nodePool)).ToNot(Succeed())
		})
	})
	Context("OciNodeClass", func() {
		It("should error when imageSelectorTerms are not defined", func() {
			nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should fail for poorly formatted image ids", func() {
			nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{
				{
					Id: "must-start-with-ocid1.image",
				},
			}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should succeed when tags don't contain restricted keys", func() {
			nodeClass.Spec.Tags = map[string]string{"karpenter.sh/custom-key": "custom-value", "kubernetes.io/role/key": "custom-value"}
			Expect(env.Client.Create(env.Context, nodeClass)).To(Succeed())
		})
		It("should error when tags contains a restricted key", func() {
			nodeClass.Spec.Tags = map[string]string{"karpenter.sh/nodepool": "custom-value"}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())

			nodeClass.Spec.Tags = map[string]string{"karpenter.sh/managed-by": env.ClusterName}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())

			nodeClass.Spec.Tags = map[string]string{fmt.Sprintf("kubernetes.io/cluster/%s", env.ClusterName): "owned"}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())

			nodeClass.Spec.Tags = map[string]string{"karpenter.sh/nodeclaim": "custom-value"}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())

			nodeClass.Spec.Tags = map[string]string{"karpenter.k8s.oracle/ocinodeclass": "custom-value"}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should fail when securityGroupSelectorTerms has id and other filters", func() {
			nodeClass.Spec.SecurityGroupSelector = []v1alpha1.SecurityGroupSelectorTerm{
				{
					Name: env.ClusterName,
					Id:   "sg-12345",
				},
			}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should fail when subnetSelectorTerms has id and other filters", func() {
			nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{
				{
					Name: env.ClusterName,
					Id:   "subnet-12345",
				},
			}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should fail when imageSelectorTerms has id and other filters", func() {
			nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{
				{
					Name: env.ClusterName,
					Id:   "ocid1.image.12345",
				},
			}
			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
		It("should fail when attach volume size out of range", func() {
			nodeClass.Spec.BlockDevices = []*v1alpha1.VolumeAttributes{{SizeInGBs: 20, VpusPerGB: 30}}

			Expect(env.Client.Create(env.Context, nodeClass)).ToNot(Succeed())
		})
	})
})
