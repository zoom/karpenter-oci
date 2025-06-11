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

package subnet

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
)

type Provider struct {
	sync.Mutex
	client api.VirtualNetworkClient
	cache  *cache.Cache
}

func NewProvider(client api.VirtualNetworkClient, cache *cache.Cache) *Provider {
	return &Provider{client: client, cache: cache}
}

func (p *Provider) GetSubnetUtilization(ctx context.Context, subnet *core.Subnet) (summary []core.IpInventoryCidrUtilizationSummary, err error) {
	resp, err := p.client.GetSubnetCidrUtilization(ctx, core.GetSubnetCidrUtilizationRequest{
		SubnetId: subnet.Id,
	})
	if err != nil {
		return nil, err
	}
	summary = resp.IpInventoryCidrUtilizationSummary
	return
}

func (p *Provider) GetSubnetAvailableIPv4Count(ctx context.Context, subnet *core.Subnet) (int, error) {
	if subnet.CidrBlock == nil {
		return 0, nil
	}
	availableCount, err := calculateTotalIps(*subnet.CidrBlock)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.GetSubnetCidrUtilization(ctx, core.GetSubnetCidrUtilizationRequest{
		SubnetId: subnet.Id,
	})
	if err != nil {
		return 0, err
	}
	for _, smy := range resp.IpInventoryCidrUtilizationSummary {
		if smy.Cidr != nil && subnet.CidrBlock != nil && *smy.Cidr == *subnet.CidrBlock {
			availableCount = availableCount - int(float32(availableCount)*(*resp.IpInventoryCidrUtilizationSummary[0].Utilization)/100)
			break
		}
	}

	if availableCount < 0 {
		availableCount = 0
	}

	return availableCount, nil
}

func (p *Provider) List(ctx context.Context, nodeClass *v1alpha1.OciNodeClass) ([]core.Subnet, error) {

	hash, err := hashstructure.Hash(nodeClass.Spec.SubnetSelector, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return nil, err
	}

	p.Lock()
	defer p.Unlock()

	if subnets, ok := p.cache.Get(fmt.Sprintf("%s:%d", nodeClass.Spec.VcnId, hash)); ok {
		return subnets.([]core.Subnet), nil
	}
	subnets := make([]core.Subnet, 0)
	for _, selector := range nodeClass.Spec.SubnetSelector {
		// Create a request and dependent object(s).
		req := core.ListSubnetsRequest{CompartmentId: common.String(options.FromContext(ctx).CompartmentId),
			VcnId:          common.String(nodeClass.Spec.VcnId),
			DisplayName:    common.String(selector.Name),
			LifecycleState: core.SubnetLifecycleStateAvailable,
		}

		// Send the request using the service client
		resp, err := p.client.ListSubnets(ctx, req)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, resp.Items...)
	}
	// todo unique and sort
	p.cache.SetDefault(fmt.Sprintf("%s:%d", nodeClass.Spec.VcnId, hash), subnets)
	return subnets, nil
}

func (p *Provider) GetSubnets(ctx context.Context, vnics []core.VnicAttachment, onlyPrimary bool) ([]core.Subnet, error) {

	subnets := make([]core.Subnet, 0)

	var ifOnlyPrimary *core.GetVnicResponse
	for _, vnic := range vnics {

		if onlyPrimary {

			getVnic := core.GetVnicRequest{VnicId: vnic.VnicId}

			resp, err := p.client.GetVnic(ctx, getVnic)
			if err != nil {
				return nil, err
			}

			if resp.IsPrimary == nil || !*resp.IsPrimary {
				ifOnlyPrimary = &resp
				continue
			}

		}

		// Create a request and dependent object(s).
		req := core.GetSubnetRequest{
			SubnetId: vnic.SubnetId,
		}

		// Send the request using the service client
		subnetResp, err := p.client.GetSubnet(ctx, req)
		if err != nil {
			return nil, err
		}

		subnets = append(subnets, subnetResp.Subnet)

		if ifOnlyPrimary != nil { // if onlyPrimary
			break
		}
	}

	// uniq the subnets
	subnets = lo.UniqBy(subnets, func(item core.Subnet) string {
		return *item.Id
	})

	return subnets, nil
}

func calculateTotalIps(cidr string) (int, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	maskLen, maxLen := ipNet.Mask.Size()
	return 1 << (maxLen - maskLen), nil
}
