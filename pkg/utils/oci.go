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
	"errors"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"strings"
)

func ConvertLaunchOptions(m *v1alpha1.LaunchOptions) (*core.LaunchOptions, error) {
	errMessage := []string{}
	ociLaunchOptions := &core.LaunchOptions{}
	if val, ok := core.GetMappingLaunchOptionsBootVolumeTypeEnum(m.BootVolumeType); !ok && m.BootVolumeType != "" {
		errMessage = append(errMessage, fmt.Sprintf("unsupported enum value for BootVolumeType: %s. Supported values are: %s.", m.BootVolumeType, strings.Join(core.GetLaunchOptionsBootVolumeTypeEnumStringValues(), ",")))
	} else {
		ociLaunchOptions.BootVolumeType = val
	}
	if val, ok := core.GetMappingLaunchOptionsFirmwareEnum(m.Firmware); !ok && m.Firmware != "" {
		errMessage = append(errMessage, fmt.Sprintf("unsupported enum value for Firmware: %s. Supported values are: %s.", m.Firmware, strings.Join(core.GetLaunchOptionsFirmwareEnumStringValues(), ",")))
	} else {
		ociLaunchOptions.Firmware = val
	}
	if val, ok := core.GetMappingLaunchOptionsNetworkTypeEnum(m.NetworkType); !ok && m.NetworkType != "" {
		errMessage = append(errMessage, fmt.Sprintf("unsupported enum value for NetworkType: %s. Supported values are: %s.", m.NetworkType, strings.Join(core.GetLaunchOptionsNetworkTypeEnumStringValues(), ",")))
	} else {
		ociLaunchOptions.NetworkType = val
	}
	if val, ok := core.GetMappingLaunchOptionsRemoteDataVolumeTypeEnum(m.RemoteDataVolumeType); !ok && m.RemoteDataVolumeType != "" {
		errMessage = append(errMessage, fmt.Sprintf("unsupported enum value for RemoteDataVolumeType: %s. Supported values are: %s.", m.RemoteDataVolumeType, strings.Join(core.GetLaunchOptionsRemoteDataVolumeTypeEnumStringValues(), ",")))
	} else {
		ociLaunchOptions.RemoteDataVolumeType = val
	}
	ociLaunchOptions.IsConsistentVolumeNamingEnabled = lo.ToPtr(m.IsConsistentVolumeNamingEnabled)
	if len(errMessage) > 0 {
		return ociLaunchOptions, errors.New(strings.Join(errMessage, "\n"))
	}
	return ociLaunchOptions, nil
}

func SafeTagKey(origin string) string {
	return strings.ReplaceAll(strings.ReplaceAll(origin, ".", "_"), " ", "_")
}
