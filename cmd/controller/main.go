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

package main

import (
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"

	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
)

func main() {
	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	ociCloudProvider := cloudprovider.New(
		op.InstanceTypesProvider,
		op.InstanceProvider,
		op.EventRecorder,
		op.GetClient(),
		op.ImageProvider,
	)
	lo.Must0(op.AddHealthzCheck("cloud-provider", ociCloudProvider.LivenessProbe))
	cloudProvider := metrics.Decorate(ociCloudProvider)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)
	coreControllers := corecontrollers.NewControllers(
		ctx,
		op.Manager,
		op.Clock,
		op.GetClient(),
		op.EventRecorder,
		cloudProvider,
		clusterState,
	)
	op.
		WithControllers(ctx, coreControllers...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.GetClient(),
			cloudProvider,
			op.EventRecorder,
			op.ImageProvider,
			op.SubnetProvider,
			op.SecurityGroupProvider,
			op.PricingProvider,
		)...).
		Start(ctx)
}
