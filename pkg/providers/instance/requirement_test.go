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
	"fmt"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"testing"
)

func TestRequirement(t *testing.T) {
	mapp := map[string]string{
		"failure-domain.beta.kubernetes.io/zone": "FAULT-DOMAIN-3",
		"topology.kubernetes.io/zone":            "US-ASHBURN-AD-3"}
	req := scheduling.NewLabelRequirements(mapp)
	for key, val := range req {
		fmt.Printf("key: %s, value: %s\n", key, val.Values())
	}
}

func TestFlexibleRequirement(t *testing.T) {
	requirements := scheduling.NewRequirements(scheduling.NewRequirement(v1alpha1.LabelIsFlexible, v1.NodeSelectorOpIn, "true"))
	if !requirements.Get(v1alpha1.LabelIsFlexible).Has("true") {
		t.Fail()
		return
	}
	requirements = scheduling.NewRequirements(scheduling.NewRequirement(v1alpha1.LabelIsFlexible, v1.NodeSelectorOpIn, "false"))
	if requirements.Get(v1alpha1.LabelIsFlexible).Has("true") {
		t.Fail()
		return
	}
}
