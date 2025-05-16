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
	"github.com/Pallinder/go-randomdata"
	"github.com/google/uuid"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/api"
	"github.com/zoom/karpenter-oci/pkg/providers/instancetype"
	"net/http"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/utils/atomic"
	"strings"
	"sync"
	"time"
)

type CmpCli struct {
	CmpBehavior
}

type CapacityPool struct {
	CapacityType string
	InstanceType string
	Zone         string
}

// CmpBehavior must be reset between tests otherwise tests will
// pollute each other.
type CmpBehavior struct {
	ListImagesOutput            AtomicPtr[core.ListImagesResponse]
	DescribeInstanceTypesOutput AtomicPtrSlice[instancetype.WrapShape]
	LaunchInstanceBehavior      MockedFunction[core.LaunchInstanceRequest, core.LaunchInstanceResponse]
	TerminateInstancesBehavior  MockedFunction[core.TerminateInstanceRequest, core.TerminateInstanceResponse]
	GetInstanceBehavior         MockedFunction[core.GetInstanceRequest, core.GetInstanceResponse]
	ListInstanceBehavior        MockedFunction[core.ListInstancesRequest, core.ListInstancesResponse]
	CalledWithListImagesInput   AtomicPtrSlice[core.ListImagesRequest]
	Instances                   sync.Map
	InsufficientCapacityPools   atomic.Slice[CapacityPool]
}

var defaultDescribeInstanceTypesOutput = core.ListShapesResponse{
	Items: []core.Shape{
		{Shape: common.String("shape-1"), IsFlexible: common.Bool(false), Ocpus: common.Float32(1), MemoryInGBs: common.Float32(4),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)},
		{Shape: common.String("shape-2"), IsFlexible: common.Bool(false), Ocpus: common.Float32(2), MemoryInGBs: common.Float32(8),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)},
		{Shape: common.String("shape-3"), IsFlexible: common.Bool(false), Ocpus: common.Float32(4), MemoryInGBs: common.Float32(16),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)},
		{Shape: common.String("shape-4"), IsFlexible: common.Bool(false), Ocpus: common.Float32(8), MemoryInGBs: common.Float32(32),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2)},
		{Shape: common.String("shape-gpu"), IsFlexible: common.Bool(false), Ocpus: common.Float32(2), MemoryInGBs: common.Float32(8),
			NetworkingBandwidthInGbps: common.Float32(10), MaxVnicAttachments: common.Int(2), Gpus: common.Int(1), GpuDescription: common.String("A100")},
	},
}

var _ api.ComputeClient = &CmpCli{}

func NewCmpCli() *CmpCli {
	return &CmpCli{}
}

func (c *CmpCli) ListImages(ctx context.Context, request core.ListImagesRequest) (response core.ListImagesResponse, err error) {
	c.CalledWithListImagesInput.Add(&request)
	if !c.ListImagesOutput.IsNil() {
		describeImagesOutput := c.ListImagesOutput.Clone()
		describeImagesOutput.Items = FilterDescribeImages(describeImagesOutput.Items, *request.DisplayName)
		return *describeImagesOutput, nil
	}
	if *request.DisplayName == "invalid" {
		return core.ListImagesResponse{}, nil
	}
	return core.ListImagesResponse{
		Items: []core.Image{{
			Id:          common.String(fmt.Sprintf("ocid1.image.oc1.iad.%s", randomdata.Alphanumeric(10))),
			DisplayName: common.String("ubuntu")}},
	}, nil
}

func FilterDescribeImages(images []core.Image, name string) []core.Image {
	return lo.Filter(images, func(image core.Image, _ int) bool {
		return *image.DisplayName == name
	})
}

func (c *CmpCli) LaunchInstance(ctx context.Context, request core.LaunchInstanceRequest) (response core.LaunchInstanceResponse, err error) {
	ptr, err := c.LaunchInstanceBehavior.Invoke(&request, func(input *core.LaunchInstanceRequest) (*core.LaunchInstanceResponse, error) {
		var insufficientErr error
		c.InsufficientCapacityPools.Range(func(pool CapacityPool) bool {
			if pool.InstanceType == lo.FromPtr(request.Shape) && pool.Zone == strings.Split(lo.FromPtr(request.AvailabilityDomain), ":")[1] {
				insufficientErr = corecloudprovider.NewInsufficientCapacityError(fmt.Errorf("instance type is insufficient"))
				return false
			}
			return true
		})
		if insufficientErr != nil {
			return nil, insufficientErr
		}
		instance := &core.Instance{
			Id:                 common.String(uuid.New().String()),
			Shape:              request.Shape,
			AvailabilityDomain: request.AvailabilityDomain,
			FaultDomain:        common.String("FAULT-DOMAIN-1"),
			TimeCreated:        &common.SDKTime{Time: time.Now()},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: common.String("ocid1.image.oc1.iad.aaaaaaaa"),
			},
		}
		c.Instances.Store(*instance.Id, instance)

		result := &core.LaunchInstanceResponse{
			Instance: *instance,
		}
		return result, nil
	})
	if err != nil {
		return core.LaunchInstanceResponse{}, err
	}
	return *ptr, nil
}

func (c *CmpCli) TerminateInstance(ctx context.Context, request core.TerminateInstanceRequest) (response core.TerminateInstanceResponse, err error) {
	ptr, err := c.TerminateInstancesBehavior.Invoke(&request, func(input *core.TerminateInstanceRequest) (*core.TerminateInstanceResponse, error) {
		var resp *core.TerminateInstanceResponse
		instanceID := *input.InstanceId
		if _, ok := c.Instances.LoadAndDelete(instanceID); ok {
			resp = &core.TerminateInstanceResponse{}
		}
		return resp, nil
	})
	return *ptr, err
}

func (c *CmpCli) GetInstance(ctx context.Context, request core.GetInstanceRequest) (response core.GetInstanceResponse, err error) {
	ptr, err := c.GetInstanceBehavior.Invoke(&request, func(input *core.GetInstanceRequest) (*core.GetInstanceResponse, error) {
		instance, ok := c.Instances.Load(*input.InstanceId)
		if !ok {
			return &core.GetInstanceResponse{RawResponse: &http.Response{StatusCode: http.StatusNotFound}}, corecloudprovider.NewNodeClaimNotFoundError(err)
		}
		return &core.GetInstanceResponse{
			RawResponse: &http.Response{StatusCode: http.StatusOK},
			Instance:    *(instance.(*core.Instance)),
		}, nil
	})
	return *ptr, err
}

func (c *CmpCli) ListInstances(ctx context.Context, request core.ListInstancesRequest) (response core.ListInstancesResponse, err error) {
	ptr, err := c.ListInstanceBehavior.Invoke(&request, func(input *core.ListInstancesRequest) (*core.ListInstancesResponse, error) {
		var instances []*core.Instance
		c.Instances.Range(func(k interface{}, v interface{}) bool {
			ins := v.(*core.Instance)
			instances = append(instances, ins)
			return true
		})

		return &core.ListInstancesResponse{
			RawResponse: nil,
			Items: lo.FlatMap[*core.Instance, core.Instance](instances, func(item *core.Instance, index int) []core.Instance {
				return []core.Instance{*item}
			}),
			OpcNextPage:  nil,
			OpcRequestId: nil,
		}, nil
	})
	return *ptr, err
}

func (c *CmpCli) ListShapes(ctx context.Context, request core.ListShapesRequest) (response core.ListShapesResponse, err error) {
	items := make([]core.Shape, 0)
	if c.DescribeInstanceTypesOutput.Len() != 0 {
		c.DescribeInstanceTypesOutput.ForEach(func(c *instancetype.WrapShape) {
			items = append(items, c.Shape)
		})
		return core.ListShapesResponse{
			Items: items,
		}, nil
	}
	return defaultDescribeInstanceTypesOutput, nil
}

func (c *CmpCli) Reset() {
	c.ListImagesOutput.Reset()
	c.DescribeInstanceTypesOutput.Reset()
	c.LaunchInstanceBehavior.Reset()
	c.TerminateInstancesBehavior.Reset()
	c.GetInstanceBehavior.Reset()
	c.ListInstanceBehavior.Reset()
	c.CalledWithListImagesInput.Reset()
	c.Instances.Range(func(k, v any) bool {
		c.Instances.Delete(k)
		return true
	})
	c.InsufficientCapacityPools.Reset()
}
