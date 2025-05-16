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

package launchtemplate

import (
	"context"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

type DefaultProvider struct {
	imageFamily     *imagefamily.Resolver
	CABundle        *string
	ClusterEndpoint string
	BootstrapToken  string
}

func NewDefaultProvider(imageFamily *imagefamily.Resolver, cABundle *string, clusterEndpoint string, bootstrapToken string) *DefaultProvider {
	return &DefaultProvider{
		imageFamily:     imageFamily,
		CABundle:        cABundle,
		ClusterEndpoint: clusterEndpoint,
		BootstrapToken:  bootstrapToken,
	}
}

func (p *DefaultProvider) CreateLaunchTemplate(ctx context.Context, nodeClass *v1alpha1.OciNodeClass, nodeClaim *v1.NodeClaim, instanceType *cloudprovider.InstanceType) ([]*imagefamily.LaunchTemplate, error) {
	imgOptions, err := p.createImageOptions(ctx, nodeClass, nodeClaim.Labels)
	if err != nil {
		return nil, err
	}
	resolvedLaunchTemplates, err := p.imageFamily.Resolve(ctx, nodeClass, nodeClaim, instanceType, imgOptions)
	return resolvedLaunchTemplates, err
}

func (p *DefaultProvider) createImageOptions(ctx context.Context, nodeClass *v1alpha1.OciNodeClass, labels map[string]string) (*imagefamily.Options, error) {
	solvedOptions := &imagefamily.Options{
		ClusterName:     options.FromContext(ctx).ClusterName,
		ClusterDns:      options.FromContext(ctx).ClusterDns,
		ClusterEndpoint: p.ClusterEndpoint,
		CABundle:        p.CABundle,
		BootstrapToken:  p.BootstrapToken,
		Labels:          labels,
		NodeClassName:   nodeClass.Name,
	}
	return solvedOptions, nil
}
