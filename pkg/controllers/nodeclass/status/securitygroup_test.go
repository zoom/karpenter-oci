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

package status

import (
	"github.com/oracle/oci-go-sdk/v65/common"
	oci_core "github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	test "github.com/zoom/karpenter-oci/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
)

var _ = Describe("NodeClass Security Group Status Controller", func() {
	BeforeEach(func() {
		nodeClass = test.OciNodeClass()
		ociEnv.VcnCli.ListSecurityGroupOutput.Set(&oci_core.ListNetworkSecurityGroupsResponse{Items: []oci_core.NetworkSecurityGroup{{
			CompartmentId:  common.String("comp_1"),
			Id:             common.String("sg-test1"),
			LifecycleState: oci_core.NetworkSecurityGroupLifecycleStateAvailable,
			VcnId:          common.String("vcn_1"),
			DisplayName:    common.String("securityGroup-test1"),
		},
			{
				CompartmentId:  common.String("comp_1"),
				Id:             common.String("sg-test2"),
				LifecycleState: oci_core.NetworkSecurityGroupLifecycleStateAvailable,
				VcnId:          common.String("vcn_1"),
				DisplayName:    common.String("securityGroup-test2"),
			},
		}})
	})
	It("Should update OciNodeClass status for Security Groups", func() {
		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, statusController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		Expect(nodeClass.Status.SecurityGroups).To(Equal([]*v1alpha1.SecurityGroup{
			{
				Id:   "sg-test1",
				Name: "securityGroup-test1",
			},
			{
				Id:   "sg-test2",
				Name: "securityGroup-test2",
			},
		}))
		Expect(nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSecurityGroupsReady).IsTrue()).To(BeTrue())
	})
})
