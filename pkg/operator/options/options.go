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

package options

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/zoom/karpenter-oci/pkg/utils"
	"os"
	coreoptions "sigs.k8s.io/karpenter/pkg/operator/options"
	"sigs.k8s.io/karpenter/pkg/utils/env"
)

func init() {
	coreoptions.Injectables = append(coreoptions.Injectables, &Options{})
}

type optionsKey struct{}

type Options struct {
	ClusterName             string
	ClusterEndpoint         string
	ClusterDns              string
	ClusterCABundle         string
	BootStrapToken          string
	CompartmentId           string
	TagNamespace            string
	VMMemoryOverheadPercent float64
	FlexCpuMemRatios        string
	FlexCpuConstrainList    string
	AvailableDomains        []string
	OciAuthMethods          string
	PriceEndpoint           string
	PriceSyncPeriod         int
	UseLocalPriceList       bool
}

func (o *Options) AddFlags(fs *coreoptions.FlagSet) {
	fs.StringVar(&o.ClusterName, "cluster-name", env.WithDefaultString("CLUSTER_NAME", ""), "[REQUIRED] The kubernetes cluster name for resource discovery.")
	fs.StringVar(&o.ClusterEndpoint, "cluster-endpoint", env.WithDefaultString("CLUSTER_ENDPOINT", ""), "The external kubernetes cluster endpoint for new nodes to connect with. If not specified, will discover the cluster endpoint using DescribeCluster API.")
	fs.StringVar(&o.ClusterDns, "cluster-dns", env.WithDefaultString("CLUSTER_DNS", ""), "clusterDNS is a IP addresses for the cluster DNS server")
	fs.StringVar(&o.ClusterCABundle, "cluster-ca-bundle", env.WithDefaultString("CLUSTER_CA_BUNDLE", ""), "Cluster CA bundle for nodes to use for TLS connections with the API server. If not set, this is taken from the controller's TLS configuration.")
	fs.StringVar(&o.BootStrapToken, "cluster-bootstrap-token", env.WithDefaultString("CLUSTER_BOOTSTRAP_TOKEN", ""), "Cluster bootstrap token for nodes to use for TLS connections with the API server, use bootstrap token generate kube config")
	fs.Float64Var(&o.VMMemoryOverheadPercent, "vm-memory-overhead-percent", utils.WithDefaultFloat64("VM_MEMORY_OVERHEAD_PERCENT", 0.0), "The VM memory overhead as a percent that will be subtracted from the total memory for all instance types.")
	fs.StringVar(&o.FlexCpuMemRatios, "flex-cpu-mem-ratios", env.WithDefaultString("FLEX_CPU_MEM_RATIOS", "4"), "the ratios of vcpu and mem, eg FLEX_CPU_MEM_RATIOS=2,4, if create flex instance with 2 cores(1 ocpu), mem should be 4Gi or 8Gi")
	fs.StringVar(&o.FlexCpuConstrainList, "flex-cpu-constrain-list", env.WithDefaultString("FLEX_CPU_CONSTRAIN_LIST", "1,2,4,8,16,32,48,64,96,128"), "to constrain the ocpu cores of flex instance, instance create in this cpu size list, ocpu is twice of vcpu")
	fs.StringVar(&o.CompartmentId, "compartment-id", env.WithDefaultString("COMPARTMENT_ID", ""), "[REQUIRED] The compartment id to create and list instances")
	fs.StringVar(&o.TagNamespace, "tag-namespace", env.WithDefaultString("TAG_NAMESPACE", "oke-karpenter-ns"), "[REQUIRED] The tag namespace used to create and list instances")
	fs.StringVar(&o.OciAuthMethods, "oci-auth-methods", env.WithDefaultString("OCI_AUTH_METHODS", "OKE"), "[REQUIRED] the auth method to access oracle cloud resource, support OKE,API_KEY,SESSION,INSTANCE_PRINCIPAL")
	fs.StringVar(&o.PriceEndpoint, "price-endpoint", env.WithDefaultString("PRICE_ENDPOINT", "https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/"), "the endpoint which is used to pull price list from oci")
	fs.IntVar(&o.PriceSyncPeriod, "price-sync-period", env.WithDefaultInt("PRICE_SYNC_PERIOD", 12), "the hours which is used to sync price list for the next time")
	fs.BoolVar(&o.UseLocalPriceList, "use-local-price-list", env.WithDefaultBool("USE_LOCAL_PRICE_LIST", false), "if use-local-price-list is true, then it will use the embedded price list rather than to use the newest price list return from oci price api")
}

func (o *Options) Parse(fs *coreoptions.FlagSet, args ...string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return fmt.Errorf("parsing flags, %w", err)
	}
	if err := o.Validate(); err != nil {
		return fmt.Errorf("validating options, %w", err)
	}
	return nil
}

func (o *Options) ToContext(ctx context.Context) context.Context {
	return ToContext(ctx, o)
}

func ToContext(ctx context.Context, opts *Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, opts)
}

func FromContext(ctx context.Context) *Options {
	retval := ctx.Value(optionsKey{})
	if retval == nil {
		return nil
	}
	return retval.(*Options)
}
