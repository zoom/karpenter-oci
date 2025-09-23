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

package drift_test

import (
	"fmt"
	"github.com/awslabs/operatorpkg/object"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	envcommon "github.com/zoom/karpenter-oci/test/pkg/environment/common"
	"github.com/zoom/karpenter-oci/test/pkg/environment/oci"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var env *oci.Environment
var nodeClass *v1alpha1.OciNodeClass
var nodePool *karpv1.NodePool

var image = "Oracle-Linux-8.10-2024.11.30-0-OKE-1.30.1-754"
var imageId = "ocid1.image.oc1.iad.aaaaaaaau3ahhbqeyyfikf27szllwurv7k2w6yo3ffwupmpk4sm6korpq7ra"
var imageCompId = "ocid1.compartment.oc1..aaaaaaaab4u67dhgtj5gpdpp3z42xqqsdnufxkatoild46u3hb67vzojfmzq"

var oldImage = "Oracle-Linux-8.10-2024.11.30-0-OKE-1.29.10-754"
var oldImageid = "ocid1.image.oc1.iad.aaaaaaaa6jyk6tlb6khxaevvacq2yge62hg3wy4jgeji7vyy4ntd5jqswoba"

var oracleImageId = "ocid1.image.oc1.iad.aaaaaaaaqgq6vyph4rlxoqijrb3b5s5gryrrirg7igwm5n3nzuhi34s3x6lq"

func TestDrift(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = oci.NewEnvironment(t)
	})
	AfterSuite(func() {
		env.Stop()
	})
	RunSpecs(t, "Drift")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultOciNodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("Drift", func() {
	var dep *appsv1.Deployment
	var selector labels.Selector
	var numPods int
	BeforeEach(func() {
		numPods = 1
		//Add pods with a do-not-disrupt annotation so that we can check node metadata before we disrupt
		dep = coretest.Deployment(coretest.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: coretest.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-app",
					},
					Annotations: map[string]string{
						karpv1.DoNotDisruptAnnotationKey: "true",
					},
				},
				TerminationGracePeriodSeconds: lo.ToPtr[int64](0),
			},
		})
		selector = labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

		nodeClass.Spec.UserData = nil
		nodeClass.Spec.AgentList = []string{"Bastion"}

	})
	Context("Budgets", func() {
		It("should respect budgets for empty drift", func() {
			nodePool = coretest.ReplaceRequirements(nodePool,
				karpv1.NodeSelectorRequirementWithMinValues{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      v1alpha1.LabelInstanceCPU,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"8"},
					},
				},
			)
			// We're expecting to create 3 nodes, so we'll expect to see 2 nodes deleting at one time.
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes: "50%",
			}}
			var numPods int32 = 6
			dep = coretest.Deployment(coretest.DeploymentOptions{
				Replicas: numPods,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							karpv1.DoNotDisruptAnnotationKey: "true",
						},
						Labels: map[string]string{"app": "large-app"},
					},
					// Each 2xlarge has 8 cpu, so each node should fit 2 pods.
					ResourceRequirements: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("3"),
						},
					},
				},
			})
			selector = labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)

			nodeClass.Spec.UserData = nil
			nodeClass.Spec.AgentList = []string{"Bastion"}

			env.ExpectCreated(nodeClass, nodePool, dep)

			nodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", 3)
			nodes := env.EventuallyExpectCreatedNodeCount("==", 3)
			env.EventuallyExpectHealthyPodCount(selector, int(numPods))

			// List nodes so that we get any updated information on the nodes. If we don't
			// we have the potential to over-write any changes Karpenter makes to the nodes.
			// Add a finalizer to each node so that we can stop termination disruptions
			By("adding finalizers to the nodes to prevent termination")
			for _, node := range nodes {
				Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).To(Succeed())
				node.Finalizers = append(node.Finalizers, envcommon.TestingFinalizer)
				env.ExpectUpdated(node)
			}

			By("making the nodes empty")
			// Delete the deployment to make all nodes empty.
			env.ExpectDeleted(dep)

			// Drift the nodeclaims
			By("drift the nodeclaims")
			nodePool.Spec.Template.Annotations = map[string]string{"test": "annotation"}
			env.ExpectUpdated(nodePool)

			env.EventuallyExpectDrifted(nodeClaims...)

			// Ensure that we get two nodes tainted, and they have overlap during the drift
			env.EventuallyExpectTaintedNodeCount("==", 2)
			nodes = env.ConsistentlyExpectDisruptionsWithNodeCount(2, 3, 5*time.Second)

			// Remove the finalizer from each node so that we can terminate
			for _, node := range nodes {
				Expect(env.ExpectTestingFinalizerRemoved(node)).To(Succeed())
			}

			// After the deletion timestamp is set and all pods are drained
			// the node should be gone
			env.EventuallyExpectNotFound(nodes[0], nodes[1])

			nodes = env.EventuallyExpectTaintedNodeCount("==", 1)
			Expect(env.ExpectTestingFinalizerRemoved(nodes[0])).To(Succeed())
			env.EventuallyExpectNotFound(nodes[0])
		})
		It("should respect budgets for non-empty delete drift", func() {
			nodePool = coretest.ReplaceRequirements(nodePool,
				karpv1.NodeSelectorRequirementWithMinValues{
					NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key:      v1alpha1.LabelInstanceCPU,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"8"},
					},
				},
			)
			// We're expecting to create 3 nodes, so we'll expect to see at most 2 nodes deleting at one time.
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes: "50%",
			}}
			var numPods int32 = 9
			dep = coretest.Deployment(coretest.DeploymentOptions{
				Replicas: numPods,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							karpv1.DoNotDisruptAnnotationKey: "true",
						},
						Labels: map[string]string{"app": "large-app"},
					},
					// Each 2xlarge has 8 cpu, so each node should fit no more than 3 pods.
					ResourceRequirements: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2100m"),
						},
					},
				},
			})
			selector = labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
			env.ExpectCreated(nodeClass, nodePool, dep)

			nodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", 3)
			nodes := env.EventuallyExpectCreatedNodeCount("==", 3)
			env.EventuallyExpectHealthyPodCount(selector, int(numPods))

			By("scaling down the deployment")
			// Update the deployment to a third of the replicas.
			dep.Spec.Replicas = lo.ToPtr[int32](3)
			env.ExpectUpdated(dep)

			// First expect there to be 3 pods, then try to spread the pods.
			env.EventuallyExpectHealthyPodCount(selector, 3)
			env.ForcePodsToSpread(nodes...)
			env.EventuallyExpectHealthyPodCount(selector, 3)

			By("cordoning and adding finalizer to the nodes")
			// Add a finalizer to each node so that we can stop termination disruptions
			for _, node := range nodes {
				Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).To(Succeed())
				node.Finalizers = append(node.Finalizers, envcommon.TestingFinalizer)
				env.ExpectUpdated(node)
			}

			By("drifting the nodes")
			// Drift the nodeclaims
			nodePool.Spec.Template.Annotations = map[string]string{"test": "annotation"}
			env.ExpectUpdated(nodePool)

			env.EventuallyExpectDrifted(nodeClaims...)

			By("enabling disruption by removing the do not disrupt annotation")
			pods := env.EventuallyExpectHealthyPodCount(selector, 3)
			// Remove the do-not-disrupt annotation so that the nodes are now disruptable
			for _, pod := range pods {
				delete(pod.Annotations, karpv1.DoNotDisruptAnnotationKey)
				env.ExpectUpdated(pod)
			}

			// Ensure that we get two nodes tainted, and they have overlap during the drift
			env.EventuallyExpectTaintedNodeCount("==", 2)
			nodes = env.ConsistentlyExpectDisruptionsWithNodeCount(2, 3, 30*time.Second)

			By("removing the finalizer from the nodes")
			Expect(env.ExpectTestingFinalizerRemoved(nodes[0])).To(Succeed())
			Expect(env.ExpectTestingFinalizerRemoved(nodes[1])).To(Succeed())

			// After the deletion timestamp is set and all pods are drained
			// the node should be gone
			env.EventuallyExpectNotFound(nodes[0], nodes[1])
		})
		It("should respect budgets for non-empty replace drift", func() {
			appLabels := map[string]string{"app": "large-app"}
			nodePool.Labels = appLabels
			// We're expecting to create 5 nodes, so we'll expect to see at most 3 nodes deleting at one time.
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes: "3",
			}}

			// Create a 5 pod deployment with hostname inter-pod anti-affinity to ensure each pod is placed on a unique node
			numPods = 5
			selector = labels.SelectorFromSet(appLabels)
			deployment := coretest.Deployment(coretest.DeploymentOptions{
				Replicas: int32(numPods),
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{
						Labels: appLabels,
					},
					PodAntiRequirements: []corev1.PodAffinityTerm{{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: appLabels,
						},
					}},
				},
			})

			env.ExpectCreated(nodeClass, nodePool, deployment)

			originalNodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", numPods)
			originalNodes := env.EventuallyExpectCreatedNodeCount("==", numPods)

			// Check that all deployment pods are online
			env.EventuallyExpectHealthyPodCount(selector, numPods)

			By("cordoning and adding finalizer to the nodes")
			// Add a finalizer to each node so that we can stop termination disruptions
			for _, node := range originalNodes {
				Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).To(Succeed())
				node.Finalizers = append(node.Finalizers, envcommon.TestingFinalizer)
				env.ExpectUpdated(node)
			}

			By("drifting the nodepool")
			nodePool.Spec.Template.Annotations = lo.Assign(nodePool.Spec.Template.Annotations, map[string]string{"test-annotation": "drift"})
			env.ExpectUpdated(nodePool)

			// Ensure that we get three nodes tainted, and they have overlap during the drift
			env.EventuallyExpectTaintedNodeCount("==", 3)
			env.EventuallyExpectNodeClaimCount("==", 8)
			env.EventuallyExpectNodeCount("==", 8)
			env.ConsistentlyExpectDisruptionsWithNodeCount(3, 8, 5*time.Second)

			for _, node := range originalNodes {
				Expect(env.ExpectTestingFinalizerRemoved(node)).To(Succeed())
			}

			// Eventually expect all the nodes to be rolled and completely removed
			// Since this completes the disruption operation, this also ensures that we aren't leaking nodes into subsequent
			// tests since nodeclaims that are actively replacing but haven't brought-up nodes yet can register nodes later
			env.EventuallyExpectNotFound(lo.Map(originalNodes, func(n *corev1.Node, _ int) client.Object { return n })...)
			env.EventuallyExpectNotFound(lo.Map(originalNodeClaims, func(n *karpv1.NodeClaim, _ int) client.Object { return n })...)
			env.ExpectNodeClaimCount("==", 5)
			env.ExpectNodeCount("==", 5)
		})
		It("should not allow drift if the budget is fully blocking", func() {
			// We're going to define a budget that doesn't allow any drift to happen
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes: "0",
			}}

			dep.Spec.Template.Annotations = nil
			env.ExpectCreated(nodeClass, nodePool, dep)

			nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
			env.EventuallyExpectCreatedNodeCount("==", 1)
			env.EventuallyExpectHealthyPodCount(selector, numPods)

			By("drifting the nodes")
			// Drift the nodeclaims
			nodePool.Spec.Template.Annotations = map[string]string{"test": "annotation"}
			env.ExpectUpdated(nodePool)

			env.EventuallyExpectDrifted(nodeClaim)
			env.ConsistentlyExpectNoDisruptions(1, time.Minute)
		})
		It("should not allow drift if the budget is fully blocking during a scheduled time", func() {
			// We're going to define a budget that doesn't allow any drift to happen
			// This is going to be on a schedule that only lasts 30 minutes, whose window starts 15 minutes before
			// the current time and extends 15 minutes past the current time
			// Times need to be in UTC since the karpenter containers were built in UTC time
			windowStart := time.Now().Add(-time.Minute * 15).UTC()
			nodePool.Spec.Disruption.Budgets = []karpv1.Budget{{
				Nodes:    "0",
				Schedule: lo.ToPtr(fmt.Sprintf("%d %d * * *", windowStart.Minute(), windowStart.Hour())),
				Duration: &metav1.Duration{Duration: time.Minute * 30},
			}}

			dep.Spec.Template.Annotations = nil
			env.ExpectCreated(nodeClass, nodePool, dep)

			nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
			env.EventuallyExpectCreatedNodeCount("==", 1)
			env.EventuallyExpectHealthyPodCount(selector, numPods)

			By("drifting the nodes")
			// Drift the nodeclaims
			nodePool.Spec.Template.Annotations = map[string]string{"test": "annotation"}
			env.ExpectUpdated(nodePool)

			env.EventuallyExpectDrifted(nodeClaim)
			env.ConsistentlyExpectNoDisruptions(1, time.Minute)
		})
	})
	It("should disrupt nodes that have drifted due to AMIs", func() {
		nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{
			{
				Name: oldImage,
				Id:   oldImageid,
			},
		}

		env.ExpectCreated(dep, nodeClass, nodePool)
		pod := env.EventuallyExpectHealthyPodCount(selector, numPods)[0]
		env.ExpectCreatedNodeCount("==", 1)

		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.EventuallyExpectNodeCount("==", 1)[0]
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{
			{
				Id:   imageId,
				Name: image,
			},
		}
		env.ExpectCreatedOrUpdated(nodeClass)

		env.EventuallyExpectDrifted(nodeClaim)

		delete(pod.Annotations, karpv1.DoNotDisruptAnnotationKey)
		env.ExpectUpdated(pod)
		env.EventuallyExpectNotFound(pod, nodeClaim, node)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
	})

	It("should disrupt nodes that have drifted due to securitygroup", func() {
		By("getting the cluster VCN id")
		vcnId := nodeClass.Spec.VcnId

		By("creating new security group")
		createSecurityGroupReq := core.CreateNetworkSecurityGroupRequest{
			CreateNetworkSecurityGroupDetails: core.CreateNetworkSecurityGroupDetails{
				CompartmentId: common.String(env.CompartmentId),
				VcnId:         common.String(vcnId),
				DisplayName:   common.String("security-group-drift"),
			},
		}

		createResp, err := env.VCNAPI.CreateNetworkSecurityGroup(env.Context, createSecurityGroupReq)
		Expect(err).To(BeNil())
		testSecurityGroupId := lo.FromPtr(createResp.Id)

		// Clean up the test security group after the test
		defer func() {
			deleteNSGWithRetry(env, testSecurityGroupId, 10)
		}()

		By("using the created security group")
		testSecurityGroup := createResp.NetworkSecurityGroup

		By("creating a new provider with the new securitygroup")
		// Get existing security groups (excluding the test one)
		existingSGs := env.GetSecurityGroups(vcnId, []string{nodeClass.Spec.SecurityGroupSelector[0].Name})

		// Create security group selector terms including the test security group
		sgTerms := []v1alpha1.SecurityGroupSelectorTerm{{Name: lo.FromPtr(testSecurityGroup.DisplayName)}}
		for _, sg := range existingSGs {
			//sgTerms = append(sgTerms, v1alpha1.SecurityGroupSelectorTerm{Id: lo.FromPtr(sg.Id)})
			sgTerms = append(sgTerms, v1alpha1.SecurityGroupSelectorTerm{Name: lo.FromPtr(sg.DisplayName)})
		}
		nodeClass.Spec.SecurityGroupSelector = sgTerms

		env.ExpectCreated(dep, nodeClass, nodePool)
		pod := env.EventuallyExpectHealthyPodCount(selector, numPods)[0]
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.ExpectCreatedNodeCount("==", 1)[0]

		// Remove the test security group to trigger drift
		sgTerms = lo.Reject(sgTerms, func(t v1alpha1.SecurityGroupSelectorTerm, _ int) bool {
			return t.Name == lo.FromPtr(testSecurityGroup.DisplayName)
		})
		nodeClass.Spec.SecurityGroupSelector = sgTerms
		env.ExpectCreatedOrUpdated(nodeClass)

		env.EventuallyExpectDrifted(nodeClaim)

		delete(pod.Annotations, karpv1.DoNotDisruptAnnotationKey)
		env.ExpectUpdated(pod)
		env.EventuallyExpectNotFound(pod, nodeClaim, node)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
	})
	It("should disrupt nodes that have drifted due to subnets", func() {
		By("creating test subnets with specified CIDR blocks")
		vcnId := nodeClass.Spec.VcnId

		// Create first subnet with CIDR 10.0.30.0/24
		createSubnet1Req := core.CreateSubnetRequest{
			CreateSubnetDetails: core.CreateSubnetDetails{
				CompartmentId: common.String(env.CompartmentId),
				VcnId:         common.String(vcnId),
				DisplayName:   common.String("drift-test-subnet-1"),
				CidrBlock:     common.String("10.0.30.0/24"),
			},
		}
		subnet1Resp, err := env.VCNAPI.CreateSubnet(env.Context, createSubnet1Req)
		Expect(err).ToNot(HaveOccurred())
		subnet1Id := lo.FromPtr(subnet1Resp.Id)

		// Create second subnet with CIDR 10.0.40.0/24
		createSubnet2Req := core.CreateSubnetRequest{
			CreateSubnetDetails: core.CreateSubnetDetails{
				CompartmentId: common.String(env.CompartmentId),
				VcnId:         common.String(vcnId),
				DisplayName:   common.String("drift-test-subnet-2"),
				CidrBlock:     common.String("10.0.40.0/24"),
			},
		}
		subnet2Resp, err := env.VCNAPI.CreateSubnet(env.Context, createSubnet2Req)
		Expect(err).ToNot(HaveOccurred())
		subnet2Id := lo.FromPtr(subnet2Resp.Id)

		// Schedule cleanup of created subnets
		defer func() {
			By("cleaning up created subnets")
			deleteSubnetWithRetry(env, subnet1Id, 10)
			deleteSubnetWithRetry(env, subnet2Id, 10)
		}()

		By("setting initial subnet selector")
		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{{Name: "drift-test-subnet-1"}}

		env.ExpectCreated(dep, nodeClass, nodePool)
		pod := env.EventuallyExpectHealthyPodCount(selector, numPods)[0]
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.ExpectCreatedNodeCount("==", 1)[0]

		By("updating subnet selector to trigger drift")
		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{{Name: "drift-test-subnet-2"}}
		env.ExpectCreatedOrUpdated(nodeClass)

		env.EventuallyExpectDrifted(nodeClaim)

		delete(pod.Annotations, karpv1.DoNotDisruptAnnotationKey)
		env.ExpectUpdated(pod)
		env.EventuallyExpectNotFound(pod, node)
		env.EventuallyExpectHealthyPodCount(selector, numPods)

		env.Cleanup()
	})
	DescribeTable("NodePool Drift", func(nodeClaimTemplate karpv1.NodeClaimTemplate) {
		updatedNodePool := coretest.NodePool(
			karpv1.NodePool{
				Spec: karpv1.NodePoolSpec{
					Template: karpv1.NodeClaimTemplate{
						Spec: karpv1.NodeClaimTemplateSpec{
							NodeClassRef: &karpv1.NodeClassReference{
								Group: object.GVK(nodeClass).Group,
								Kind:  object.GVK(nodeClass).Kind,
								Name:  nodeClass.Name,
							},
							// keep the same instance type requirements to prevent considering instance types that require swap
							Requirements: nodePool.Spec.Template.Spec.Requirements,
						},
					},
				},
			},
			karpv1.NodePool{
				Spec: karpv1.NodePoolSpec{
					Template: nodeClaimTemplate,
				},
			},
		)
		updatedNodePool.ObjectMeta = nodePool.ObjectMeta

		env.ExpectCreated(dep, nodeClass, nodePool)
		pod := env.EventuallyExpectHealthyPodCount(selector, numPods)[0]
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.ExpectCreatedNodeCount("==", 1)[0]

		env.ExpectCreatedOrUpdated(updatedNodePool)

		env.EventuallyExpectDrifted(nodeClaim)

		delete(pod.Annotations, karpv1.DoNotDisruptAnnotationKey)
		env.ExpectUpdated(pod)

		// Nodes will need to have the start-up taint removed before the node can be considered as initialized
		fmt.Println(CurrentSpecReport().LeafNodeText)
		if CurrentSpecReport().LeafNodeText == "Start-up Taints" {
			nodes := env.EventuallyExpectCreatedNodeCount("==", 2)
			sort.Slice(nodes, func(i int, j int) bool {
				return nodes[i].CreationTimestamp.Before(&nodes[j].CreationTimestamp)
			})
			nodeTwo := nodes[1]
			// Remove the startup taints from the new nodes to initialize them
			Eventually(func(g Gomega) {
				g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeTwo), nodeTwo)).To(Succeed())
				g.Expect(len(nodeTwo.Spec.Taints)).To(BeNumerically("==", 1))
				_, found := lo.Find(nodeTwo.Spec.Taints, func(t corev1.Taint) bool {
					return t.MatchTaint(&corev1.Taint{Key: "example.com/another-taint-2", Effect: corev1.TaintEffectPreferNoSchedule})
				})
				g.Expect(found).To(BeTrue())
				stored := nodeTwo.DeepCopy()
				nodeTwo.Spec.Taints = lo.Reject(nodeTwo.Spec.Taints, func(t corev1.Taint, _ int) bool { return t.Key == "example.com/another-taint-2" })
				g.Expect(env.Client.Patch(env.Context, nodeTwo, client.StrategicMergeFrom(stored))).To(Succeed())
			}).Should(Succeed())
		}
		env.EventuallyExpectNotFound(pod, node)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
	},
		Entry("Annotations", karpv1.NodeClaimTemplate{
			ObjectMeta: karpv1.ObjectMeta{
				Annotations: map[string]string{"keyAnnotationTest": "valueAnnotationTest"},
			},
		}),
		Entry("Labels", karpv1.NodeClaimTemplate{
			ObjectMeta: karpv1.ObjectMeta{
				Labels: map[string]string{"keyLabelTest": "valueLabelTest"},
			},
		}),
		Entry("Taints", karpv1.NodeClaimTemplate{
			Spec: karpv1.NodeClaimTemplateSpec{
				Taints: []corev1.Taint{{Key: "example.com/another-taint-2", Effect: corev1.TaintEffectPreferNoSchedule}},
			},
		}),
		Entry("Start-up Taints", karpv1.NodeClaimTemplate{
			Spec: karpv1.NodeClaimTemplateSpec{
				StartupTaints: []corev1.Taint{{Key: "example.com/another-taint-2", Effect: corev1.TaintEffectPreferNoSchedule}},
			},
		}),
		Entry("NodeRequirements", karpv1.NodeClaimTemplate{
			Spec: karpv1.NodeClaimTemplateSpec{
				// since this will overwrite the default requirements, add instance category and family selectors back into requirements
				Requirements: []karpv1.NodeSelectorRequirementWithMinValues{
					//{NodeSelectorRequirement: corev1.NodeSelectorRequirement{Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeSpot}}},
					{NodeSelectorRequirement: corev1.NodeSelectorRequirement{
						Key: v1alpha1.LabelInstanceCPU, Operator: corev1.NodeSelectorOpNotIn, Values: []string{"2"},
					}},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      v1alpha1.LabelInstanceShapeName,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"VM.Standard.E4.Flex", "VM.Standard.E2.2", "VM.Standard.E2.4"},
						},
					},
				},
			},
		}),
	)

	It("should update the nodepool-hash annotation on the nodepool and nodeclaim when the nodepool's nodepool-hash-version annotation does not match the controller hash version", func() {
		env.ExpectCreated(dep, nodeClass, nodePool)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		nodePool = env.ExpectExists(nodePool).(*karpv1.NodePool)
		expectedHash := nodePool.Hash()

		By(fmt.Sprintf("expect nodepool %s and nodeclaim %s to contain %s and %s annotations", nodePool.Name, nodeClaim.Name, karpv1.NodePoolHashAnnotationKey, karpv1.NodePoolHashVersionAnnotationKey))
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodePool), nodePool)).To(Succeed())
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), nodeClaim)).To(Succeed())

			g.Expect(nodePool.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashAnnotationKey, expectedHash))
			g.Expect(nodePool.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashVersionAnnotationKey, karpv1.NodePoolHashVersion))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashAnnotationKey, expectedHash))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashVersionAnnotationKey, karpv1.NodePoolHashVersion))
		}).WithTimeout(30 * time.Second).Should(Succeed())

		nodePool.Annotations = lo.Assign(nodePool.Annotations, map[string]string{
			karpv1.NodePoolHashAnnotationKey:        "test-hash-1",
			karpv1.NodePoolHashVersionAnnotationKey: "test-hash-version-1",
		})
		// Updating `nodePool.Spec.Template.Annotations` would normally trigger drift on all nodeclaims owned by the
		// nodepool. However, the nodepool-hash-version does not match the controller hash version, so we will see that
		// none of the nodeclaims will be drifted and all nodeclaims will have an updated `nodepool-hash` and `nodepool-hash-version` annotation
		nodePool.Spec.Template.Annotations = lo.Assign(nodePool.Spec.Template.Annotations, map[string]string{
			"test-key": "test-value",
		})
		nodeClaim.Annotations = lo.Assign(nodePool.Annotations, map[string]string{
			karpv1.NodePoolHashAnnotationKey:        "test-hash-2",
			karpv1.NodePoolHashVersionAnnotationKey: "test-hash-version-2",
		})

		// The nodeclaim will need to be updated first, as the hash controller will only be triggered on changes to the nodepool
		env.ExpectUpdated(nodeClaim, nodePool)
		expectedHash = nodePool.Hash()

		// Expect all nodeclaims not to be drifted and contain an updated `nodepool-hash` and `nodepool-hash-version` annotation
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodePool), nodePool)).To(Succeed())
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), nodeClaim)).To(Succeed())

			g.Expect(nodePool.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashAnnotationKey, expectedHash))
			g.Expect(nodePool.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashVersionAnnotationKey, karpv1.NodePoolHashVersion))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashAnnotationKey, expectedHash))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashVersionAnnotationKey, karpv1.NodePoolHashVersion))
		})
	})
	It("should update the ocinodeclass-hash annotation on the ocinodeclass and nodeclaim when the ocinodeclass's ocinodeclass-hash-version annotation does not match the controller hash version", func() {
		env.ExpectCreated(dep, nodeClass, nodePool)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		nodeClass = env.ExpectExists(nodeClass).(*v1alpha1.OciNodeClass)
		expectedHash := nodeClass.Hash()

		By(fmt.Sprintf("expect nodeclass %s and nodeclaim %s to contain %s and %s annotations", nodeClass.Name, nodeClaim.Name, v1alpha1.AnnotationOciNodeClassHash, v1alpha1.AnnotationOciNodeClassHashVersion))
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClass), nodeClass)).To(Succeed())
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), nodeClaim)).To(Succeed())

			g.Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, expectedHash))
			g.Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, expectedHash))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
		}).WithTimeout(60 * time.Second).Should(Succeed())

		nodeClass.Annotations = lo.Assign(nodeClass.Annotations, map[string]string{
			v1alpha1.AnnotationOciNodeClassHash:        "test-hash-1",
			v1alpha1.AnnotationOciNodeClassHashVersion: "test-hash-version-1",
		})
		// Updating `nodeClass.Spec.DefinedTags` would normally trigger drift on all nodeclaims using the
		// nodeclass. However, the ocinodeclass-hash-version does not match the controller hash version, so we will see that
		// none of the nodeclaims will be drifted and all nodeclaims will have an updated `ocinodeclass-hash` and `ocinodeclass-hash-version` annotation
		nodeClass.Spec.DefinedTags = lo.Assign(nodeClass.Spec.DefinedTags, map[string]v1alpha1.DefinedTagValue{env.TagNamespace: {
			"test-key": "test-value",
		}})
		nodeClaim.Annotations = lo.Assign(nodePool.Annotations, map[string]string{
			v1alpha1.AnnotationOciNodeClassHash:        "test-hash-2",
			v1alpha1.AnnotationOciNodeClassHashVersion: "test-hash-version-2",
		})

		// The nodeclaim will need to be updated first, as the hash controller will only be triggered on changes to the nodeclass
		env.ExpectUpdated(nodeClaim, nodeClass)
		expectedHash = nodeClass.Hash()

		// Expect all nodeclaims not to be drifted and contain an updated `ocinodeclass-hash` and `ocinodeclass-hash-version` annotation
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClass), nodeClass)).To(Succeed())
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(nodeClaim), nodeClaim)).To(Succeed())

			g.Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, expectedHash))
			g.Expect(nodeClass.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHash, expectedHash))
			g.Expect(nodeClaim.Annotations).To(HaveKeyWithValue(v1alpha1.AnnotationOciNodeClassHashVersion, v1alpha1.OciNodeClassHashVersion))
		}).WithTimeout(60 * time.Second).Should(Succeed())
		env.ConsistentlyExpectNodeClaimsNotDrifted(time.Minute, nodeClaim)
	})
	Context("Failure", func() {
		It("should not disrupt a drifted node if the replacement node never registers", func() {
			// launch a new nodeClaim
			var numPods int32 = 2
			dep := coretest.Deployment(coretest.DeploymentOptions{
				Replicas: 2,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "inflate"}},
					PodAntiRequirements: []corev1.PodAffinityTerm{{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "inflate"},
						}},
					},
				},
			})
			env.ExpectCreated(dep, nodeClass, nodePool)

			startingNodeClaimState := env.EventuallyExpectCreatedNodeClaimCount("==", int(numPods))
			env.EventuallyExpectCreatedNodeCount("==", int(numPods))

			// Drift the nodeClaim with bad configuration that will not register a NodeClaim
			nodeClass.Spec.ImageFamily = v1alpha1.CustomImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{
				Id:            oracleImageId,
				CompartmentId: imageCompId,
			}}
			env.ExpectCreatedOrUpdated(nodeClass)

			env.EventuallyExpectDrifted(startingNodeClaimState...)

			// Expect only a single node to be tainted due to default disruption budgets
			taintedNodes := env.EventuallyExpectTaintedNodeCount("==", 1)

			// Drift should fail and the original node should be untainted
			// TODO: reduce timeouts when disruption waits are factored out
			env.EventuallyExpectNodesUntaintedWithTimeout(11*time.Minute, taintedNodes...)

			// Expect all the NodeClaims that existed on the initial provisioning loop are not removed.
			// Assert this over several minutes to ensure a subsequent disruption controller pass doesn't
			// successfully schedule the evicted pods to the in-flight nodeclaim and disrupt the original node
			Consistently(func(g Gomega) {
				nodeClaims := &karpv1.NodeClaimList{}
				g.Expect(env.Client.List(env, nodeClaims, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
				startingNodeClaimUIDs := sets.New(lo.Map(startingNodeClaimState, func(nc *karpv1.NodeClaim, _ int) types.UID { return nc.UID })...)
				nodeClaimUIDs := sets.New(lo.Map(nodeClaims.Items, func(nc karpv1.NodeClaim, _ int) types.UID { return nc.UID })...)
				g.Expect(nodeClaimUIDs.IsSuperset(startingNodeClaimUIDs)).To(BeTrue())
			}, "2m").Should(Succeed())

		})
		It("should not disrupt a drifted node if the replacement node registers but never initialized", func() {
			// launch a new nodeClaim
			var numPods int32 = 2
			dep := coretest.Deployment(coretest.DeploymentOptions{
				Replicas: 2,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "inflate"}},
					PodAntiRequirements: []corev1.PodAffinityTerm{{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "inflate"},
						}},
					},
				},
			})
			env.ExpectCreated(dep, nodeClass, nodePool)

			startingNodeClaimState := env.EventuallyExpectCreatedNodeClaimCount("==", int(numPods))
			env.EventuallyExpectCreatedNodeCount("==", int(numPods))

			// Drift the nodeClaim with bad configuration that never initializes
			nodePool.Spec.Template.Spec.StartupTaints = []corev1.Taint{{Key: "example.com/taint", Effect: corev1.TaintEffectPreferNoSchedule}}
			env.ExpectCreatedOrUpdated(nodePool)
			env.EventuallyExpectDrifted(startingNodeClaimState...)

			// Expect only a single node to get tainted due to default disruption budgets
			taintedNodes := env.EventuallyExpectTaintedNodeCount("==", 1)

			// Drift should fail and original node should be untainted
			// TODO: reduce timeouts when disruption waits are factored out
			env.EventuallyExpectNodesUntaintedWithTimeout(11*time.Minute, taintedNodes...)

			// Expect that the new nodeClaim/node is kept around after the un-cordon
			nodeList := &corev1.NodeList{}
			Expect(env.Client.List(env, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
			Expect(nodeList.Items).To(HaveLen(int(numPods) + 1))

			nodeClaimList := &karpv1.NodeClaimList{}
			Expect(env.Client.List(env, nodeClaimList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
			Expect(nodeClaimList.Items).To(HaveLen(int(numPods) + 1))

			// Expect all the NodeClaims that existed on the initial provisioning loop are not removed
			// Assert this over several minutes to ensure a subsequent disruption controller pass doesn't
			// successfully schedule the evicted pods to the in-flight nodeclaim and disrupt the original node
			Consistently(func(g Gomega) {
				nodeClaims := &karpv1.NodeClaimList{}
				g.Expect(env.Client.List(env, nodeClaims, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
				startingNodeClaimUIDs := sets.New(lo.Map(startingNodeClaimState, func(m *karpv1.NodeClaim, _ int) types.UID { return m.UID })...)
				nodeClaimUIDs := sets.New(lo.Map(nodeClaims.Items, func(m karpv1.NodeClaim, _ int) types.UID { return m.UID })...)
				g.Expect(nodeClaimUIDs.IsSuperset(startingNodeClaimUIDs)).To(BeTrue())
			}, "2m").Should(Succeed())

		})
		It("should not drift any nodes if their PodDisruptionBudgets are unhealthy", func() {
			// Create a deployment that contains a readiness probe that will never succeed
			// This way, the pod will bind to the node, but the PodDisruptionBudget will never go healthy
			var numPods int32 = 2
			dep := coretest.Deployment(coretest.DeploymentOptions{
				Replicas: 2,
				PodOptions: coretest.PodOptions{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "inflate"}},
					PodAntiRequirements: []corev1.PodAffinityTerm{{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "inflate"},
						}},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Port: intstr.FromInt32(80),
							},
						},
					},
				},
			})
			selectorFromSet := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
			minAvailable := intstr.FromInt32(numPods - 1)
			pdb := coretest.PodDisruptionBudget(coretest.PDBOptions{
				Labels:       dep.Spec.Template.Labels,
				MinAvailable: &minAvailable,
			})
			env.ExpectCreated(dep, nodeClass, nodePool, pdb)

			nodeClaims := env.EventuallyExpectCreatedNodeClaimCount("==", int(numPods))
			env.EventuallyExpectCreatedNodeCount("==", int(numPods))

			// Expect pods to be bound but not to be ready since we are intentionally failing the readiness check
			env.EventuallyExpectBoundPodCount(selectorFromSet, int(numPods))

			// Drift the nodeclaims
			nodePool.Spec.Template.Annotations = map[string]string{"test": "annotation"}
			env.ExpectUpdated(nodePool)

			env.EventuallyExpectDrifted(nodeClaims...)
			env.ConsistentlyExpectNoDisruptions(int(numPods), time.Minute)

		})
	})
})

// deleteNSGWithRetry attempts to delete a Network Security Group with exponential backoff
// This is needed because NSGs cannot be deleted while VNICs are still attached
func deleteNSGWithRetry(env *oci.Environment, nsgId string, maxRetries int) {
	backoffDuration := time.Second
	maxBackoff := 60 * time.Second

	for i := 0; i < maxRetries; i++ {
		deleteReq := core.DeleteNetworkSecurityGroupRequest{
			NetworkSecurityGroupId: common.String(nsgId),
		}
		_, err := env.VCNAPI.DeleteNetworkSecurityGroup(env.Context, deleteReq)
		if err == nil {
			// Successfully deleted
			return
		}

		// Check if the error is about VNICs still being attached
		if i < maxRetries-1 { // Don't sleep on the last iteration
			time.Sleep(backoffDuration)
			backoffDuration *= 2
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}
		}
	}
	// If we get here, we failed to delete after all retries
	// Log the error but don't fail the test since this is cleanup
	GinkgoWriter.Printf("Warning: Failed to delete NSG %s after %d retries\n", nsgId, maxRetries)
}

// deleteSubnetWithRetry attempts to delete a Subnet with exponential backoff
// This is needed because subnets cannot be deleted while resources are still attached
func deleteSubnetWithRetry(env *oci.Environment, subnetId string, maxRetries int) {
	backoffDuration := time.Second
	maxBackoff := 60 * time.Second

	for i := 0; i < maxRetries; i++ {
		deleteReq := core.DeleteSubnetRequest{
			SubnetId: common.String(subnetId),
		}
		_, err := env.VCNAPI.DeleteSubnet(env.Context, deleteReq)
		if err == nil {
			// Successfully deleted
			return
		}

		// Check if the error is about resources still being attached
		if i < maxRetries-1 { // Don't sleep on the last iteration
			time.Sleep(backoffDuration)
			backoffDuration *= 2
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}
		}
	}
	// If we get here, we failed to delete after all retries
	// Log the error but don't fail the test since this is cleanup
	GinkgoWriter.Printf("Warning: Failed to delete subnet %s after %d retries\n", subnetId, maxRetries)
}
