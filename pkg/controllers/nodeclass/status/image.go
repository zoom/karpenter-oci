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
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	"github.com/zoom/karpenter-oci/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

type Image struct {
	imageProvider *imagefamily.Provider
}

func (i *Image) Reconcile(ctx context.Context, nodeClass *v1alpha1.OciNodeClass) (reconcile.Result, error) {
	images, err := i.imageProvider.List(ctx, nodeClass)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting amis, %w", err)
	}
	if len(images) == 0 {
		nodeClass.Status.Images = []*v1alpha1.Image{}
		nodeClass.StatusConditions().SetFalse(v1alpha1.ConditionTypeImageReady, "ImageNotFound", "Image spec did not match any images")
		return reconcile.Result{}, nil
	}
	// todo filter the image with the arch requirement, like amd64 and arm64
	nodeClass.Status.Images = lo.Map(images, func(image core.Image, _ int) *v1alpha1.Image {
		return &v1alpha1.Image{
			Id:            utils.ToString(image.Id),
			Name:          utils.ToString(image.DisplayName),
			CompartmentId: utils.ToString(image.CompartmentId),
		}
	})
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeImageReady)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
