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
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

func FindCompatibleInstanceType(instanceTypes []*cloudprovider.InstanceType, imgs []*v1alpha1.Image) map[string][]*cloudprovider.InstanceType {

	imgIDS := map[string][]*cloudprovider.InstanceType{}
	for _, instanceType := range instanceTypes {
		for _, ami := range imgs {

			amiRequirement := scheduling.NewNodeSelectorRequirements() // mock the image requirement since image does not support requirements yet
			if err := instanceType.Requirements.Compatible(
				amiRequirement,
				scheduling.AllowUndefinedWellKnownLabels,
			); err == nil {
				imgIDS[ami.Id] = append(imgIDS[ami.Id], instanceType)
				break
			}
		}
	}
	return imgIDS
}
