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

package securitygroup_test

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"github.com/zoom/karpenter-oci/pkg/utils"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	coretestv1alpha1 "sigs.k8s.io/karpenter/pkg/test/v1alpha1"
	"sort"
	"sync"

	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context
var stop context.CancelFunc
var env *coretest.Environment
var ociEnv *test.Environment
var nodeClass *v1alpha1.OciNodeClass

func TestOci(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "SecurityGroupProvider")
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
		Spec: v1alpha1.OciNodeClassSpec{SecurityGroupNames: []string{"securityGroup-test1", "securityGroup-test2", "securityGroup-test3"}},
	})
	ociEnv.Reset()
})

var _ = AfterEach(func() {
	ExpectCleanedUp(ctx, env.Client)
})

var _ = Describe("SecurityGroupProvider", func() {
	It("should discover security groups by names", func() {
		nodeClass.Spec.SecurityGroupNames = []string{"securityGroup-test2", "securityGroup-test3"}
		securityGroups, err := ociEnv.SecurityGroupProvider.List(ctx, nodeClass)
		Expect(err).To(BeNil())
		ExpectConsistsOfSecurityGroups([]core.NetworkSecurityGroup{
			{
				Id:          common.String("sg-test2"),
				DisplayName: common.String("securityGroup-test2"),
			},
			{
				Id:          common.String("sg-test3"),
				DisplayName: common.String("securityGroup-test3"),
			},
		}, securityGroups)
	})
	Context("Provider Cache", func() {
		It("should resolve security groups from cache that are filtered by name", func() {
			expectedSecurityGroups := ociEnv.VcnCli.ListSecurityGroupOutput.Clone().Items
			for _, sg := range expectedSecurityGroups {
				nodeClass.Spec.SecurityGroupNames = []string{utils.ToString(sg.DisplayName)}
				// Call list to request from oci and store in the cache
				_, err := ociEnv.SecurityGroupProvider.List(ctx, nodeClass)
				Expect(err).To(BeNil())
			}

			for _, cachedObject := range ociEnv.SecurityGroupCache.Items() {
				cachedSecurityGroup := cachedObject.Object.([]core.NetworkSecurityGroup)
				Expect(cachedSecurityGroup).To(HaveLen(2))
				lo.ContainsBy(expectedSecurityGroups, func(item core.NetworkSecurityGroup) bool {
					return lo.FromPtr(item.Id) == lo.FromPtr(expectedSecurityGroups[0].Id)
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
				securityGroups, err := ociEnv.SecurityGroupProvider.List(ctx, nodeClass)
				Expect(err).ToNot(HaveOccurred())

				Expect(securityGroups).To(HaveLen(3))
				// Sort everything in parallel and ensure that we don't get data races
				sort.Slice(securityGroups, func(i, j int) bool {
					return *securityGroups[i].Id < *securityGroups[j].Id
				})
				Expect(securityGroups).To(BeEquivalentTo([]core.NetworkSecurityGroup{
					{
						CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
						Id:             common.String("sg-test1"),
						LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
						DisplayName:    common.String("securityGroup-test1"),
					},
					{
						CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
						Id:             common.String("sg-test2"),
						LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
						DisplayName:    common.String("securityGroup-test2"),
					},
					{
						CompartmentId:  common.String("ocid1.compartment.oc1..aaaaaaaa"),
						Id:             common.String("sg-test3"),
						LifecycleState: core.NetworkSecurityGroupLifecycleStateAvailable,
						DisplayName:    common.String("securityGroup-test3"),
					},
				}))
			}()
		}
		wg.Wait()
	})
})

func ExpectConsistsOfSecurityGroups(expected, actual []core.NetworkSecurityGroup) {
	GinkgoHelper()
	Expect(actual).To(HaveLen(len(expected)))
	for _, elem := range expected {
		_, ok := lo.Find(actual, func(s core.NetworkSecurityGroup) bool {
			return lo.FromPtr(s.Id) == lo.FromPtr(elem.Id) &&
				lo.FromPtr(s.DisplayName) == lo.FromPtr(elem.DisplayName)
		})
		Expect(ok).To(BeTrue(), `Expected security group with {"GroupId": %q, "GroupName": %q} to exist`, lo.FromPtr(elem.Id), lo.FromPtr(elem.DisplayName))
	}
}
