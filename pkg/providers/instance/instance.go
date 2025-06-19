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

package instance

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/samber/lo/mutable"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/cache"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/launchtemplate"
	"github.com/zoom/karpenter-oci/pkg/providers/securitygroup"
	"github.com/zoom/karpenter-oci/pkg/providers/subnet"
	"github.com/zoom/karpenter-oci/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"net/http"
	corev1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"strconv"
	"strings"
)

type Provider struct {
	compClient             api.ComputeClient
	subnetProvider         *subnet.Provider
	securityGroupProvider  *securitygroup.Provider
	launchTemplateProvider *launchtemplate.DefaultProvider
	unavailableOfferings   *cache.UnavailableOfferings
}

const Gi = 1024 * 1024 * 1024

func NewProvider(compClient api.ComputeClient, subnetProvider *subnet.Provider, securityGroupProvider *securitygroup.Provider, launchProvider *launchtemplate.DefaultProvider, unavailableOfferings *cache.UnavailableOfferings) *Provider {
	return &Provider{
		compClient:             compClient,
		subnetProvider:         subnetProvider,
		securityGroupProvider:  securityGroupProvider,
		launchTemplateProvider: launchProvider,
		unavailableOfferings:   unavailableOfferings,
	}
}

func (p *Provider) Create(ctx context.Context, nodeClass *v1alpha1.OciNodeClass, nodeClaim *corev1.NodeClaim, instanceTypes []*corecloudprovider.InstanceType) (*core.Instance, error) {
	subnets, err := p.subnetProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}
	if len(subnets) == 0 {
		return nil, fmt.Errorf("no subnets found for vcn: %s, selector: %v", nodeClass.Spec.VcnId, nodeClass.Spec.SubnetSelector)
	}
	sgs, err := p.securityGroupProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}
	sgsIds := lo.Map[core.NetworkSecurityGroup, string](sgs, func(item core.NetworkSecurityGroup, index int) string {
		return utils.ToString(item.Id)
	})
	instanceType, zone := pickBestInstanceType(nodeClaim, instanceTypes)
	ad, ok := lo.Find(options.FromContext(ctx).AvailableDomains, func(item string) bool {
		return strings.Contains(item, zone)
	})
	if !ok {
		return nil, fmt.Errorf("failed to find a zone for %s, available az: %s", zone, options.FromContext(ctx).AvailableDomains)
	}
	if instanceType == nil {
		return nil, corecloudprovider.NewInsufficientCapacityError(fmt.Errorf("no instance types available"))
	}
	blockDevices := lo.Map[*v1alpha1.VolumeAttributes, core.LaunchAttachVolumeDetails](nodeClass.Spec.BlockDevices,
		func(item *v1alpha1.VolumeAttributes, index int) core.LaunchAttachVolumeDetails {
			return core.LaunchAttachIScsiVolumeDetails{
				LaunchCreateVolumeDetails: core.LaunchCreateVolumeFromAttributes{VpusPerGB: common.Int64(item.VpusPerGB),
					SizeInGBs: common.Int64(item.SizeInGBs)}}
		})
	template, err := p.launchTemplateProvider.CreateLaunchTemplate(ctx, nodeClass, nodeClaim, instanceType)
	if err != nil {
		return nil, err
	}
	metadata := make(map[string]string, 0)
	if nodeClass.Spec.MetaData != nil {
		metadata = nodeClass.Spec.MetaData
	}
	// insert max pod and subnet info
	if metadata["oke-native-pod-networking"] == "true" {
		metadata["oke-max-pods"] = fmt.Sprint(instanceType.Capacity.Pods().Value())
		metadata["pod-subnets"] = utils.ToString(subnets[0].Id)
	}
	userdata, err := template[0].UserData.Script()
	if err != nil {
		return nil, err
	}
	metadata["user_data"] = userdata

	req := core.LaunchInstanceRequest{LaunchInstanceDetails: core.LaunchInstanceDetails{
		// todo subnet id balance
		CreateVnicDetails:       &core.CreateVnicDetails{SubnetId: subnets[0].Id, NsgIds: sgsIds},
		LaunchVolumeAttachments: blockDevices,
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(template[0].ImageId),
			BootVolumeVpusPerGB: common.Int64(nodeClass.Spec.BootConfig.BootVolumeVpusPerGB),
			BootVolumeSizeInGBs: common.Int64(nodeClass.Spec.BootConfig.BootVolumeSizeInGBs)},
		DefinedTags:        map[string]map[string]interface{}{options.FromContext(ctx).TagNamespace: getTags(ctx, nodeClass, nodeClaim)},
		CompartmentId:      common.String(options.FromContext(ctx).CompartmentId),
		DisplayName:        common.String(nodeClaim.Name),
		AvailabilityDomain: common.String(ad),
		Shape:              common.String(instanceType.Name),
		Metadata:           metadata,
		InstanceOptions:    &core.InstanceOptions{AreLegacyImdsEndpointsDisabled: common.Bool(true)},
	}}

	// for flexible instance, specify the ocpu and memory
	if instanceType.Requirements.Get(v1alpha1.LabelIsFlexible).Has("true") {
		vcpuVal := instanceType.Requirements.Get(v1alpha1.LabelInstanceCPU).Values()
		memoryInMiVal := instanceType.Requirements.Get(v1alpha1.LabelInstanceMemory).Values()
		if len(vcpuVal) == 0 || len(memoryInMiVal) == 0 {
			return nil, fmt.Errorf("failed to calculate cpu and memory for flex instance when creating instance, nodecliam: %s", nodeClaim.Name)
		}
		vcpu, _ := strconv.Atoi(vcpuVal[0])
		memoryInMi, _ := strconv.Atoi(memoryInMiVal[0])
		req.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			MemoryInGBs: common.Float32(float32(memoryInMi / 1024)),
			Ocpus:       common.Float32(float32(vcpu / 2.0))}
	}
	if nodeClass.Spec.LaunchOptions != nil {
		launchOpts, err := utils.ConvertLaunchOptions(nodeClass.Spec.LaunchOptions)
		req.LaunchOptions = launchOpts
		if err != nil {
			return nil, err
		}
	}
	if len(nodeClass.Spec.AgentList) != 0 {
		req.AgentConfig = &core.LaunchInstanceAgentConfigDetails{
			PluginsConfig: lo.Map(nodeClass.Spec.AgentList, func(item string, index int) core.InstanceAgentPluginConfigDetails {
				return core.InstanceAgentPluginConfigDetails{Name: common.String(item), DesiredState: core.InstanceAgentPluginConfigDetailsDesiredStateEnabled}
			}),
		}
	}
	// Send the request using the service client
	resp, err := p.compClient.LaunchInstance(ctx, req)
	if err != nil {
		p.updateUnavailableOfferingsCache(ctx, err, instanceType.Name, zone)
		return nil, err
	}
	return &resp.Instance, nil
}

func getTags(ctx context.Context, nodeClass *v1alpha1.OciNodeClass, nodeClaim *corev1.NodeClaim) map[string]interface{} {
	staticTags := map[string]string{
		corev1.NodePoolLabelKey:         nodeClaim.Labels[corev1.NodePoolLabelKey],
		v1alpha1.ManagedByAnnotationKey: options.FromContext(ctx).ClusterName,
		v1alpha1.LabelNodeClass:         nodeClass.Name,
	}
	return removeExcludingChars(48, staticTags, nodeClass.Spec.Tags)
}

// https://docs.oracle.com/en-us/iaas/Content/Tagging/Concepts/taggingoverview.htm#limits
func removeExcludingChars(sizeLimit int, tags ...map[string]string) map[string]interface{} {
	res := make(map[string]interface{}, len(tags))
	for _, tagPair := range tags {
		for key, val := range tagPair {
			res[utils.SafeTagKey(key)] = val
			if len(res) >= sizeLimit {
				return res
			}
		}
	}
	return res
}

// todo verify with oci response
func (p *Provider) updateUnavailableOfferingsCache(ctx context.Context, err error, instancetype string, zone string) {
	if corecloudprovider.IsInsufficientCapacityError(err) {
		p.unavailableOfferings.MarkUnavailableForLaunchInstanceErr(ctx, err, corev1.CapacityTypeOnDemand, instancetype, zone)
	}
}

func pickBestInstanceType(nodeClaim *corev1.NodeClaim, instanceTypes corecloudprovider.InstanceTypes) (*corecloudprovider.InstanceType, string) {
	if len(instanceTypes) == 0 {
		return nil, ""
	}
	sortedInstanceType := instanceTypes.OrderByPrice(scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...))
	instanceType := sortedInstanceType[0]
	// Zone - ideally random/spread from requested zones that support given Priority
	requestedZones := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...).Get(v1.LabelTopologyZone)
	priorityOfferings := lo.Filter(instanceType.Offerings.Available(), func(o *corecloudprovider.Offering, _ int) bool {
		return requestedZones.Has(o.Requirements.Get(v1.LabelTopologyZone).Any())
	})
	if len(priorityOfferings) == 0 {
		return nil, ""
	}
	zonesWithPriority := lo.Map(priorityOfferings, func(o *corecloudprovider.Offering, _ int) string {
		return o.Requirements.Get(v1.LabelTopologyZone).Any()
	})
	mutable.Shuffle(zonesWithPriority)
	return instanceType, zonesWithPriority[0]
}

func (p *Provider) Delete(ctx context.Context, id string) error {
	req := core.TerminateInstanceRequest{
		InstanceId:                         common.String(id),
		PreserveBootVolume:                 common.Bool(false),
		PreserveDataVolumesCreatedAtLaunch: common.Bool(false)}
	resp, err := p.compClient.TerminateInstance(ctx, req)
	if err != nil {
		return err
	}
	if resp.HTTPResponse().StatusCode == http.StatusNotFound || resp.HTTPResponse().StatusCode == http.StatusNoContent {
		return corecloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("instance already terminated"))
	}
	return nil
}

func (p *Provider) Get(ctx context.Context, id string) (*core.Instance, error) {
	out, err := p.compClient.GetInstance(ctx, core.GetInstanceRequest{InstanceId: common.String(id)})
	if err != nil {
		return nil, fmt.Errorf("failed to get instances, %w", err)
	}
	if out.RawResponse != nil && out.RawResponse.StatusCode == http.StatusNotFound {
		return nil, corecloudprovider.NewNodeClaimNotFoundError(err)
	}
	if out.LifecycleState == core.InstanceLifecycleStateTerminated {
		return nil, corecloudprovider.NewNodeClaimNotFoundError(err)
	}
	return &out.Instance, nil
}

func (p *Provider) List(ctx context.Context) ([]core.Instance, error) {
	nextPage := "0"
	instances := make([]core.Instance, 0)
	for nextPage != "" {
		if nextPage == "0" {
			nextPage = ""
		}
		req := core.ListInstancesRequest{SortBy: core.ListInstancesSortByTimecreated,
			Limit:         common.Int(50),
			Page:          common.String(nextPage),
			SortOrder:     core.ListInstancesSortOrderDesc,
			CompartmentId: common.String(options.FromContext(ctx).CompartmentId)}

		// Send the request using the service client
		resp, err := p.compClient.ListInstances(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("list instances error, %w", err)
		}
		inses := lo.FilterMap[core.Instance, core.Instance](resp.Items, func(item core.Instance, index int) (core.Instance, bool) {
			val, found := item.DefinedTags[options.FromContext(ctx).TagNamespace][utils.SafeTagKey(v1alpha1.ManagedByAnnotationKey)]
			return item, found && val == options.FromContext(ctx).ClusterName
		})
		instances = append(instances, inses...)
		if resp.OpcNextPage != nil {
			nextPage = *resp.OpcNextPage
		} else {
			nextPage = ""
		}
	}
	return instances, nil
}

func (p *Provider) GetVnicAttachments(ctx context.Context, instance *core.Instance) ([]core.VnicAttachment, error) {
	getVnicReq := core.ListVnicAttachmentsRequest{
		CompartmentId: common.String(options.FromContext(ctx).CompartmentId),
		InstanceId:    common.String(*instance.Id),
	}

	resp, err := p.compClient.ListVnicAttachments(ctx, getVnicReq)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

func (p *Provider) GetSubnets(ctx context.Context, vnics []core.VnicAttachment, onlyPrimaryNic bool) ([]core.Subnet, error) {

	return p.subnetProvider.GetSubnets(ctx, vnics, onlyPrimaryNic)
}

func (p *Provider) GetSecurityGroups(ctx context.Context, vnics []core.VnicAttachment, onlyPrimaryNic bool) ([]core.NetworkSecurityGroup, error) {

	return p.securityGroupProvider.GetSecurityGroups(ctx, vnics, onlyPrimaryNic)
}
