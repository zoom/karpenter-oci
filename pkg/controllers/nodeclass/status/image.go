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
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"github.com/zoom/karpenter-oci/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sort"
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
	sortImages := lo.Map(images, func(image internalmodel.WrapImage, _ int) *v1alpha1.Image {
		reqs := lo.Map(image.Requirements.NodeSelectorRequirements(), func(item karpv1.NodeSelectorRequirementWithMinValues, _ int) corev1.NodeSelectorRequirement {
			return item.NodeSelectorRequirement
		})

		sort.Slice(reqs, func(i, j int) bool {
			if len(reqs[i].Key) != len(reqs[j].Key) {
				return len(reqs[i].Key) < len(reqs[j].Key)
			}
			return reqs[i].Key < reqs[j].Key
		})
		return &v1alpha1.Image{
			Id:            utils.ToString(image.Image.Id),
			Name:          utils.ToString(image.Image.DisplayName),
			CompartmentId: utils.ToString(image.Image.CompartmentId),
			Requirements:  reqs,
		}
	})
	sort.Slice(sortImages, func(i, j int) bool {
		return sortImages[i].Id < sortImages[j].Id
	})
	nodeClass.Status.Images = sortImages
	nodeClass.StatusConditions().SetTrue(v1alpha1.ConditionTypeImageReady)
	return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
