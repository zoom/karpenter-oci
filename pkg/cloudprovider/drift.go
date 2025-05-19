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

package cloudprovider

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

const (
	ImageDrift         cloudprovider.DriftReason = "ImageDrift"
	SubnetDrift        cloudprovider.DriftReason = "SubnetDrift"
	SecurityGroupDrift cloudprovider.DriftReason = "SecurityGroupDrift"
	NodeClassDrift     cloudprovider.DriftReason = "NodeClassDrift"
)

func (c *CloudProvider) isNodeClassDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim, nodePool *karpv1.NodePool, nodeClass *v1alpha1.OciNodeClass) (cloudprovider.DriftReason, error) {
	instance, err := c.instanceProvider.Get(ctx, nodeClaim.Status.ProviderID)
	if err != nil {
		return "", err
	}

	imageDrifted, err := c.isImageDrifted(ctx, nodeClaim, nodePool, instance, nodeClass)
	if err != nil {
		return "", fmt.Errorf("calculating image drift, %w", err)
	}
	vnics, err := c.instanceProvider.GetVnicAttachments(ctx, instance)
	if err != nil {
		return "", fmt.Errorf("calculating securitygroup drift, %w", err)
	}

	sgs, err := c.instanceProvider.GetSecurityGroups(ctx, vnics, true)
	if err != nil {
		return "", fmt.Errorf("calculating securitygroup drift, %w", err)
	}
	securitygroupDrifted, err := c.areSecurityGroupsDrifted(sgs, nodeClass)
	if err != nil {
		return "", fmt.Errorf("calculating securitygroup drift, %w", err)
	}

	// get subnet
	subnets, err := c.instanceProvider.GetSubnets(ctx, vnics, true)
	if err != nil {
		return "", fmt.Errorf("calculating subnet drift, %w", err)
	}

	subnetDrifted, err := c.isSubnetDrifted(subnets, nodeClass)
	if err != nil {
		return "", fmt.Errorf("calculating subnet drift, %w", err)
	}
	drifted := lo.FindOrElse([]cloudprovider.DriftReason{imageDrifted, securitygroupDrifted, subnetDrifted}, "", func(i cloudprovider.DriftReason) bool {
		return string(i) != ""
	})
	return drifted, nil
}

func (c *CloudProvider) isImageDrifted(ctx context.Context, nodeClaim *karpv1.NodeClaim, nodePool *karpv1.NodePool,
	instance *core.Instance, nodeClass *v1alpha1.OciNodeClass) (cloudprovider.DriftReason, error) {
	instanceTypes, err := c.GetInstanceTypes(ctx, nodePool)
	if err != nil {
		return "", fmt.Errorf("getting instanceTypes, %w", err)
	}
	nodeInstanceType, found := lo.Find(instanceTypes, func(instType *cloudprovider.InstanceType) bool {
		return instType.Name == nodeClaim.Labels[corev1.LabelInstanceTypeStable]
	})
	if !found {
		return "", fmt.Errorf(`finding node instance type "%s"`, nodeClaim.Labels[corev1.LabelInstanceTypeStable])
	}
	if len(nodeClass.Status.Images) == 0 {
		return "", fmt.Errorf("no image exist given constraints")
	}
	mappedImgs := imagefamily.FindCompatibleInstanceType([]*cloudprovider.InstanceType{nodeInstanceType}, nodeClass.Status.Images)
	if !lo.Contains(lo.Keys(mappedImgs), *instance.ImageId) {
		return ImageDrift, nil
	}
	return "", nil
}

// Checks if the subnets are drifted, by comparing the subnet returned from the subnetProvider
// to the oci instance subnets
func (c *CloudProvider) isSubnetDrifted(subnets []core.Subnet, nodeClass *v1alpha1.OciNodeClass) (cloudprovider.DriftReason, error) {
	// subnets need to be found to check for drift
	if len(nodeClass.Status.Subnets) == 0 {
		return "", fmt.Errorf("no subnets are discovered")
	}

	for _, sub := range subnets {

		_, found := lo.Find(nodeClass.Status.Subnets, func(subnet *v1alpha1.Subnet) bool {
			return subnet.Id == *sub.Id
		})

		if !found {
			return SubnetDrift, nil
		}
	}

	return "", nil
}

// Checks if the security groups are drifted, by comparing the security groups returned from the SecurityGroupProvider
// to the oci instance security groups
func (c *CloudProvider) areSecurityGroupsDrifted(sgs []core.NetworkSecurityGroup, nodeClass *v1alpha1.OciNodeClass) (cloudprovider.DriftReason, error) {
	securityGroupIds := sets.New(lo.Map(nodeClass.Status.SecurityGroups, func(sg *v1alpha1.SecurityGroup, _ int) string { return sg.Id })...)
	if len(securityGroupIds) == 0 {
		return "", fmt.Errorf("no security groups are present in the status")
	}

	sgsIds := sets.New(lo.Map(sgs, func(item core.NetworkSecurityGroup, _ int) string {
		return *item.Id
	})...)

	if !securityGroupIds.Equal(sgsIds) {
		return SecurityGroupDrift, nil
	}
	return "", nil
}
