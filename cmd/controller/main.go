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
	operator_alt "github.com/zoom/karpenter-oci/pkg/alt/karpenter-core/pkg/operator"
	"github.com/zoom/karpenter-oci/pkg/controllers"

	"github.com/zoom/karpenter-oci/pkg/cloudprovider"
	"github.com/zoom/karpenter-oci/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corecontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	corewebhooks "sigs.k8s.io/karpenter/pkg/webhooks"
)

func main() {
	ctx, op := operator.NewOperator(operator_alt.NewOperator())

	ociCloudProvider := cloudprovider.New(
		op.InstanceTypesProvider,
		op.InstanceProvider,
		op.EventRecorder,
		op.GetClient(),
		op.ImageProvider,
	)
	lo.Must0(op.AddHealthzCheck("cloud-provider", ociCloudProvider.LivenessProbe))
	cloudProvider := metrics.Decorate(ociCloudProvider)
	coreControllers := corecontrollers.NewControllers(
		ctx,
		op.Manager,
		op.Clock,
		op.GetClient(),
		op.EventRecorder,
		cloudProvider,
	)
	op.
		WithControllers(ctx, coreControllers...).
		WithWebhooks(ctx, corewebhooks.NewWebhooks()...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.GetClient(),
			cloudProvider,
			op.EventRecorder,
			op.ImageProvider,
			op.SubnetProvider,
			op.SecurityGroupProvider,
		)...).
		Start(ctx, cloudProvider)
}
