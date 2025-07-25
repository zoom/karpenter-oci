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
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/test/pkg/debug"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	"sigs.k8s.io/karpenter/pkg/test"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/samber/lo"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Utilization", Label(debug.NoWatch), Label(debug.NoEvents), func() {
	It("should provision one pod per node", func() {
		test.ReplaceRequirements(nodePool,
			karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"VM.Standard.E4.Flex"},
				},
			},
			karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      v1alpha1.LabelInstanceCPU,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"2"},
				},
			},
			karpv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      v1alpha1.LabelInstanceShapeName,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		)
		deployment := test.Deployment(test.DeploymentOptions{
			Replicas: 10,
			PodOptions: test.PodOptions{
				ResourceRequirements: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: func() resource.Quantity {
							dsOverhead := env.GetDaemonSetOverhead(nodePool)
							base := lo.ToPtr(resource.MustParse("1700m"))
							base.Sub(*dsOverhead.Cpu())
							return *base
						}(),
					},
				},
			},
		})

		env.ExpectCreated(nodeClass, nodePool, deployment)
		env.EventuallyExpectHealthyPodCountWithTimeout(time.Minute*10, labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels), int(*deployment.Spec.Replicas))
		env.ExpectCreatedNodeCount("==", int(*deployment.Spec.Replicas)) // One pod per node enforced by instance size
	})
})
