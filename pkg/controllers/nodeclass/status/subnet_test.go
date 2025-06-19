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
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	test2 "github.com/zoom/karpenter-oci/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
)

var _ = Describe("NodeClass Subnet Status Controller", func() {
	BeforeEach(func() {
		nodeClass = test2.OciNodeClass()
		ociEnv.VcnCli.ListSubnetsOutput.Set(&core.ListSubnetsResponse{
			Items: []core.Subnet{{
				CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
				Id:             common.String("subnet-id-1"),
				LifecycleState: core.SubnetLifecycleStateAvailable,
				VcnId:          common.String("vcn_1"),
				DisplayName:    common.String("private-1"),
			},
				{
					CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
					Id:             common.String("subnet-id-2"),
					LifecycleState: core.SubnetLifecycleStateAvailable,
					VcnId:          common.String("vcn_1"),
					DisplayName:    common.String("private-1"),
				},
				{
					CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
					Id:             common.String("subnet-id-3"),
					LifecycleState: core.SubnetLifecycleStateAvailable,
					VcnId:          common.String("vcn_1"),
					DisplayName:    common.String("private-3"),
				}},
		})
	})
	It("Should update OciNodeClass status for Subnets", func() {
		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, statusController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		Expect(nodeClass.Status.Subnets).To(Equal([]*v1alpha1.Subnet{
			{
				Id:   "subnet-id-1",
				Name: "private-1",
			},
			{
				Id:   "subnet-id-2",
				Name: "private-1",
			},
		}))
		Expect(nodeClass.StatusConditions().IsTrue(v1alpha1.ConditionTypeSubnetsReady)).To(BeTrue())
	})
	It("Should not resolve a invalid selectors for Subnet", func() {
		nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{{
			Name: "fake_subnet_name",
		}}
		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, statusController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		Expect(nodeClass.Status.Subnets).To(BeNil())
		Expect(nodeClass.StatusConditions().Get(v1alpha1.ConditionTypeSubnetsReady).IsFalse()).To(BeTrue())
	})
})
