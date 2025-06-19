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

package oci

import (
	"fmt"
	oci_common "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/pkg/test"
	"github.com/zoom/karpenter-oci/pkg/utils"
	"github.com/zoom/karpenter-oci/test/pkg/environment/common"
	"os"
	"os/user"
	"testing"

	"github.com/samber/lo"
)

type Environment struct {
	*common.Environment
	Region        string
	CompartmentId string

	VCNAPI      core.VirtualNetworkClient
	CMPAPI      core.ComputeClient
	IDENTITYAPI identity.IdentityClient
	STORAGEAPI  core.BlockstorageClient

	ClusterName         string
	ClusterEndpoint     string
	ClusterDns          string
	TagNamespace        string
	PrivateCluster      bool
	AvailableDomainInfo []string
}

var EphemeralInitContainerImage = "alpine"

// replace the below var with yours
// env
var clusterName = "karpenter-test-132"
var clusterEndPoint = "https://10.0.0.13:6443"
var compId = "ocid1.compartment.oc1."
var clusterDns = "10.96.5.5"

// nodeclass
var vcnId = "ocid1.vcn.oc1.iad.amaaaaaa"
var subnetName = "oke-nodesubnet-quick"
var sgName = "test-sg1"

func NewEnvironment(t *testing.T) *Environment {
	env := common.NewEnvironment(t)
	//cfg := lo.Must(operator.NewOCIProvisioner())
	user := lo.Must(user.Current())
	cfg := oci_common.CustomProfileSessionTokenConfigProvider(fmt.Sprintf("%s/.oci/config", user.HomeDir), "SESSION")
	_ = os.Setenv("CLUSTER_NAME", clusterName)
	_ = os.Setenv("CLUSTER_ENDPOINT", clusterEndPoint)
	_ = os.Setenv("COMPARTMENT_ID", compId)
	_ = os.Setenv("CLUSTER_DNS", clusterDns)
	_ = os.Setenv("TAG_NAMESPACE", "oke-karpenter-ns")
	ociEnv := &Environment{
		Region:          lo.Must(cfg.Region()),
		Environment:     env,
		CMPAPI:          lo.Must(core.NewComputeClientWithConfigurationProvider(cfg)),
		VCNAPI:          lo.Must(core.NewVirtualNetworkClientWithConfigurationProvider(cfg)),
		IDENTITYAPI:     lo.Must(identity.NewIdentityClientWithConfigurationProvider(cfg)),
		STORAGEAPI:      lo.Must(core.NewBlockstorageClientWithConfigurationProvider(cfg)),
		ClusterName:     lo.Must(os.LookupEnv("CLUSTER_NAME")),
		ClusterEndpoint: lo.Must(os.LookupEnv("CLUSTER_ENDPOINT")),
		CompartmentId:   lo.Must(os.LookupEnv("COMPARTMENT_ID")),
		ClusterDns:      lo.Must(os.LookupEnv("CLUSTER_DNS")),
		TagNamespace:    lo.Must(os.LookupEnv("TAG_NAMESPACE")),
	}

	ociEnv.AvailableDomainInfo = lo.Map(lo.Must(ociEnv.IDENTITYAPI.ListAvailabilityDomains(env.Context,
		identity.ListAvailabilityDomainsRequest{CompartmentId: oci_common.String(ociEnv.CompartmentId)})).Items,
		func(item identity.AvailabilityDomain, index int) string {
			return utils.ToString(item.Name)
		})
	return ociEnv
}

func (env *Environment) DefaultOciNodeClass() *v1alpha1.OciNodeClass {
	nodeClass := test.OciNodeClass()
	nodeClass.Spec.ImageFamily = v1alpha1.OracleOKELinuxImageFamily
	nodeClass.Spec.ImageSelector = []v1alpha1.ImageSelectorTerm{{Name: "Oracle-Linux-8.10-2025.02.28-0-OKE-1.30.1-760",
		CompartmentId: "ocid1.compartment.oc1..aaaaaaaab4u67dhgtj5gpdpp3z42xqqsdnufxkatoild46u3hb67vzojfmzq"}}
	nodeClass.Spec.SecurityGroupSelector = []v1alpha1.SecurityGroupSelectorTerm{
		{
			Name: sgName,
		},
	}
	nodeClass.Spec.SubnetSelector = []v1alpha1.SubnetSelectorTerm{
		{
			Name: subnetName,
		},
	}
	nodeClass.Spec.VcnId = vcnId
	nodeClass.Spec.Tags = map[string]string{}
	return nodeClass
}
