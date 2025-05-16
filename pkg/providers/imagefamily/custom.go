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
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily/bootstrap"
	v1 "k8s.io/api/core/v1"
)

type Custom struct {
	DefaultFamily
	*Options
}

// UserData returns the default userdata script for the AMI Family
func (c Custom) UserData(_ *v1alpha1.KubeletConfiguration, _ []v1.Taint, _ map[string]string, customUserData *string, _ *string) bootstrap.Bootstrapper {
	return bootstrap.Custom{
		Options: bootstrap.Options{
			CustomUserData: customUserData,
		},
	}
}
