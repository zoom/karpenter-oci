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

package termination

import (
	"fmt"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/utils"

	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/karpenter/pkg/events"
)

func WaitingOnNodeClaimTerminationEvent(nodeClass *v1alpha1.OciNodeClass, names []string) events.Event {
	return events.Event{
		InvolvedObject: nodeClass,
		Type:           v1.EventTypeNormal,
		Reason:         "WaitingOnNodeClaimTermination",
		Message:        fmt.Sprintf("Waiting on NodeClaim termination for %s", utils.PrettySlice(names, 5)),
		DedupeValues:   []string{string(nodeClass.UID)},
	}
}
