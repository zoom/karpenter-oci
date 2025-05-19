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

package options_test

import (
	"context"
	"flag"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/test"
	"os"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/karpenter/pkg/utils/testing"
)

var ctx context.Context

func TestOptions(t *testing.T) {
	ctx = TestContextWithLogger(t)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Options")
}

var _ = Describe("Options", func() {
	var fs *coreoptions.FlagSet
	var opts *options.Options

	BeforeEach(func() {
		fs = &coreoptions.FlagSet{
			FlagSet: flag.NewFlagSet("karpenter", flag.ContinueOnError),
		}
		opts = &options.Options{}
	})
	AfterEach(func() {
		os.Clearenv()
	})

	It("should correctly override default vars when CLI flags are set", func() {
		opts.AddFlags(fs)
		err := opts.Parse(fs,
			"--cluster-name", "env-cluster",
			"--cluster-endpoint", "https://env-cluster",
			"--cluster-ca-bundle", "env-bundle",
			"--cluster-bootstrap-token", "env-token",
			"--compartment-id", "ocid1.compartment.oc1..aaaaaaaa",
			"--vm-memory-overhead-percent", "0.075",
			"--flex-cpu-mem-ratios", "2,4",
			"--flex-cpu-constrain-list", "2,4,8")
		Expect(err).ToNot(HaveOccurred())
		expectOptionsEqual(opts, test.Options(test.OptionsFields{
			ClusterName:             lo.ToPtr("env-cluster"),
			ClusterEndpoint:         lo.ToPtr("https://env-cluster"),
			ClusterCABundle:         lo.ToPtr("env-bundle"),
			BootStrapToken:          lo.ToPtr("env-token"),
			CompartmentId:           lo.ToPtr("ocid1.compartment.oc1..aaaaaaaa"),
			VMMemoryOverheadPercent: lo.ToPtr[float64](0.075),
			FlexCpuMemRatios:        lo.ToPtr("2,4"),
			FlexCpuConstrainList:    lo.ToPtr("2,4,8"),
		}))
	})
	It("should correctly fallback to env vars when CLI flags aren't set", func() {
		_ = os.Setenv("CLUSTER_NAME", "env-cluster")
		_ = os.Setenv("CLUSTER_ENDPOINT", "https://env-cluster")
		_ = os.Setenv("CLUSTER_CA_BUNDLE", "env-bundle")
		_ = os.Setenv("CLUSTER_BOOTSTRAP_TOKEN", "env-token")
		_ = os.Setenv("COMPARTMENT_ID", "ocid1.compartment.oc1..aaaaaaaa")
		_ = os.Setenv("VM_MEMORY_OVERHEAD_PERCENT", "0.075")
		_ = os.Setenv("FLEX_CPU_MEM_RATIOS", "2,4")
		_ = os.Setenv("FLEX_CPU_CONSTRAIN_LIST", "2,4,8")
		_ = os.Setenv("AVAILABLE_DOMAIN_PREFIX", "env-prefix")

		// Add flags after we set the environment variables so that the parsing logic correctly refers
		// to the new environment variable values
		opts.AddFlags(fs)
		err := opts.Parse(fs)
		Expect(err).ToNot(HaveOccurred())
		expectOptionsEqual(opts, test.Options(test.OptionsFields{
			ClusterName:             lo.ToPtr("env-cluster"),
			ClusterEndpoint:         lo.ToPtr("https://env-cluster"),
			ClusterCABundle:         lo.ToPtr("env-bundle"),
			BootStrapToken:          lo.ToPtr("env-token"),
			CompartmentId:           lo.ToPtr("ocid1.compartment.oc1..aaaaaaaa"),
			VMMemoryOverheadPercent: lo.ToPtr[float64](0.075),
			FlexCpuMemRatios:        lo.ToPtr("2,4"),
			FlexCpuConstrainList:    lo.ToPtr("2,4,8"),
		}))
	})

	Context("Validation", func() {
		BeforeEach(func() {
			opts.AddFlags(fs)
		})
		It("should fail when cluster name is not set", func() {
			err := opts.Parse(fs)
			Expect(err).To(HaveOccurred())
		})
		It("should fail when clusterEndpoint is invalid (not absolute)", func() {
			err := opts.Parse(fs, "--cluster-name", "test-cluster", "--cluster-endpoint", "00000000000000000000000.oracle.com")
			Expect(err).To(HaveOccurred())
		})
		It("should fail when vmMemoryOverheadPercent is negative", func() {
			err := opts.Parse(fs, "--cluster-name", "test-cluster", "--vm-memory-overhead-percent", "-0.01")
			Expect(err).To(HaveOccurred())
		})
	})
})

func expectOptionsEqual(optsA *options.Options, optsB *options.Options) {
	GinkgoHelper()
	Expect(optsA.ClusterName).To(Equal(optsB.ClusterName))
	Expect(optsA.ClusterEndpoint).To(Equal(optsB.ClusterEndpoint))
	Expect(optsA.ClusterCABundle).To(Equal(optsB.ClusterCABundle))
	Expect(optsA.BootStrapToken).To(Equal(optsB.BootStrapToken))
	Expect(optsA.CompartmentId).To(Equal(optsB.CompartmentId))
	Expect(optsA.VMMemoryOverheadPercent).To(Equal(optsB.VMMemoryOverheadPercent))
	Expect(optsA.FlexCpuMemRatios).To(Equal(optsB.FlexCpuMemRatios))
	Expect(optsA.FlexCpuConstrainList).To(Equal(optsB.FlexCpuConstrainList))
	Expect(optsA.AvailableDomains).To(Equal(optsB.AvailableDomains))
}
