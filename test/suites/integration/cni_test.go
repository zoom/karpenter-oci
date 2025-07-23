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
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/test/pkg/environment/oci"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/karpenter/pkg/test"
)

var _ = Describe("CNITests", func() {
	It("should set eni-limited maxPods", func() {
		pod := test.Pod()
		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
		var node corev1.Node
		Expect(env.Client.Get(env.Context, types.NamespacedName{Name: pod.Spec.NodeName}, &node)).To(Succeed())
		allocatablePods, _ := node.Status.Allocatable.Pods().AsInt64()
		vcpu, err := strconv.Atoi(node.Labels["karpenter.k8s.oracle/instance-cpu"])
		Expect(err).ToNot(HaveOccurred())
		Expect(allocatablePods).To(Equal(eniLimitedPodsFor(env, node.Labels["node.kubernetes.io/instance-type"], vcpu/2)))
	})
	It("should set max pods to 110 if maxPods is set in kubelet", func() {
		nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{MaxPods: lo.ToPtr[int32](110)}
		pod := test.Pod()
		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		var node corev1.Node
		Expect(env.Client.Get(env.Context, types.NamespacedName{Name: pod.Spec.NodeName}, &node)).To(Succeed())
		allocatablePods, _ := node.Status.Allocatable.Pods().AsInt64()
		Expect(allocatablePods).To(Equal(int64(110)))
	})
})

func eniLimitedPodsFor(env *oci.Environment, instanceType string, ocpu int) int64 {
	shapes, err := env.CMPAPI.ListShapes(env.Context, core.ListShapesRequest{
		CompartmentId:      lo.ToPtr(env.CompartmentId),
		AvailabilityDomain: lo.ToPtr(env.AvailableDomainInfo[0]),
	})
	Expect(err).ToNot(HaveOccurred())
	shape, find := lo.Find(shapes.Items, func(item core.Shape) bool {
		return lo.FromPtr(item.Shape) == instanceType
	})
	Expect(find).To(BeTrue())
	var calMaxVnic int64
	if shape.MaxVnicAttachmentOptions != nil && shape.MaxVnicAttachmentOptions.DefaultPerOcpu != nil {
		if ocpu == 1 {
			calMaxVnic = 2
		} else {
			calMaxVnic = int64(*shape.MaxVnicAttachmentOptions.DefaultPerOcpu) * int64(ocpu)
		}
		calMaxVnic = min(24, calMaxVnic)
	} else {
		calMaxVnic = int64(*shape.MaxVnicAttachments)
	}

	return (calMaxVnic - 1) * 31
}
