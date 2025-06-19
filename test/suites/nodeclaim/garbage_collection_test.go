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
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily/bootstrap"
	"time"

	"github.com/samber/lo"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GarbageCollection", func() {
	var instanceInput core.LaunchInstanceRequest

	BeforeEach(func() {
		securityGroups := env.GetSecurityGroups(nodeClass.Spec.VcnId, []string{nodeClass.Spec.SecurityGroupSelector[0].Name})
		subnets := env.GetSubnets(nodeClass.Spec.VcnId, []string{nodeClass.Spec.SubnetSelector[0].Name})
		images := env.GetImages(nodeClass.Spec.ImageSelector[0].CompartmentId, []string{nodeClass.Spec.ImageSelector[0].Name})
		Expect(securityGroups).ToNot(HaveLen(0))
		Expect(subnets).ToNot(HaveLen(0))

		instanceInput = core.LaunchInstanceRequest{LaunchInstanceDetails: core.LaunchInstanceDetails{
			CreateVnicDetails: &core.CreateVnicDetails{SubnetId: subnets[0].Id, NsgIds: lo.Map(securityGroups, func(item core.NetworkSecurityGroup, index int) string {
				return lo.FromPtr(item.Id)
			})},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId:             common.String(images[0]),
				BootVolumeVpusPerGB: common.Int64(nodeClass.Spec.BootConfig.BootVolumeVpusPerGB),
				BootVolumeSizeInGBs: common.Int64(nodeClass.Spec.BootConfig.BootVolumeSizeInGBs)},
			DefinedTags: map[string]map[string]interface{}{env.TagNamespace: {
				"karpenter_k8s_oracle/ocinodeclass": nodeClass.Name,
				"karpenter_sh/nodepool":             nodePool.Name,
				"karpenter_sh/managed-by":           env.ClusterName,
			},
			},
			CompartmentId:      common.String(env.CompartmentId),
			DisplayName:        common.String(nodePool.Name),
			AvailabilityDomain: common.String(env.AvailableDomainInfo[0]),
			Shape:              common.String("VM.Standard.E4.Flex"),
			ShapeConfig: &core.LaunchInstanceShapeConfigDetails{
				MemoryInGBs: common.Float32(8),
				Ocpus:       common.Float32(1)},
			InstanceOptions: &core.InstanceOptions{AreLegacyImdsEndpointsDisabled: common.Bool(true)},
		}}
	})
	It("should succeed to garbage collect an Instance that was launched by a NodeClaim but has no Instance mapping", func() {
		oke := bootstrap.OKE{
			Options: bootstrap.Options{
				ClusterName:     env.ClusterName,
				ClusterEndpoint: env.ClusterEndpoint,
				ClusterDns:      env.ClusterDns,
				CABundle:        lo.ToPtr(env.ExpectCABundle()),
			},
		}
		instanceInput.Metadata = map[string]string{"user_data": lo.Must(oke.Script())}
		// Create an instance manually to mock Karpenter launching an instance
		out := env.EventuallyExpectRunInstances(instanceInput)

		DeferCleanup(func() {
			_, err := env.CMPAPI.TerminateInstance(env.Context, core.TerminateInstanceRequest{
				InstanceId: out.Id,
			})
			Expect(err).ToNot(HaveOccurred())
		})

		// Wait for the node to register with the cluster
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]

		// Eventually expect the node and the instance to be removed (shutting-down)
		env.EventuallyExpectNotFound(node)
		Eventually(func(g Gomega) {
			g.Expect(string(env.GetInstanceByID(lo.FromPtr(out.Id)).LifecycleState)).To(BeElementOf("TERMINATING", "TERMINATED"))
		}, time.Second*10).Should(Succeed())
	})
})
