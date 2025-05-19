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

type ComputeClient interface {
	GetImage(ctx context.Context, request core.GetImageRequest) (response core.GetImageResponse, err error)
	ListImages(ctx context.Context, request core.ListImagesRequest) (response core.ListImagesResponse, err error)
	LaunchInstance(ctx context.Context, request core.LaunchInstanceRequest) (response core.LaunchInstanceResponse, err error)
	TerminateInstance(ctx context.Context, request core.TerminateInstanceRequest) (response core.TerminateInstanceResponse, err error)
	GetInstance(ctx context.Context, request core.GetInstanceRequest) (response core.GetInstanceResponse, err error)
	ListInstances(ctx context.Context, request core.ListInstancesRequest) (response core.ListInstancesResponse, err error)
	ListShapes(ctx context.Context, request core.ListShapesRequest) (response core.ListShapesResponse, err error)
	ListVnicAttachments(ctx context.Context, request core.ListVnicAttachmentsRequest) (response core.ListVnicAttachmentsResponse, err error)
}
