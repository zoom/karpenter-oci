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

package fake

import (
	"context"
	"fmt"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
)

var _ api.VirtualNetworkClient = &VcnCli{}

type VcnCli struct {
	VcnBehavior
	// Store created security groups for retrieval
	createdSecurityGroups map[string]core.NetworkSecurityGroup
	// Store created subnets for retrieval
	createdSubnets map[string]core.Subnet
}

type VcnBehavior struct {
	ListSubnetsOutput              AtomicPtr[core.ListSubnetsResponse]
	ListSecurityGroupOutput        AtomicPtr[core.ListNetworkSecurityGroupsResponse]
	GetSubnetOutput                AtomicPtr[core.GetSubnetResponse]
	GetVnicOutput                  AtomicPtr[core.GetVnicResponse]
	GetSecurityGroupResponse       AtomicPtr[core.GetNetworkSecurityGroupResponse]
	CreateSecurityGroupResponse    AtomicPtr[core.CreateNetworkSecurityGroupResponse]
	DeleteSecurityGroupResponse    AtomicPtr[core.DeleteNetworkSecurityGroupResponse]
	CreateSubnetResponse           AtomicPtr[core.CreateSubnetResponse]
	DeleteSubnetResponse           AtomicPtr[core.DeleteSubnetResponse]
	GetSubnetCidrUtilizationOutput AtomicPtr[map[string]core.GetSubnetCidrUtilizationResponse]
}

var DefaultVnics = []core.Vnic{
	{
		Id:          common.String("ocid1.vnic.oci.iad.aaaaaa"),
		DisplayName: common.String("nic-0"),
		IsPrimary:   common.Bool(true),
		SubnetId:    DefaultSubnets[0].Id,
		NsgIds:      []string{*DefaultSecurityGroup[0].Id, *DefaultSecurityGroup[1].Id, *DefaultSecurityGroup[2].Id},
	},
	{
		Id:          common.String("ocid1.vnic.oci.iad.aaaaab"),
		DisplayName: common.String("nic-1"),
		SubnetId:    DefaultSubnets[1].Id,
		NsgIds:      []string{*DefaultSecurityGroup[1].Id},
	},
}

var DefaultSubnets = []core.Subnet{
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaaa"),
		DisplayName:    common.String("private-1"),
		LifecycleState: core.SubnetLifecycleStateAvailable,
		CidrBlock:      common.String("10.0.0.0/24"),
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaab"),
		DisplayName:    common.String("private-1"),
		LifecycleState: core.SubnetLifecycleStateAvailable,
		CidrBlock:      common.String("10.0.0.10/24"),
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaac"),
		DisplayName:    common.String("private-2"),
		LifecycleState: core.SubnetLifecycleStateAvailable,
		CidrBlock:      common.String("10.0.0.20/24"),
	},
}

var DefaultSecurityGroup = []core.NetworkSecurityGroup{
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("sg-test1"),
		DisplayName:    common.String("securityGroup-test1"),
		LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("sg-test2"),
		DisplayName:    common.String("securityGroup-test2"),
		LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("sg-test3"),
		DisplayName:    common.String("securityGroup-test3"),
		LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
	},
}

func NewVcnCli() *VcnCli {
	return &VcnCli{
		createdSecurityGroups: make(map[string]core.NetworkSecurityGroup),
		createdSubnets:        make(map[string]core.Subnet),
	}
}

func (v *VcnCli) ListSubnets(ctx context.Context, request core.ListSubnetsRequest) (response core.ListSubnetsResponse, err error) {
	if !v.ListSubnetsOutput.IsNil() {
		describeSubnetsOutput := v.ListSubnetsOutput.Clone()
		describeSubnetsOutput.Items = FilterDescribeSubnets(describeSubnetsOutput.Items, *request.DisplayName)
		return *describeSubnetsOutput, nil
	}

	if request.DisplayName == nil {
		return core.ListSubnetsResponse{}, fmt.Errorf("InvalidParameterValue: The filter 'null' is invalid")
	}

	// Combine default subnets with created subnets
	allSubnets := make([]core.Subnet, 0)
	allSubnets = append(allSubnets, DefaultSubnets...)
	for _, subnet := range v.createdSubnets {
		allSubnets = append(allSubnets, subnet)
	}

	return core.ListSubnetsResponse{Items: FilterDescribeSubnets(allSubnets, *request.DisplayName)}, nil
}

func (v *VcnCli) ListNetworkSecurityGroups(ctx context.Context, request core.ListNetworkSecurityGroupsRequest) (response core.ListNetworkSecurityGroupsResponse, err error) {
	if !v.ListSecurityGroupOutput.IsNil() {
		describeSgsOutput := v.ListSecurityGroupOutput.Clone()
		describeSgsOutput.Items = FilterDescribeSecurityGroups(describeSgsOutput.Items, *request.DisplayName)
		return *describeSgsOutput, nil
	}

	if request.DisplayName == nil {
		return core.ListNetworkSecurityGroupsResponse{}, fmt.Errorf("InvalidParameterValue: The filter 'null' is invalid")
	}
	return core.ListNetworkSecurityGroupsResponse{Items: FilterDescribeSecurityGroups(DefaultSecurityGroup, *request.DisplayName)}, nil
}

func (v *VcnCli) GetSubnet(ctx context.Context, request core.GetSubnetRequest) (response core.GetSubnetResponse, err error) {

	if request.SubnetId == nil {
		return core.GetSubnetResponse{}, fmt.Errorf("InvalidParameterValue: The subnetId is empty")
	}

	if !v.GetSubnetOutput.IsNil() {
		getSubnetOutput := *v.GetSubnetOutput.Clone()
		return getSubnetOutput, nil
	}

	// Check created subnets first
	if subnet, found := v.createdSubnets[*request.SubnetId]; found {
		return core.GetSubnetResponse{Subnet: subnet}, nil
	}

	// Fall back to default subnets
	subnet, found := lo.Find(DefaultSubnets, func(item core.Subnet) bool {
		return item.Id == request.SubnetId
	})

	if !found {
		return core.GetSubnetResponse{}, fmt.Errorf("no subnet is found with %s", *request.SubnetId)
	}

	return core.GetSubnetResponse{Subnet: subnet}, nil
}

func (v *VcnCli) GetVnic(ctx context.Context, request core.GetVnicRequest) (response core.GetVnicResponse, err error) {

	if request.VnicId == nil {
		return core.GetVnicResponse{}, fmt.Errorf("InvalidParameterValue: The vnicId is empty")
	}

	if !v.GetVnicOutput.IsNil() {
		return *v.GetVnicOutput.Clone(), nil
	}

	vnic, found := lo.Find(DefaultVnics, func(item core.Vnic) bool {
		return *request.VnicId == *item.Id
	})

	if !found {
		return core.GetVnicResponse{}, fmt.Errorf("no vnic is found with %s", *request.VnicId)
	}

	return core.GetVnicResponse{Vnic: vnic}, nil
}

func (v *VcnCli) GetNetworkSecurityGroup(ctx context.Context, request core.GetNetworkSecurityGroupRequest) (response core.GetNetworkSecurityGroupResponse, err error) {

	if request.NetworkSecurityGroupId == nil {
		return core.GetNetworkSecurityGroupResponse{}, fmt.Errorf("InvalidParameterValue: networkSecurityGroupId is empty")
	}

	if !v.GetSecurityGroupResponse.IsNil() {
		return *v.GetSecurityGroupResponse.Clone(), nil
	}

	// Check created security groups first
	if sg, found := v.createdSecurityGroups[*request.NetworkSecurityGroupId]; found {
		return core.GetNetworkSecurityGroupResponse{NetworkSecurityGroup: sg}, nil
	}

	// Fall back to default security groups
	sgs, found := lo.Find(DefaultSecurityGroup, func(item core.NetworkSecurityGroup) bool {
		return *item.Id == *request.NetworkSecurityGroupId
	})

	if !found {
		return core.GetNetworkSecurityGroupResponse{}, fmt.Errorf("no sgs is found with %s", *request.NetworkSecurityGroupId)
	}

	return core.GetNetworkSecurityGroupResponse{NetworkSecurityGroup: sgs}, nil
}

func (v *VcnCli) GetSubnetCidrUtilization(ctx context.Context, request core.GetSubnetCidrUtilizationRequest) (response core.GetSubnetCidrUtilizationResponse, err error) {
	if !v.GetSubnetCidrUtilizationOutput.IsNil() {
		output := v.GetSubnetCidrUtilizationOutput.Clone()
		return (*output)[*request.SubnetId], nil
	}

	if request.SubnetId == nil {
		return core.GetSubnetCidrUtilizationResponse{}, fmt.Errorf("InvalidParameterValue: The filter 'null' is invalid")
	}
	return core.GetSubnetCidrUtilizationResponse{IpInventoryCidrUtilizationCollection: core.IpInventoryCidrUtilizationCollection{
		Count: common.Int(1),
		IpInventoryCidrUtilizationSummary: []core.IpInventoryCidrUtilizationSummary{
			{Utilization: common.Float32(0), Cidr: common.String("10.0.0.0/24"), AddressType: common.String("Private_IPv4")},
		},
	}}, nil
}

func FilterDescribeSecurityGroups(sgs []core.NetworkSecurityGroup, displayName string) []core.NetworkSecurityGroup {
	return lo.Filter(sgs, func(sg core.NetworkSecurityGroup, _ int) bool {
		return *sg.DisplayName == displayName
	})
}

func FilterDescribeSubnets(subnets []core.Subnet, name string) []core.Subnet {
	return lo.Filter(subnets, func(subnet core.Subnet, _ int) bool {
		return *subnet.DisplayName == name
	})
}

func (v *VcnCli) CreateNetworkSecurityGroup(ctx context.Context, request core.CreateNetworkSecurityGroupRequest) (response core.CreateNetworkSecurityGroupResponse, err error) {
	if !v.CreateSecurityGroupResponse.IsNil() {
		return *v.CreateSecurityGroupResponse.Clone(), nil
	}

	// Create a mock security group for testing
	newSG := core.NetworkSecurityGroup{
		CompartmentId:  request.CompartmentId,
		Id:             common.String("sg-" + *request.DisplayName),
		DisplayName:    request.DisplayName,
		VcnId:          request.VcnId,
		LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
		DefinedTags:    request.DefinedTags,
		FreeformTags:   request.FreeformTags,
		TimeCreated:    &common.SDKTime{Time: time.Now()},
	}

	// Store the created security group for later retrieval
	v.createdSecurityGroups[*newSG.Id] = newSG

	return core.CreateNetworkSecurityGroupResponse{
		NetworkSecurityGroup: newSG,
	}, nil
}

func (v *VcnCli) DeleteNetworkSecurityGroup(ctx context.Context, request core.DeleteNetworkSecurityGroupRequest) (response core.DeleteNetworkSecurityGroupResponse, err error) {
	if !v.DeleteSecurityGroupResponse.IsNil() {
		return *v.DeleteSecurityGroupResponse.Clone(), nil
	}

	// Remove from created security groups if it exists
	if request.NetworkSecurityGroupId != nil {
		// Simulate OCI behavior: check if NSG is still attached to VNICs
		// In a real test environment with actual instances, this would check for attached VNICs
		// For the fake client, we'll simulate a brief delay where deletion fails initially
		if sg, exists := v.createdSecurityGroups[*request.NetworkSecurityGroupId]; exists {
			// If the security group was just created (within last 5 seconds), simulate VNIC attachment
			if sg.TimeCreated != nil && time.Since(sg.TimeCreated.Time) < 5*time.Second {
				return core.DeleteNetworkSecurityGroupResponse{}, fmt.Errorf("NetworkSecurityGroup %s cannot be deleted since it still has vnics attached to it", *request.NetworkSecurityGroupId)
			}
		}
		delete(v.createdSecurityGroups, *request.NetworkSecurityGroupId)
	}

	// Mock deletion - return success
	return core.DeleteNetworkSecurityGroupResponse{}, nil
}

func (v *VcnCli) CreateSubnet(ctx context.Context, request core.CreateSubnetRequest) (response core.CreateSubnetResponse, err error) {
	if !v.CreateSubnetResponse.IsNil() {
		return *v.CreateSubnetResponse.Clone(), nil
	}

	// Create a mock subnet for testing
	newSubnet := core.Subnet{
		CompartmentId:  request.CompartmentId,
		Id:             common.String("subnet-" + *request.DisplayName),
		DisplayName:    request.DisplayName,
		VcnId:          request.VcnId,
		CidrBlock:      request.CidrBlock,
		LifecycleState: core.SubnetLifecycleStateAvailable,
		DefinedTags:    request.DefinedTags,
		FreeformTags:   request.FreeformTags,
		TimeCreated:    &common.SDKTime{Time: time.Now()},
	}

	// Store the created subnet for later retrieval
	v.createdSubnets[*newSubnet.Id] = newSubnet

	return core.CreateSubnetResponse{
		Subnet: newSubnet,
	}, nil
}

func (v *VcnCli) DeleteSubnet(ctx context.Context, request core.DeleteSubnetRequest) (response core.DeleteSubnetResponse, err error) {
	if !v.DeleteSubnetResponse.IsNil() {
		return *v.DeleteSubnetResponse.Clone(), nil
	}

	// Remove from created subnets if it exists
	if request.SubnetId != nil {
		// Simulate OCI behavior: check if subnet is still in use by instances
		// For the fake client, we'll simulate a brief delay where deletion fails initially
		if subnet, exists := v.createdSubnets[*request.SubnetId]; exists {
			// If the subnet was just created (within last 5 seconds), simulate being in use
			if subnet.TimeCreated != nil && time.Since(subnet.TimeCreated.Time) < 5*time.Second {
				return core.DeleteSubnetResponse{}, fmt.Errorf("subnet %s cannot be deleted since it still has resources attached to it", *request.SubnetId)
			}
		}
		delete(v.createdSubnets, *request.SubnetId)
	}

	// Mock deletion - return success
	return core.DeleteSubnetResponse{}, nil
}

func (v *VcnCli) Reset() {
	v.ListSubnetsOutput.Reset()
	v.CreateSecurityGroupResponse.Reset()
	v.DeleteSecurityGroupResponse.Reset()
	v.CreateSubnetResponse.Reset()
	v.DeleteSubnetResponse.Reset()
	// Clear created resources
	v.createdSecurityGroups = make(map[string]core.NetworkSecurityGroup)
	v.createdSubnets = make(map[string]core.Subnet)
}
