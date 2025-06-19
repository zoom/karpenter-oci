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

package oci

import (
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	utils2 "github.com/zoom/karpenter-oci/pkg/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck,ST1001
	. "github.com/onsi/gomega"    //nolint:staticcheck,ST1001
)

func (env *Environment) ExpectWindowsIPAMEnabled() {
	GinkgoHelper()
	env.ExpectConfigMapDataOverridden(types.NamespacedName{Namespace: "kube-system", Name: "amazon-vpc-cni"}, map[string]string{
		"enable-windows-ipam": "true",
	})
}

func (env *Environment) ExpectWindowsIPAMDisabled() {
	GinkgoHelper()
	env.ExpectConfigMapDataOverridden(types.NamespacedName{Namespace: "kube-system", Name: "amazon-vpc-cni"}, map[string]string{
		"enable-windows-ipam": "false",
	})
}

func (env *Environment) ExpectInstance(nodeName string) Assertion {
	return Expect(env.GetInstance(nodeName))
}

func (env *Environment) GetInstance(nodeName string) core.Instance {
	node := env.GetNode(nodeName)
	return env.GetInstanceByID(env.ExpectParsedProviderID(node.Spec.ProviderID))
}

func (env *Environment) ExpectInstanceTerminated(nodeName string) {
	GinkgoHelper()
	node := env.GetNode(nodeName)
	_, err := env.CMPAPI.TerminateInstance(env.Context, core.TerminateInstanceRequest{
		InstanceId: common.String(env.ExpectParsedProviderID(node.Spec.ProviderID)),
	})
	Expect(err).To(Succeed())
}

func (env *Environment) GetInstanceByID(instanceID string) core.Instance {
	GinkgoHelper()
	instance, err := env.CMPAPI.GetInstance(env.Context, core.GetInstanceRequest{
		InstanceId: common.String(instanceID),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(instance.RawResponse.StatusCode).To(Equal(http.StatusOK))
	return instance.Instance
}

func (env *Environment) GetVolumeAttach(instanceId string) []core.VolumeAttachment {
	attach, err := env.CMPAPI.ListVolumeAttachments(env.Context, core.ListVolumeAttachmentsRequest{
		CompartmentId: lo.ToPtr(env.CompartmentId),
		InstanceId:    lo.ToPtr(instanceId),
	})
	Expect(err).ToNot(HaveOccurred())
	return attach.Items
}

func (env *Environment) GetVolumes(ids ...string) []core.Volume {
	volumes := make([]core.Volume, 0)
	for _, id := range ids {
		volumes = append(volumes, env.GetVolume(id))
	}
	return volumes
}

func (env *Environment) GetVolume(id string) core.Volume {
	dvo, err := env.STORAGEAPI.GetVolume(env.Context, core.GetVolumeRequest{
		VolumeId: lo.ToPtr(id),
	})
	Expect(err).ToNot(HaveOccurred())
	return dvo.Volume
}

func (env *Environment) GetNetworkInterface(id string) core.VnicAttachment {
	GinkgoHelper()
	dnio, err := env.CMPAPI.GetVnicAttachment(env.Context, core.GetVnicAttachmentRequest{VnicAttachmentId: common.String(id)})
	Expect(err).ToNot(HaveOccurred())
	return dnio.VnicAttachment
}

func (env *Environment) GetSubnets(vcnId string, names []string) []core.Subnet {
	subnets := make([]core.Subnet, 0)
	for _, name := range names {
		req := core.ListSubnetsRequest{CompartmentId: common.String(env.CompartmentId),
			VcnId:          common.String(vcnId),
			DisplayName:    common.String(name),
			LifecycleState: core.SubnetLifecycleStateAvailable,
		}
		subnets = append(subnets, lo.Must(env.VCNAPI.ListSubnets(env.Context, req)).Items...)
	}

	return subnets
}

func (env *Environment) GetSubnetByInstanceId(instanceId string) string {
	vnicAttach, err := env.CMPAPI.ListVnicAttachments(env.Context, core.ListVnicAttachmentsRequest{CompartmentId: lo.ToPtr(env.CompartmentId), InstanceId: common.String(instanceId)})
	Expect(err).ToNot(HaveOccurred())
	for _, vnicAttachment := range vnicAttach.Items {
		vnic, err := env.VCNAPI.GetVnic(env.Context, core.GetVnicRequest{
			VnicId: vnicAttachment.VnicId,
		})
		Expect(err).ToNot(HaveOccurred())
		if lo.FromPtr(vnic.IsPrimary) {
			return lo.FromPtr(vnic.SubnetId)
		}
	}
	return ""
}

func (env *Environment) GetSecurityGroupByInstanceId(instanceId string) []string {
	vnicAttach, err := env.CMPAPI.ListVnicAttachments(env.Context, core.ListVnicAttachmentsRequest{CompartmentId: lo.ToPtr(env.CompartmentId), InstanceId: common.String(instanceId)})
	Expect(err).ToNot(HaveOccurred())
	for _, vnicAttachment := range vnicAttach.Items {
		vnic, err := env.VCNAPI.GetVnic(env.Context, core.GetVnicRequest{
			VnicId: vnicAttachment.VnicId,
		})
		Expect(err).ToNot(HaveOccurred())
		if lo.FromPtr(vnic.IsPrimary) {
			return vnic.NsgIds
		}
	}
	return []string{}
}

// GetSecurityGroups returns all getSecurityGroups matching the label selector
func (env *Environment) GetSecurityGroups(vcnId string, names []string) []core.NetworkSecurityGroup {
	securityGroups := make([]core.NetworkSecurityGroup, 0)
	for _, name := range names {
		req := core.ListNetworkSecurityGroupsRequest{CompartmentId: common.String(env.CompartmentId),
			VcnId:          common.String(vcnId),
			DisplayName:    common.String(name),
			LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
		}
		securityGroups = append(securityGroups, lo.Must(env.VCNAPI.ListNetworkSecurityGroups(env.Context, req)).Items...)
	}

	return securityGroups
}

func (env *Environment) GetImages(compartmentId string, names []string) []string {
	images := make([]string, 0)
	for _, name := range names {
		req := core.ListImagesRequest{CompartmentId: common.String(compartmentId), DisplayName: common.String(name), LifecycleState: core.ImageLifecycleStateAvailable}
		images = append(images, lo.Map(lo.Must(env.CMPAPI.ListImages(env.Context, req)).Items, func(item core.Image, index int) string { return utils2.ToString(item.Id) })...)
	}
	return images
}

func (env *Environment) ExpectParsedProviderID(providerID string) string {
	GinkgoHelper()
	providerIDSplit := strings.Split(providerID, "/")
	Expect(len(providerIDSplit)).ToNot(Equal(0))
	return providerIDSplit[len(providerIDSplit)-1]
}

func (env *Environment) K8sVersion() string {
	GinkgoHelper()

	return env.K8sVersionWithOffset(0)
}

func (env *Environment) K8sVersionWithOffset(offset int) string {
	GinkgoHelper()

	serverVersion, err := env.KubeClient.Discovery().ServerVersion()
	Expect(err).To(BeNil())
	minorVersion, err := strconv.Atoi(strings.TrimSuffix(serverVersion.Minor, "+"))
	Expect(err).To(BeNil())
	// Choose a minor version one lesser than the server's minor version. This ensures that we choose an AMI for
	// this test that wouldn't be selected as Karpenter's SSM default (therefore avoiding false positives), and also
	// ensures that we aren't violating version skew.
	return fmt.Sprintf("%s.%d", serverVersion.Major, minorVersion-offset)
}

func (env *Environment) K8sMinorVersion() int {
	GinkgoHelper()

	version, err := strconv.Atoi(strings.Split(env.K8sVersion(), ".")[1])
	Expect(err).ToNot(HaveOccurred())
	return version
}

func (env *Environment) EventuallyExpectRunInstances(instanceInput core.LaunchInstanceRequest) core.Instance {
	GinkgoHelper()
	var reservation core.Instance
	Eventually(func(g Gomega) {
		out, err := env.CMPAPI.LaunchInstance(env.Context, instanceInput)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out.RawResponse.StatusCode).To(Equal(http.StatusOK))
		reservation = out.Instance
	}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	return reservation
}
