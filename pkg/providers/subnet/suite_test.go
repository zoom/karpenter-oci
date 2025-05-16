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

package subnet_test

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"sigs.k8s.io/karpenter/pkg/test/expectations"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"sort"
	"sync"
	"testing"

	"github.com/samber/lo"

	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var stop context.CancelFunc
var env *coretest.Environment
var ociEnv *test.Environment
var nodeClass *v1alpha1.OciNodeClass

func TestSubnet(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "SubnetProvider")
}

var _ = BeforeSuite(func() {
	env = coretest.NewEnvironment(coretest.WithCRDs(apis.CRDs...), coretest.WithCRDs(coretestv1alpha1.CRDs...), coretest.WithFieldIndexers(test.OciNodeClassFieldIndexer(ctx)))
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	ctx, stop = context.WithCancel(ctx)
	ociEnv = test.NewEnvironment(ctx, env)
})

var _ = AfterSuite(func() {
	stop()
	Expect(env.Stop()).To(Succeed(), "Failed to stop environment")
})

var _ = BeforeEach(func() {
	ctx = coreoptions.ToContext(ctx, coretest.Options())
	ctx = options.ToContext(ctx, test.Options())
	nodeClass = test.OciNodeClass(v1alpha1.OciNodeClass{
		Spec: v1alpha1.OciNodeClassSpec{SubnetName: "private-1"},
	})
	ociEnv.Reset()
})

var _ = AfterEach(func() {
	expectations.ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("SubnetProvider", func() {
	Context("List", func() {
		It("should discover subnet by name", func() {
			nodeClass.Spec.SubnetName = "private-1"
			subnets, err := ociEnv.SubnetProvider.List(ctx, nodeClass)
			Expect(err).To(BeNil())
			ExpectConsistsOfSubnets([]core.Subnet{
				{
					Id:          lo.ToPtr("ocid1.subnet.oc1.iad.aaaaaaaa"),
					DisplayName: lo.ToPtr("private-1"),
				},
				{
					Id:          lo.ToPtr("ocid1.subnet.oc1.iad.aaaaaaab"),
					DisplayName: lo.ToPtr("private-1"),
				},
			}, subnets)
		})
	})
	Context("Provider Cache", func() {
		It("should resolve subnets from cache that are filtered by name", func() {
			expectedSubnets := ociEnv.VcnCli.ListSubnetsOutput.Clone().Items
			for _, subnet := range expectedSubnets {
				nodeClass.Spec.SubnetName = lo.FromPtr[string](subnet.DisplayName)
				// Call list to request from aws and store in the cache
				_, err := ociEnv.SubnetProvider.List(ctx, nodeClass)
				Expect(err).To(BeNil())
			}

			for _, cachedObject := range ociEnv.SubnetCache.Items() {
				cachedSubnet := cachedObject.Object.([]core.Subnet)
				Expect(cachedSubnet).To(HaveLen(1))
				lo.ContainsBy(expectedSubnets, func(item core.Subnet) bool {
					return lo.FromPtr(item.Id) == lo.FromPtr(cachedSubnet[0].Id)
				})
			}
		})
	})
	It("should not cause data races when calling List() simultaneously", func() {
		wg := sync.WaitGroup{}
		for i := 0; i < 10000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				subnets, err := ociEnv.SubnetProvider.List(ctx, nodeClass)
				Expect(err).ToNot(HaveOccurred())

				Expect(subnets).To(HaveLen(2))
				// Sort everything in parallel and ensure that we don't get data races
				sort.Slice(subnets, func(i, j int) bool {
					return *subnets[i].Id < *subnets[j].Id
				})
				Expect(subnets).To(BeEquivalentTo([]core.Subnet{
					{
						CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
						Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaaa"),
						LifecycleState: core.SubnetLifecycleStateAvailable,
						DisplayName:    common.String("private-1"),
					},
					{
						CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
						Id:             common.String("ocid1.subnet.oc1.iad.aaaaaaab"),
						LifecycleState: core.SubnetLifecycleStateAvailable,
						DisplayName:    common.String("private-1"),
					},
				}))
			}()
		}
		wg.Wait()
	})
})

func ExpectConsistsOfSubnets(expected, actual []core.Subnet) {
	GinkgoHelper()
	Expect(actual).To(HaveLen(len(expected)))
	for _, elem := range expected {
		_, ok := lo.Find(actual, func(s core.Subnet) bool {
			return lo.FromPtr(s.Id) == lo.FromPtr(elem.Id) &&
				lo.FromPtr(s.DisplayName) == lo.FromPtr(elem.DisplayName)
		})
		Expect(ok).To(BeTrue(), `Expected subnet with {"SubnetId": %q, "AvailabilityZone": %q} to exist`, lo.FromPtr(elem.Id), lo.FromPtr(elem.DisplayName))
	}
}
