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

package imagefamily

import (
	"context"
	"fmt"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/patrickmn/go-cache"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
)

type Provider struct {
	cache  *cache.Cache
	client api.ComputeClient
}

func NewProvider(client api.ComputeClient, cache *cache.Cache) *Provider {
	return &Provider{
		client: client,
		cache:  cache,
	}
}

func (p *Provider) List(ctx context.Context, nodeclass *v1alpha1.OciNodeClass) ([]core.Image, error) {
	hash, err := hashstructure.Hash(nodeclass.Spec.ImageSelector, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return nil, err
	}
	if images, ok := p.cache.Get(fmt.Sprintf("%d", hash)); ok {
		return images.([]core.Image), nil
	}
	images := make([]core.Image, 0)
	for _, selector := range nodeclass.Spec.ImageSelector {
		req := core.ListImagesRequest{
			CompartmentId:  common.String(selector.CompartmentId),
			DisplayName:    common.String(selector.Name),
			LifecycleState: core.ImageLifecycleStateAvailable}

		// Send the request using the service client
		resp, err := p.client.ListImages(ctx, req)
		if err != nil {
			return nil, err
		}
		images = append(images, resp.Items...)
	}
	// todo sort and unique
	p.cache.SetDefault(fmt.Sprintf("%d", hash), images)
	return images, nil
}
