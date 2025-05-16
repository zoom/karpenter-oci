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

package status

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/securitygroup"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sort"
	"time"
)

type SecurityGroup struct {
	securityGroupProvider *securitygroup.Provider
}

func (sg *SecurityGroup) Reconcile(ctx context.Context, nodeClass *v1alpha1.OciNodeClass) (reconcile.Result, error) {
	securityGroups, err := sg.securityGroupProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting security groups, %w", err)
	}
	if len(securityGroups) == 0 && len(nodeClass.Spec.SecurityGroupNames) > 0 {
		nodeClass.Status.SecurityGroups = nil
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeSecurityGroupsReady, "SecurityGroupsNotFound", "SecurityGroupSelector did not match any SecurityGroups")
		return reconcile.Result{}, nil
	}
	sort.Slice(securityGroups, func(i, j int) bool {
		return *securityGroups[i].Id < *securityGroups[j].Id
	})
	nodeClass.Status.SecurityGroups = lo.Map(securityGroups, func(securityGroup core.NetworkSecurityGroup, _ int) *v1alpha1.SecurityGroup {
		return &v1alpha1.SecurityGroup{
			Id:   *securityGroup.Id,
			Name: *securityGroup.DisplayName,
		}
	})
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeSecurityGroupsReady)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
