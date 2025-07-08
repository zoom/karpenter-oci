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
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("KubeletConfiguration Overrides", func() {
	Context("All kubelet configuration set", func() {
		BeforeEach(func() {
			// MaxPods needs to account for the daemonsets that will run on the nodes
			nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
				MaxPods:     lo.ToPtr(int32(110)),
				PodsPerCore: lo.ToPtr(int32(10)),
				SystemReserved: map[string]string{
					string(corev1.ResourceCPU):              "200m",
					string(corev1.ResourceMemory):           "200Mi",
					string(corev1.ResourceEphemeralStorage): "1Gi",
				},
				KubeReserved: map[string]string{
					string(corev1.ResourceCPU):              "200m",
					string(corev1.ResourceMemory):           "200Mi",
					string(corev1.ResourceEphemeralStorage): "1Gi",
				},
				EvictionHard: map[string]string{
					"memory.available":   "5%",
					"nodefs.available":   "5%",
					"nodefs.inodesFree":  "5%",
					"imagefs.available":  "5%",
					"imagefs.inodesFree": "5%",
					"pid.available":      "3%",
				},
				EvictionSoft: map[string]string{
					"memory.available":   "10%",
					"nodefs.available":   "10%",
					"nodefs.inodesFree":  "10%",
					"imagefs.available":  "10%",
					"imagefs.inodesFree": "10%",
					"pid.available":      "6%",
				},
				EvictionSoftGracePeriod: map[string]metav1.Duration{
					"memory.available":   {Duration: time.Minute * 2},
					"nodefs.available":   {Duration: time.Minute * 2},
					"nodefs.inodesFree":  {Duration: time.Minute * 2},
					"imagefs.available":  {Duration: time.Minute * 2},
					"imagefs.inodesFree": {Duration: time.Minute * 2},
					"pid.available":      {Duration: time.Minute * 2},
				},
				EvictionMaxPodGracePeriod:   lo.ToPtr(int32(120)),
				ImageGCHighThresholdPercent: lo.ToPtr(int32(50)),
				ImageGCLowThresholdPercent:  lo.ToPtr(int32(10)),
				CPUCFSQuota:                 lo.ToPtr(false),
			}
			// todo disable kube-dns-autoscaler
		})
	})
	// TODO  fixme
	//It("should schedule pods onto separate nodes when maxPods is set", func() {
	//	// Get the DS pod count and use it to calculate the DS pod overhead
	//	// add one count for proxymux-client, it has the node selector node.info.ds_proxymux_client=true
	//	dsCount := env.GetDaemonSetCount(nodePool) + 2
	//	nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
	//		MaxPods: lo.ToPtr(1 + int32(dsCount)),
	//	}
	//
	//	numPods := 3
	//	dep := test.Deployment(test.DeploymentOptions{
	//		Replicas: int32(numPods),
	//		PodOptions: test.PodOptions{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Labels: map[string]string{"app": "large-app"},
	//			},
	//			ResourceRequirements: corev1.ResourceRequirements{
	//				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	//			},
	//		},
	//	})
	//	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
	//	env.ExpectCreated(nodeClass, nodePool, dep)
	//
	//	env.EventuallyExpectHealthyPodCount(selector, numPods)
	//	env.ExpectCreatedNodeCount("==", 3)
	//	env.EventuallyExpectUniqueNodeNames(selector, 3)
	//})
	//It("should schedule pods onto separate nodes when podsPerCore is set", func() {
	//	// PodsPerCore needs to account for the daemonsets that will run on the nodes
	//	// This will have 4 pods available on each node (2 taken by daemonset pods)
	//	test.ReplaceRequirements(nodePool,
	//		karpv1.NodeSelectorRequirementWithMinValues{
	//			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
	//				Key:      v1alpha1.LabelInstanceCPU,
	//				Operator: corev1.NodeSelectorOpIn,
	//				Values:   []string{"2"},
	//			},
	//		},
	//	)
	//	numPods := 4
	//	dep := test.Deployment(test.DeploymentOptions{
	//		Replicas: int32(numPods),
	//		PodOptions: test.PodOptions{
	//			ObjectMeta: metav1.ObjectMeta{
	//				Labels: map[string]string{"app": "large-app"},
	//			},
	//			ResourceRequirements: corev1.ResourceRequirements{
	//				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	//			},
	//		},
	//	})
	//	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
	//
	//	// Get the DS pod count and use it to calculate the DS pod overhead
	//	// We calculate podsPerCore to split the test pods and the DS pods between two nodes:
	//	//   1. If # of DS pods is odd, we will have i.e. ceil((3+2)/2) = 3
	//	//      Since we restrict node to two cores, we will allow 6 pods. One node will have 3
	//	//      DS pods and 3 test pods. Other node will have 1 test pod and 3 DS pods
	//	//   2. If # of DS pods is even, we will have i.e. ceil((4+2)/2) = 3
	//	//      Since we restrict node to two cores, we will allow 6 pods. Both nodes will have
	//	//      4 DS pods and 2 test pods.
	//
	//	// add one count for proxymux-client, it has the node selector node.info.ds_proxymux_client=true
	//	dsCount := env.GetDaemonSetCount(nodePool) + 2
	//	nodeClass.Spec.Kubelet = &v1alpha1.KubeletConfiguration{
	//		PodsPerCore: lo.ToPtr(int32(math.Ceil(float64(2+dsCount) / 2))),
	//	}
	//
	//	env.ExpectCreated(nodeClass, nodePool, dep)
	//	env.EventuallyExpectHealthyPodCount(selector, numPods)
	//	env.ExpectCreatedNodeCount("==", 2)
	//	env.EventuallyExpectUniqueNodeNames(selector, 2)
	//})
})
