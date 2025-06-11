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
	"sort"
	"strconv"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/subnet"
	"github.com/zoom/karpenter-oci/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Subnet struct {
	subnetProvider *subnet.Provider
}

func (s *Subnet) Reconcile(ctx context.Context, nodeClass *v1alpha1.OciNodeClass) (reconcile.Result, error) {
	subnets, err := s.subnetProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting subnets, %w", err)
	}
	if len(subnets) == 0 {
		nodeClass.Status.Subnets = nil
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeSubnetsReady, "SubnetsNotFound", "SubnetSelector did not match any Subnets")
		return reconcile.Result{}, nil
	}
	sort.Slice(subnets, func(i, j int) bool {
		return *subnets[i].Id < *subnets[j].Id
	})
	nodeClass.Status.Subnets = lo.Map(subnets, func(ociSubnet core.Subnet, _ int) *v1alpha1.Subnet {
		subnetStatus := &v1alpha1.Subnet{
			Id:   utils.ToString(ociSubnet.Id),
			Name: utils.ToString(ociSubnet.DisplayName),
		}
		v4, v6, err1 := s.subnetProvider.GetSubnetUtilization(ctx, &ociSubnet)
		if err1 != nil {
			log.FromContext(ctx).V(1).Error(err1, "subnetProvider.GetSubnetUtilization failed.", "subnetId", ociSubnet.Id)
		}
		if ociSubnet.CidrBlock != nil {
			subnetStatus.IPv4CidrBlock = *ociSubnet.CidrBlock
		}
		if v4 != nil {
			subnetStatus.IPv4Utilization = strconv.FormatFloat(float64(*v4), 'f', -1, 32)
		}
		if v6 != nil {
			subnetStatus.IPv6Utilization = strconv.FormatFloat(float64(*v6), 'f', -1, 32)
		}
		return subnetStatus
	})
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeSubnetsReady)
	return reconcile.Result{RequeueAfter: time.Minute}, nil
}
