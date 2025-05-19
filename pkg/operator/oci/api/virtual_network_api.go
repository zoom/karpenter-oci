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

package api

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/core"
)

type VirtualNetworkClient interface {
	ListSubnets(ctx context.Context, request core.ListSubnetsRequest) (response core.ListSubnetsResponse, err error)
	ListNetworkSecurityGroups(ctx context.Context, request core.ListNetworkSecurityGroupsRequest) (response core.ListNetworkSecurityGroupsResponse, err error)
	GetNetworkSecurityGroup(ctx context.Context, request core.GetNetworkSecurityGroupRequest) (response core.GetNetworkSecurityGroupResponse, err error)
	GetSubnet(ctx context.Context, request core.GetSubnetRequest) (response core.GetSubnetResponse, err error)
	GetVnic(ctx context.Context, request core.GetVnicRequest) (response core.GetVnicResponse, err error)
}
