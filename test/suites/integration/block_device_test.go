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

package integration_test

import (
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BlockDeviceMappings", func() {
	It("should use specified block device mappings", func() {
		nodeClass.Spec.BlockDevices = []*v1alpha1.VolumeAttributes{
			{
				SizeInGBs: 100,
				VpusPerGB: 10,
			},
		}
		pod := test.Pod()

		env.ExpectCreated(pod, nodeClass, nodePool)
		env.EventuallyExpectHealthy(pod)
		env.ExpectCreatedNodeCount("==", 1)
		instance := env.GetInstance(pod.Spec.NodeName)
		attach := env.GetVolumeAttach(lo.FromPtr(instance.Id))
		volumes := env.GetVolumes(lo.Map(attach, func(item core.VolumeAttachment, index int) string {
			return lo.FromPtr(item.GetVolumeId())
		})...)
		Expect(volumes[0]).To(HaveField("SizeInGBs", HaveValue(Equal(int64(100)))))
		Expect(volumes[0]).To(HaveField("VpusPerGB", HaveValue(Equal(int64(10)))))
	})
})
