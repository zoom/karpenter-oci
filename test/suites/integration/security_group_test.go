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
	"github.com/awslabs/operatorpkg/status"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/karpenter/pkg/test"

	"github.com/zoom/karpenter-oci/test/pkg/environment/oci"

	. "github.com/awslabs/operatorpkg/test/expectations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SecurityGroups", func() {
	It("should use the security-group-id selector", func() {
		securityGroups := env.GetSecurityGroups(nodeClass.Spec.VcnId, []string{"test-sg1"})
		Expect(len(securityGroups)).To(BeNumerically(">", 1))
		nodeClass.Spec.SecurityGroupSelector = lo.Map(securityGroups, func(sg core.NetworkSecurityGroup, _ int) v1alpha1.SecurityGroupSelectorTerm {
			return v1alpha1.SecurityGroupSelectorTerm{
				Id: lo.FromPtr(sg.Id),
			}
		})
		pod := test.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
		nodeSgs := env.GetSecurityGroupByInstanceId(env.Monitor.CreatedNodes()[0].Spec.ProviderID)
		sort.Slice(nodeSgs, func(i, j int) bool {
			return nodeSgs[i] < nodeSgs[j]
		})
		nodeClassSgs := lo.Map(securityGroups, func(sg core.NetworkSecurityGroup, _ int) string { return lo.FromPtr(sg.Id) })
		sort.Slice(nodeClassSgs, func(i, j int) bool {
			return nodeClassSgs[i] < nodeClassSgs[j]
		})
		Expect(nodeSgs).To(Equal(nodeClassSgs))
	})

	It("should use the security group selector with names", func() {
		securityGroups := env.GetSecurityGroups(nodeClass.Spec.VcnId, []string{"test-sg1"})
		Expect(len(securityGroups)).To(BeNumerically(">", 1))
		first := securityGroups[0]
		last := securityGroups[len(securityGroups)-1]

		nodeClass.Spec.SecurityGroupSelector = []v1alpha1.SecurityGroupSelectorTerm{
			{
				Name: lo.FromPtr(first.DisplayName),
			},
			{
				Name: lo.FromPtr(last.DisplayName),
			},
		}
		pod := test.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		nodeSgs := env.GetSecurityGroupByInstanceId(env.Monitor.CreatedNodes()[0].Spec.ProviderID)
		sort.Slice(nodeSgs, func(i, j int) bool {
			return nodeSgs[i] < nodeSgs[j]
		})
		nodeClassSgs := lo.Map(securityGroups, func(sg core.NetworkSecurityGroup, _ int) string { return lo.FromPtr(sg.Id) })
		sort.Slice(nodeClassSgs, func(i, j int) bool {
			return nodeClassSgs[i] < nodeClassSgs[j]
		})
		Expect(nodeSgs).To(Equal(nodeClassSgs))
	})

	It("should update the OciNodeClass status security groups", func() {
		env.ExpectCreated(nodeClass)
		EventuallyExpectSecurityGroups(env, nodeClass)
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: v1alpha1.ConditionTypeSecurityGroupsReady, Status: metav1.ConditionTrue})
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: status.ConditionReady, Status: metav1.ConditionTrue})
	})

	It("should have the NodeClass status as not ready since security groups were not resolved", func() {
		nodeClass.Spec.SecurityGroupSelector = []v1alpha1.SecurityGroupSelectorTerm{
			{
				Name: "invalidName",
			},
		}
		env.ExpectCreated(nodeClass)
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: v1alpha1.ConditionTypeSecurityGroupsReady, Status: metav1.ConditionFalse, Message: "SecurityGroupSelector did not match any SecurityGroups"})
		ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: status.ConditionReady, Status: metav1.ConditionFalse, Message: "SecurityGroupsReady=False"})
	})
})

func EventuallyExpectSecurityGroups(env *oci.Environment, nodeClass *v1alpha1.OciNodeClass) {
	securityGroups := env.GetSecurityGroups(nodeClass.Spec.VcnId, []string{"test-sg1"})
	Expect(securityGroups).ToNot(HaveLen(0))

	ids := sets.New(lo.Map(securityGroups, func(s core.NetworkSecurityGroup, _ int) string {
		return lo.FromPtr(s.Id)
	})...)
	Eventually(func(g Gomega) {
		temp := &v1alpha1.OciNodeClass{}
		g.Expect(env.Client.Get(env, client.ObjectKeyFromObject(nodeClass), temp)).To(Succeed())
		g.Expect(sets.New(lo.Map(temp.Status.SecurityGroups, func(s *v1alpha1.SecurityGroup, _ int) string {
			return s.Id
		})...).Equal(ids))
	}).WithTimeout(10 * time.Second).Should(Succeed())
}
