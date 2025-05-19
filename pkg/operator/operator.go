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

package operator

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	operator_alt "github.com/zoom/karpenter-oci/pkg/alt/karpenter-core/pkg/operator"
	ocicache "github.com/zoom/karpenter-oci/pkg/cache"
	"github.com/zoom/karpenter-oci/pkg/operator/oci/config"
	metadata "github.com/zoom/karpenter-oci/pkg/operator/oci/instance"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/imagefamily"
	"github.com/zoom/karpenter-oci/pkg/providers/instance"
	"github.com/zoom/karpenter-oci/pkg/providers/instancetype"
	"github.com/zoom/karpenter-oci/pkg/providers/launchtemplate"
	"github.com/zoom/karpenter-oci/pkg/providers/pricing"
	"github.com/zoom/karpenter-oci/pkg/providers/securitygroup"
	"github.com/zoom/karpenter-oci/pkg/providers/subnet"
	"github.com/zoom/karpenter-oci/pkg/utils"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"knative.dev/pkg/ptr"
	"os"
	"os/user"
)

const (
	authByApiKey            = "API_KEY"
	authByOke               = "OKE"
	authBySession           = "SESSION"
	authByInstancePrincipal = "INSTANCE_PRINCIPAL"
	configFilePath          = "/etc/oci/config.yaml"
)

//func init() {
//	lo.Must0(apis.AddToScheme(scheme.Scheme))
//}

type Operator struct {
	*operator_alt.Operator

	ImageProvider         *imagefamily.Provider
	InstanceTypesProvider *instancetype.Provider
	InstanceProvider      *instance.Provider
	SubnetProvider        *subnet.Provider
	SecurityGroupProvider *securitygroup.Provider
}

func NewOperator(ctx context.Context, operator *operator_alt.Operator) (context.Context, *Operator) {
	var configProvider common.ConfigurationProvider
	authMethod := options.FromContext(ctx).OciAuthMethods
	switch authMethod {
	case authByApiKey:
		configProvider = lo.Must(NewOCIProvisioner())
	case authBySession:
		user := lo.Must(user.Current())
		configProvider = common.CustomProfileSessionTokenConfigProvider(fmt.Sprintf("%s/.oci/config", user.HomeDir), "SESSION")
	case authByInstancePrincipal:
		configProvider = lo.Must(auth.InstancePrincipalConfigurationProvider())
	case authByOke:
		configProvider = lo.Must(auth.OkeWorkloadIdentityConfigurationProvider())
	default:
		configProvider = lo.Must(auth.OkeWorkloadIdentityConfigurationProvider())
	}

	// inject available domain
	option := options.FromContext(ctx)
	client := lo.Must(identity.NewIdentityClientWithConfigurationProvider(configProvider))
	req := identity.ListAvailabilityDomainsRequest{CompartmentId: common.String(option.CompartmentId)}
	rep := lo.Must(client.ListAvailabilityDomains(ctx, req))
	option.AvailableDomains = lo.FlatMap(rep.Items, func(item identity.AvailabilityDomain, index int) []string {
		return []string{utils.ToString(item.Name)}
	})
	options.ToContext(ctx, option)

	// price list syncer
	priceSyncer := pricing.NewPriceListSyncer(option.PriceEndpoint, option.PriceSyncPeriod, option.UseLocalPriceList)
	if !option.UseLocalPriceList {
		lo.Must0(priceSyncer.Start(), "failed to sync price list")
	}

	region := lo.Must(configProvider.Region())
	cmpClient := lo.Must(core.NewComputeClientWithConfigurationProvider(configProvider))
	netClient := lo.Must(core.NewVirtualNetworkClientWithConfigurationProvider(configProvider))
	subnetProvider := subnet.NewProvider(netClient, cache.New(ocicache.DefaultTTL, ocicache.DefaultCleanupInterval))
	sgProvider := securitygroup.NewProvider(netClient, cache.New(ocicache.DefaultTTL, ocicache.DefaultCleanupInterval))
	imageProvider := imagefamily.NewProvider(cmpClient, cache.New(ocicache.DefaultTTL, ocicache.DefaultCleanupInterval))
	imageResolver := imagefamily.NewResolver(imageProvider)
	launchProvider := launchtemplate.NewDefaultProvider(imageResolver, lo.Must(GetCABundle(ctx, operator.GetConfig())), options.FromContext(ctx).ClusterEndpoint, options.FromContext(ctx).BootStrapToken)
	unavailableOfferCache := ocicache.NewUnavailableOfferings()
	instanceProvider := instance.NewProvider(cmpClient, subnetProvider, sgProvider, launchProvider, unavailableOfferCache)
	instancetypeProvider := instancetype.NewProvider(region, cmpClient, cache.New(ocicache.InstanceTypesAndZonesTTL, ocicache.DefaultCleanupInterval), unavailableOfferCache, priceSyncer)
	return ctx, &Operator{
		Operator:              operator,
		ImageProvider:         imageProvider,
		InstanceTypesProvider: instancetypeProvider,
		InstanceProvider:      instanceProvider,
		SubnetProvider:        subnetProvider,
		SecurityGroupProvider: sgProvider,
	}
}

// NewOCIProvisioner creates a new OCI provisioner.
func NewOCIProvisioner() (common.ConfigurationProvider, error) {
	configPath, ok := os.LookupEnv("CONFIG_YAML_FILENAME")
	if !ok {
		configPath = configFilePath
	}

	cfg, err := config.FromFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load configuration file at path %s", configPath)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, errors.Wrapf(err, "invalid configuration")
	}

	metadata, mdErr := metadata.New().Get()
	if mdErr != nil {
		zap.S().With(zap.Error(mdErr)).Warn("unable to retrieve instance metadata.")
	}

	if cfg.CompartmentID == "" {
		if metadata == nil {
			return nil, errors.Wrap(mdErr, "unable to get compartment OCID")
		}

		zap.S().With("compartmentID", metadata.CompartmentID).Infof("'CompartmentID' not given. Using compartment OCID from instance metadata.")
		cfg.CompartmentID = metadata.CompartmentID
	}

	cp, err := config.NewConfigurationProvider(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create volume provisioner client.")
	}
	return cp, nil
}

func GetCABundle(ctx context.Context, restConfig *rest.Config) (*string, error) {
	// Discover CA Bundle from the REST client. We could alternatively
	// have used the simpler client-go InClusterConfig() method.
	// However, that only works when Karpenter is running as a Pod
	// within the same cluster it's managing.
	if caBundle := options.FromContext(ctx).ClusterCABundle; caBundle != "" {
		return lo.ToPtr(caBundle), nil
	}
	if restConfig == nil {
		return nil, nil
	}
	transportConfig, err := restConfig.TransportConfig()
	if err != nil {
		return nil, fmt.Errorf("discovering caBundle, loading transport config, %w", err)
	}
	_, err = transport.TLSConfigFor(transportConfig) // fills in CAData!
	if err != nil {
		return nil, fmt.Errorf("discovering caBundle, loading TLS config, %w", err)
	}
	return ptr.String(base64.StdEncoding.EncodeToString(transportConfig.TLS.CAData)), nil
}
