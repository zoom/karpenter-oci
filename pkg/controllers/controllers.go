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

package controllers

import (
	"context"
	"github.com/awslabs/operatorpkg/controller"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclaim/garbagecollection"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclass/hash"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclass/status"
	"github.com/zoom/karpenter-oci/pkg/controllers/nodeclass/termination"
	controllerPricing "github.com/zoom/karpenter-oci/pkg/controllers/providers/pricing"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	"github.com/zoom/karpenter-oci/pkg/providers/pricing"
	"github.com/zoom/karpenter-oci/pkg/providers/securitygroup"
	"github.com/zoom/karpenter-oci/pkg/providers/subnet"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/events"
)

func NewControllers(ctx context.Context, kubeClient client.Client, cloudProvider cloudprovider.CloudProvider,
	recorder events.Recorder, imageProvider *imagefamily.Provider, subnetProvider *subnet.Provider,
	securityProvider *securitygroup.Provider, pricingProvider pricing.Provider) []controller.Controller {
	controllers := []controller.Controller{
		hash.NewController(kubeClient),
		status.NewController(kubeClient, subnetProvider, securityProvider, imageProvider),
		termination.NewController(kubeClient, recorder),
		garbagecollection.NewController(kubeClient, cloudProvider),
		controllerPricing.NewController(pricingProvider),
	}
	return controllers
}
