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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coretest "sigs.k8s.io/karpenter/pkg/test"
)

var _ = Describe("Tags", func() {
	Context("Tagging Controller", func() {
		It("should tag with karpenter.sh/nodeclaim and Name tag", func() {
			pod := coretest.Pod()
			env.ExpectCreated(nodePool, nodeClass, pod)
			env.EventuallyExpectCreatedNodeCount("==", 1)
			node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
			nodeClaim := env.ExpectNodeClaimCount("==", 1)[0]

			nodeInstance := env.GetInstance(node.Name)
			Expect(nodeInstance.DefinedTags[env.TagNamespace]).To(HaveKeyWithValue("karpenter_sh/nodeclaim", nodeClaim.Name))
			Expect(nodeInstance.DefinedTags[env.TagNamespace]).To(HaveKeyWithValue("karpenter_sh/managed-by", env.ClusterName))
		})
	})
})
