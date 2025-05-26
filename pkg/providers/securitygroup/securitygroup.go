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

package securitygroup

import (
	"context"
	"fmt"
	"sync"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/patrickmn/go-cache"
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

func (p *Provider) List(ctx context.Context, nodeClass *v1alpha1.OciNodeClass) ([]core.NetworkSecurityGroup, error) {
	if len(nodeClass.Spec.SecurityGroupSelector) == 0 {
		return []core.NetworkSecurityGroup{}, nil
	}
	hash, err := hashstructure.Hash(nodeClass.Spec.SecurityGroupSelector, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return nil, err
	}

	p.Lock()
	defer p.Unlock()

	if sgs, ok := p.cache.Get(fmt.Sprintf("%s:%d", nodeClass.Spec.VcnId, hash)); ok {
		return sgs.([]core.NetworkSecurityGroup), nil
	}
	// Create a request and dependent object(s).
	sgs := make([]core.NetworkSecurityGroup, 0)
	for _, selector := range nodeClass.Spec.SecurityGroupSelector {
		req := core.ListNetworkSecurityGroupsRequest{CompartmentId: common.String(options.FromContext(ctx).CompartmentId),
			VcnId:          common.String(nodeClass.Spec.VcnId),
			DisplayName:    common.String(selector.Name),
			LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
		}
		// Send the request using the service client
		resp, err := p.client.ListNetworkSecurityGroups(ctx, req)
		if err != nil {
			return nil, err
		}
		sgs = append(sgs, resp.Items...)
	}
	p.cache.SetDefault(fmt.Sprintf("%s:%d", nodeClass.Spec.VcnId, hash), sgs)
	return sgs, nil
}

func (p *Provider) GetSecurityGroups(ctx context.Context, vnics []core.VnicAttachment, onlyPrimaryVnic bool) ([]core.NetworkSecurityGroup, error) {

	sgs := make([]core.NetworkSecurityGroup, 0)

	// vnics
	for _, vnic := range vnics {
		getVnic := core.GetVnicRequest{VnicId: vnic.VnicId}

		resp, err := p.client.GetVnic(ctx, getVnic)
		if err != nil {
			return nil, err
		}

		if onlyPrimaryVnic && (resp.IsPrimary == nil || !*resp.IsPrimary) {
			continue
		}

		for _, nsgId := range resp.NsgIds {

			getSg := core.GetNetworkSecurityGroupRequest{
				NetworkSecurityGroupId: common.String(nsgId),
			}

			group, err := p.client.GetNetworkSecurityGroup(ctx, getSg)
			if err != nil {
				return nil, err
			}
			sgs = append(sgs, group.NetworkSecurityGroup)
		}

	}

	return sgs, nil

}
