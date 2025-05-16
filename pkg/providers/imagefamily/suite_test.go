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

package imagefamily_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
	"testing"
)

var ctx context.Context
var env *coretest.Environment
var ociEnv *test.Environment
var nodeClass *v1alpha1.OciNodeClass

func TestImageFamily(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "AMISelector")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	ociEnv = test.NewEnvironment(ctx, env)
})

var _ = BeforeEach(func() {
	// Set up the DescribeImages API so that we can call it by ID with the mock parameters that we generate
	ociEnv.CmpCli.ListImagesOutput.Set(&core.ListImagesResponse{
		Items: []core.Image{{
			Id:             common.String("ocid1.image.oc1.iad.aaaaaaaa"),
			LifecycleState: core.ImageLifecycleStateAvailable,
			DisplayName:    common.String("Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679")}},
	})
})

var _ = AfterEach(func() {
	ociEnv.Reset()
})

var _ = AfterSuite(func() {
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = Describe("AMIProvider", func() {
	BeforeEach(func() {
		nodeClass = test.OciNodeClass(v1alpha1.OciNodeClass{
			Spec: v1alpha1.OciNodeClassSpec{
				Image: &v1alpha1.Image{Name: "Oracle-Linux-8.9-2024.01.26-0-OKE-1.27.10-679"},
			},
		})
	})
	It("should succeed to resolve oracle linux", func() {
		nodeClass.Spec.ImageFamily = v1alpha1.Ubuntu2204ImageFamily
		amis, err := ociEnv.AMIProvider.List(ctx, nodeClass)
		Expect(err).ToNot(HaveOccurred())
		Expect(amis).To(HaveLen(1))
	})
	It("should succeed to resolve AMIs (Ubuntu)", func() {
		nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
		amis, err := ociEnv.AMIProvider.List(ctx, nodeClass)
		Expect(err).ToNot(HaveOccurred())
		Expect(amis).To(HaveLen(1))
	})
})
