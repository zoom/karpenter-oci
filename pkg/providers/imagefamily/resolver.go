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
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily/bootstrap"
	core "k8s.io/api/core/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

const (
	MemoryAvailable   = "memory.available"
	NodeFSAvailable   = "nodefs.available"
	NodeFSInodesFree  = "nodefs.inodesFree"
	ImageFSAvailable  = "imagefs.available"
	ImageFSInodesFree = "imagefs.inodesFree"
)

type Resolver struct {
	amiProvider *Provider
}

// NewResolver constructs a new launch template Resolver
func NewResolver(amiProvider *Provider) *Resolver {
	return &Resolver{
		amiProvider: amiProvider,
	}
}

// Options define the static launch template parameters
type Options struct {
	ClusterName     string
	ClusterEndpoint string
	ClusterDns      string
	CABundle        *string `hash:"ignore"`
	BootstrapToken  string
	Labels          map[string]string `hash:"ignore"`
	NodeClassName   string
}

// LaunchTemplate holds the dynamically generated launch template parameters
type LaunchTemplate struct {
	*Options
	UserData bootstrap.Bootstrapper
	ImageId  string
}

// DefaultFamily provides default values for AMIFamilies that compose it
type DefaultFamily struct{}

type ImageFamily interface {
	UserData(kubeletConfig *v1alpha1.KubeletConfiguration, taints []core.Taint, labels map[string]string, customUserData *string, preInstallScript *string) bootstrap.Bootstrapper
}

func (r Resolver) Resolve(ctx context.Context, nodeClass *v1alpha1.OciNodeClass, nodeClaim *v1.NodeClaim, instanceType *cloudprovider.InstanceType, options *Options) ([]*LaunchTemplate, error) {
	images, err := r.amiProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("no images exist given constraints")
	}
	imageFamily := GetImageFamily(nodeClass.Spec.ImageFamily, options)
	res := make([]*LaunchTemplate, 0)
	for _, image := range images {
		temp, err := r.resolveLaunchTemplate(nodeClass, nodeClaim, instanceType, imageFamily, *image.Id, options)
		if err != nil {
			return nil, err
		}
		res = append(res, temp)
	}
	return res, nil
}

func GetImageFamily(imageFamily string, options *Options) ImageFamily {
	switch imageFamily {
	case v1alpha1.Ubuntu2204ImageFamily:
		return &UbuntuLinux{Options: options}
	case v1alpha1.OracleOKELinuxImageFamily:
		return &OracleOKELinux{Options: options}
	case v1alpha1.CustomImageFamily:
		return &Custom{Options: options}
	default:
		return &OracleOKELinux{Options: options}
	}
}

func (r Resolver) resolveLaunchTemplate(nodeClass *v1alpha1.OciNodeClass, nodeClaim *v1.NodeClaim, instanceType *cloudprovider.InstanceType, imageFamily ImageFamily, imageId string, options *Options) (*LaunchTemplate, error) {
	kubeletConfig := &v1alpha1.KubeletConfiguration{}
	if nodeClass.Spec.Kubelet != nil {
		kubeletConfig = nodeClass.Spec.Kubelet.DeepCopy()
	}
	// nolint:gosec
	// We know that it's not possible to have values that would overflow int32 here since we control
	// the maxPods values that we pass in here
	if kubeletConfig.MaxPods == nil {
		kubeletConfig.MaxPods = lo.ToPtr(int32(instanceType.Capacity.Pods().Value()))
	}
	resolved := &LaunchTemplate{
		Options: options,
		UserData: imageFamily.UserData(
			kubeletConfig,
			append(nodeClaim.Spec.Taints, nodeClaim.Spec.StartupTaints...),
			options.Labels,
			nodeClass.Spec.UserData,
			nodeClass.Spec.PreInstallScript,
		),
		ImageId: imageId,
	}
	return resolved, nil
}
