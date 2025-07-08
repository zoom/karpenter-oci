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
	"time"

	"github.com/awslabs/operatorpkg/status"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/karpenter/pkg/test"

	. "github.com/awslabs/operatorpkg/test/expectations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subnets", func() {
	It("should use the subnet-id selector", func() {
		subnets := env.GetSubnets(nodeClass.Spec.VcnId, lo.Map(nodeClass.Spec.SubnetSelector, func(item v1alpha1.SubnetSelectorTerm, index int) string {
			return item.Name
		}))
		Expect(len(subnets)).ToNot(Equal(0))

		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{
			{
				Id: lo.FromPtr(subnets[0].Id),
			},
		}
		pod := test.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		Expect(env.GetSubnetByInstanceId(env.Monitor.CreatedNodes()[0].Spec.ProviderID)).To(Equal(lo.FromPtr(subnets[0].Id)))
	})
	It("should use the subnet selector with name", func() {
		// Get all the subnets for the cluster
		subnets := env.GetSubnets(nodeClass.Spec.VcnId, lo.Map(nodeClass.Spec.SubnetSelector, func(item v1alpha1.SubnetSelectorTerm, index int) string {
			return item.Name
		}))
		Expect(len(subnets)).To(Equal(1))
		firstSubnet := subnets[0]
		lastSubnet := subnets[len(subnets)-1]

		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{
			{
				Name: lo.FromPtr(firstSubnet.DisplayName),
			},
			{
				Name: lo.FromPtr(lastSubnet.DisplayName),
			},
		}
		pod := test.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		Expect(env.GetSubnetByInstanceId(env.Monitor.CreatedNodes()[0].Spec.ProviderID)).To(Equal(lo.FromPtr(firstSubnet.Id)))
	})

	It("should have the NodeClass status for subnets", func() {
		env.ExpectCreated(nodeClass)
		EventuallyExpectSubnets(env, nodeClass)
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: v1alpha1.ConditionTypeSubnetsReady, Status: metav1.ConditionTrue})
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: status.ConditionReady, Status: metav1.ConditionTrue})
	})
	It("should have the NodeClass status as not ready since subnets were not resolved", func() {
		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{
			{
				Name: "invalidName",
			},
		}
		env.ExpectCreated(nodeClass)
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: v1alpha1.ConditionTypeSubnetsReady, Status: metav1.ConditionFalse, Message: "SubnetSelector did not match any Subnets"})
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: status.ConditionReady, Status: metav1.ConditionFalse, Message: "SubnetsReady=False"})
	})
})

// SubnetInfo is a simple struct for testing
type SubnetInfo struct {
	Name string
	ID   string
}

func EventuallyExpectSubnets(env *oci.Environment, nodeClass *v1alpha1.OciNodeClass) {
	subnets := env.GetSubnets(nodeClass.Spec.VcnId, lo.Map(nodeClass.Spec.SubnetSelector, func(item v1alpha1.SubnetSelectorTerm, index int) string {
		return item.Name
	}))
	Expect(subnets).ToNot(HaveLen(0))
	ids := sets.New(lo.Map(subnets, func(item core.Subnet, index int) string {
		return lo.FromPtr(item.Id)
	})...)

	Eventually(func(g Gomega) {
		temp := &v1alpha1.OciNodeClass{}
		g.Expect(env.Client.Get(env, client.ObjectKeyFromObject(nodeClass), temp)).To(Succeed())
		g.Expect(sets.New(lo.Map(temp.Status.Subnets, func(s *v1alpha1.Subnet, _ int) string {
			return s.Id
		})...).Equal(ids))
	}).WithTimeout(10 * time.Second).Should(Succeed())
}
