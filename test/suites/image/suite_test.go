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

package image_test

import (
	"encoding/base64"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"net/url"
	"os"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	"strings"
	"testing"
	"time"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/awslabs/operatorpkg/status"
	. "github.com/awslabs/operatorpkg/test/expectations"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	environmentoci "github.com/zoom/karpenter-oci/test/pkg/environment/oci"
	"k8s.io/apimachinery/pkg/types"
)

var env *environmentoci.Environment
var nodeClass *v1alpha1.OciNodeClass
var nodePool *karpv1.NodePool

var imageMaps = map[string]string{
	"Oracle-Linux-8.10-2024.11.30-0-OKE-1.30.1-754": "ocid1.image.oc1.iad.aaaaaaaau3ahhbqeyyfikf27szllwurv7k2w6yo3ffwupmpk4sm6korpq7ra",
	"Oracle-Linux-8.10-2024.09.30-0-OKE-1.30.1-747": "ocid1.image.oc1.iad.aaaaaaaajvtta4i5sq4pwx2375evyqk27kbyjcskfxjwz4vwxz6ersmmax6q",
	"Oracle-Linux-8.10-2024.06.30-0-OKE-1.30.1-716": "ocid1.image.oc1.iad.aaaaaaaa3mw56urimjwfb2cpqzuqffk44nm5q27bm2r2qheiej43jwjp3wfa"}
var imageCompId = "ocid1.compartment.oc1..aaaaaaaab4u67dhgtj5gpdpp3z42xqqsdnufxkatoild46u3hb67vzojfmzq"

func TestAMI(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = environmentoci.NewEnvironment(t)
	})
	AfterSuite(func() {
		env.Stop()
	})
	RunSpecs(t, "Image")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultOciNodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("Image", func() {
	var imageId string
	var imageName string
	BeforeEach(func() {
		imageName = lo.Keys(imageMaps)[0]
		imageId = env.GetImages(imageCompId, []string{imageName})[0]
		fmt.Printf("use image: %s", imageId)
	})

	It("should use the image defined by the image Selector Terms", func() {
		pod := coretest.Pod()
		nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: imageName, CompartmentId: imageCompId}}
		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		env.ExpectInstance(pod.Spec.NodeName).To(HaveField("ImageId", HaveValue(Equal(imageId))))
	})
	It("should support image selector ids", func() {
		nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
		nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{
			{
				Id: imageId,
			},
		}
		pod := coretest.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)

		env.ExpectInstance(pod.Spec.NodeName).To(HaveField("ImageId", HaveValue(Equal(imageId))))
	})

	Context("ImageFamily", func() {
		It("should support Custom ImageFamily with Image Selectors", func() {
			nodeClass.Spec.ImageFamily = v1alpha1.CustomImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: imageName, CompartmentId: imageCompId}}
			rawContent, err := os.ReadFile("testdata/custom_userdata_input.sh")
			Expect(err).ToNot(HaveOccurred())
			url, _ := url.Parse(env.ClusterEndpoint)
			nodeClass.Spec.UserData = lo.ToPtr(fmt.Sprintf(string(rawContent), url.Hostname(), env.ClusterDns, env.ExpectCABundle()))
			pod := coretest.Pod()

			env.ExpectCreated(pod, nodeClass, nodePool)
			env.EventuallyExpectHealthy(pod)
			env.ExpectCreatedNodeCount("==", 1)

			env.ExpectInstance(pod.Spec.NodeName).To(HaveField("ImageId", HaveValue(Equal(imageId))))
		})

		It("should have ocinodeClass status as not ready since Image was not resolved", func() {
			nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: "image-123", CompartmentId: imageCompId}}
			env.ExpectCreated(nodeClass)
			ExpectStatusConditions(env, env.Client, 1*time.Minute, nodeClass, status.Condition{Type: v1alpha1.ConditionTypeImageReady, Status: metav1.ConditionFalse, Message: "Image spec did not match any images"})

		})
	})

	Context("UserData", func() {
		It("should merge UserData contents for OKE image Family", func() {
			content, err := os.ReadFile("testdata/userdata_input.sh")
			Expect(err).ToNot(HaveOccurred())
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: imageName, CompartmentId: imageCompId}}
			nodeClass.Spec.UserData = common.String(string(content))
			nodePool.Spec.Template.Spec.Taints = []corev1.Taint{{Key: "example.com", Value: "value", Effect: "NoExecute"}}
			nodePool.Spec.Template.Spec.StartupTaints = []corev1.Taint{{Key: "example.com", Value: "value", Effect: "NoSchedule"}}
			pod := coretest.Pod(coretest.PodOptions{Tolerations: []corev1.Toleration{{Key: "example.com", Operator: corev1.TolerationOpExists}}})

			env.ExpectCreated(pod, nodeClass, nodePool)
			env.EventuallyExpectHealthy(pod)
			Expect(env.GetNode(pod.Spec.NodeName).Spec.Taints).To(ContainElements(
				corev1.Taint{Key: "example.com", Value: "value", Effect: "NoExecute"},
				corev1.Taint{Key: "example.com", Value: "value", Effect: "NoSchedule"},
			))
			actualUserData, err := base64.StdEncoding.DecodeString(getInstance(pod.Spec.NodeName).Metadata["user_data"])
			Expect(err).ToNot(HaveOccurred())
			// Since the node has joined the cluster, we know our bootstrapping was correct.
			// Just verify if the UserData contains our custom content too, rather than doing a byte-wise comparison.
			Expect(string(actualUserData)).To(ContainSubstring("Running custom user data script"))
		})
		It("should merge non-MIME UserData contents for OKE image Family", func() {
			content, err := os.ReadFile("testdata/no_mime_userdata_input.sh")
			Expect(err).ToNot(HaveOccurred())
			nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: imageName, CompartmentId: imageCompId}}
			nodeClass.Spec.UserData = common.String(string(content))
			nodePool.Spec.Template.Spec.Taints = []corev1.Taint{{Key: "example.com", Value: "value", Effect: "NoExecute"}}
			nodePool.Spec.Template.Spec.StartupTaints = []corev1.Taint{{Key: "example.com", Value: "value", Effect: "NoSchedule"}}
			pod := coretest.Pod(coretest.PodOptions{Tolerations: []corev1.Toleration{{Key: "example.com", Operator: corev1.TolerationOpExists}}})

			env.ExpectCreated(pod, nodeClass, nodePool)
			env.EventuallyExpectHealthy(pod)
			Expect(env.GetNode(pod.Spec.NodeName).Spec.Taints).To(ContainElements(
				corev1.Taint{Key: "example.com", Value: "value", Effect: "NoExecute"},
				corev1.Taint{Key: "example.com", Value: "value", Effect: "NoSchedule"},
			))
			actualUserData, err := base64.StdEncoding.DecodeString(getInstance(pod.Spec.NodeName).Metadata["user_data"])
			Expect(err).ToNot(HaveOccurred())
			// Since the node has joined the cluster, we know our bootstrapping was correct.
			// Just verify if the UserData contains our custom content too, rather than doing a byte-wise comparison.
			Expect(string(actualUserData)).To(ContainSubstring("Running custom user data script"))
		})
	})
})

//nolint:unparam
func getInstance(nodeName string) core.Instance {
	var node corev1.Node
	Expect(env.Client.Get(env.Context, types.NamespacedName{Name: nodeName}, &node)).To(Succeed())
	providerIDSplit := strings.Split(node.Spec.ProviderID, "/")
	instanceID := providerIDSplit[len(providerIDSplit)-1]
	out, err := env.CMPAPI.GetInstance(env.Context, core.GetInstanceRequest{
		InstanceId: common.String(instanceID),
	})
	Expect(err).ToNot(HaveOccurred())
	return out.Instance
}
