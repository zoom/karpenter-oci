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
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
)

var _ = Describe("NodeClass Image Status Controller", func() {
	BeforeEach(func() {
		nodeClass = test.OciNodeClass()
		ociEnv.CmpCli.ListImagesOutput.Set(&core.ListImagesResponse{Items: []core.Image{{
			CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
			Id:             common.String("ocid1.image.oc1..aaaaaaaa"),
			LifecycleState: core.ImageLifecycleStateAvailable,
			DisplayName:    common.String("Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679"),
		}}})
	})
	It("should resolve a valid AMI selector", func() {
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{
			Name:          "Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679",
			CompartmentId: "ocid1.compartment.oc1..aaaaaaaa"}}
		ExpectApplied(ctx, env.Client, nodeClass)
		ExpectObjectReconciled(ctx, env.Client, statusController, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		Expect(nodeClass.Status.Images).To(Equal(
			[]*v1alpha1.Image{{
				Id:            "ocid1.image.oc1..aaaaaaaa",
				Name:          "Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679",
				CompartmentId: "ocid1.compartment.oc1..aaaaaaaa",
			},
			},
		))
		Expect(nodeClass.StatusConditions().IsTrue(v1alpha1.ConditionTypeImageReady)).To(BeTrue())
	})
	It("should get error when resolving images and have status condition set to false", func() {
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{
			Name:          "fake-image-name",
			CompartmentId: "ocid1.compartment.oc1..aaaaaaaa"}}
		ExpectApplied(ctx, env.Client, nodeClass)
		nodeClass = ExpectExists(ctx, env.Client, nodeClass)
		Expect(nodeClass.StatusConditions().IsTrue(v1alpha1.ConditionTypeImageReady)).To(BeFalse())
	})
})
