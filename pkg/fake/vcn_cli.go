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
	ListSubnetsOutput       AtomicPtr[core.ListSubnetsResponse]
	ListSecurityGroupOutput AtomicPtr[core.ListNetworkSecurityGroupsResponse]
}

var defaultSubnets = []core.Subnet{
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

var defaultSecurityGroup = []core.NetworkSecurityGroup{
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
	return core.ListSubnetsResponse{Items: FilterDescribeSubnets(defaultSubnets, *request.DisplayName)}, nil
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
	return core.ListNetworkSecurityGroupsResponse{Items: FilterDescribeSecurityGroups(defaultSecurityGroup, *request.DisplayName)}, nil
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
