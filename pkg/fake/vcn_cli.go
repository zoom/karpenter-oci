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
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
)

var _ api.VirtualNetworkClient = &VcnCli{}

type VcnCli struct {
	VcnBehavior
}

type VcnBehavior struct {
	ListSubnetsOutput        AtomicPtr[core.ListSubnetsResponse]
	ListSecurityGroupOutput  AtomicPtr[core.ListNetworkSecurityGroupsResponse]
	GetSubnetOutput          AtomicPtr[core.GetSubnetResponse]
	GetVnicOutput            AtomicPtr[core.GetVnicResponse]
	GetSecurityGroupResponse AtomicPtr[core.GetNetworkSecurityGroupResponse]
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
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaab"),
		DisplayName:    common.String("private-1"),
		LifecycleState: core.SubnetLifecycleStateAvailable,
	},
	{
		CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
		Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaac"),
		DisplayName:    common.String("private-2"),
		LifecycleState: core.SubnetLifecycleStateAvailable,
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
	return &VcnCli{}
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
	return core.ListSubnetsResponse{Items: FilterDescribeSubnets(DefaultSubnets, *request.DisplayName)}, nil
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

	sgs, found := lo.Find(DefaultSecurityGroup, func(item core.NetworkSecurityGroup) bool {
		return *item.Id == *request.NetworkSecurityGroupId
	})

	if !found {
		return core.GetNetworkSecurityGroupResponse{}, fmt.Errorf("no sgs is found with %s", *request.NetworkSecurityGroupId)
	}

	return core.GetNetworkSecurityGroupResponse{NetworkSecurityGroup: sgs}, nil
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

func (v *VcnCli) Reset() {
	v.ListSubnetsOutput.Reset()
}
