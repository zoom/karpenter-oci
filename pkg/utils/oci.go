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

package utils

import (
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"strings"
)

// define a constant for preemptible instance
const CapacityTypePreemptible = "preemptible"

func ConvertLaunchOptions(m *v1alpha1.LaunchOptions) (*core.LaunchOptions, error) {
	ociLaunchOptions := &core.LaunchOptions{}
	if m.BootVolumeType != nil {
		ociLaunchOptions.BootVolumeType, _ = core.GetMappingLaunchOptionsBootVolumeTypeEnum(lo.FromPtr(m.BootVolumeType))
	}
	if m.Firmware != nil {
		ociLaunchOptions.Firmware, _ = core.GetMappingLaunchOptionsFirmwareEnum(lo.FromPtr(m.Firmware))
	}
	if m.NetworkType != nil {
		ociLaunchOptions.NetworkType, _ = core.GetMappingLaunchOptionsNetworkTypeEnum(lo.FromPtr(m.NetworkType))
	}
	if m.RemoteDataVolumeType != nil {
		ociLaunchOptions.RemoteDataVolumeType, _ = core.GetMappingLaunchOptionsRemoteDataVolumeTypeEnum(lo.FromPtr(m.RemoteDataVolumeType))
	}
	if m.IsConsistentVolumeNamingEnabled != nil {
		ociLaunchOptions.IsConsistentVolumeNamingEnabled = m.IsConsistentVolumeNamingEnabled
	}
	return ociLaunchOptions, nil
}

func SafeTagKey(origin string) string {
	return strings.ReplaceAll(strings.ReplaceAll(origin, ".", "_"), " ", "_")
}
