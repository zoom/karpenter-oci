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
	"fmt"
	"github.com/Pallinder/go-randomdata"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func InstanceID() string {
	return fmt.Sprintf("ocid1.instance.oc1.iad.%s", randomdata.Alphanumeric(60))
}

// todo impl me
func MakeInstances() []*internalmodel.WrapShape {
	instanceTypes := make([]*internalmodel.WrapShape, 0)
	return instanceTypes
}

func MakeUniqueInstancesAndFamilies(instances []*cloudprovider.InstanceType, numInstanceFamilies int) ([]*cloudprovider.InstanceType, sets.Set[string]) {
	var instanceTypes []*cloudprovider.InstanceType
	instanceFamilies := sets.Set[string]{}
	for _, it := range instances {
		var found bool
		for instFamily := range instanceFamilies {
			if it.Name == instFamily {
				found = true
				break
			}
		}
		if !found {
			instanceTypes = append(instanceTypes, it)
			instanceFamilies.Insert(it.Name)
			if len(instanceFamilies) == numInstanceFamilies {
				break
			}
		}
	}
	return instanceTypes, instanceFamilies
}
