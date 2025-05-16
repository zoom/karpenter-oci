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

package utils

import (
	"encoding/json"
	"fmt"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"os"
	"regexp"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/apis/v1beta1"
	"strconv"
	"strings"
)

var qualifiedNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9\-_.]`)
var startQualifiedNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]`)
var endQualifiedNameRegexp = regexp.MustCompile(`[a-zA-Z0-9]$`)

// PrettySlice truncates a slice after a certain number of max items to ensure
// that the Slice isn't too long
func PrettySlice[T any](s []T, maxItems int) string {
	var sb strings.Builder
	for i, elem := range s {
		if i > maxItems-1 {
			fmt.Fprintf(&sb, " and %d other(s)", len(s)-i)
			break
		} else if i > 0 {
			fmt.Fprint(&sb, ", ")
		}
		fmt.Fprint(&sb, elem)
	}
	return sb.String()
}

func SanitizeLabelValue(value string) string {
	// strict length
	if len(value) > 63 {
		value = value[:63]
	}

	// replace "_"
	value = qualifiedNameRegexp.ReplaceAllString(value, "_")

	// empty
	if value == "" {
		return value
	}

	// start with alphanumeric
	if !startQualifiedNameRegexp.MatchString(value) {
		value = "a_" + value
	}

	// end with alphanumeric
	if !endQualifiedNameRegexp.MatchString(value) {
		value = value + "_z"
	}

	return value
}

func FilterMap[K comparable, V any](m map[K]V, f func(K, V) bool) map[K]V {
	ret := map[K]V{}
	for k, v := range m {
		if f(k, v) {
			ret[k] = v
		}
	}
	return ret
}

// GetKubletConfigurationWithNodePool use the most recent version of the kubelet configuration.
// The priority of fields is listed below:
// 1.) v1 NodePool kubelet annotation (Showing a user configured using v1beta1 NodePool at some point)
// 2.) v1 EC2NodeClass will be used (showing a user configured using v1 EC2NodeClass)
func GetKubletConfigurationWithNodePool(nodePool *v1.NodePool, nodeClass *v1alpha1.OciNodeClass) (*v1alpha1.KubeletConfiguration, error) {
	if nodePool != nil {
		if annotation, ok := nodePool.Annotations[v1.KubeletCompatibilityAnnotationKey]; ok {
			return parseKubeletConfiguration(annotation)
		}
	}
	// DeepCopy the nodeClass.Spec.Kubelet if it exists, so we don't have the chance to mutate it indirectly
	if nodeClass.Spec.Kubelet != nil {
		return nodeClass.Spec.Kubelet.DeepCopy(), nil
	}
	return nil, nil
}

func GetKubeletConfigurationWithNodeClaim(nodeClaim *v1.NodeClaim, nodeClass *v1alpha1.OciNodeClass) (*v1alpha1.KubeletConfiguration, error) {
	if annotation, ok := nodeClaim.Annotations[v1.KubeletCompatibilityAnnotationKey]; ok {
		return parseKubeletConfiguration(annotation)
	}
	// DeepCopy the nodeClass.Spec.Kubelet if it exists, so we don't have the chance to mutate it indirectly
	if nodeClass.Spec.Kubelet != nil {
		return nodeClass.Spec.Kubelet.DeepCopy(), nil
	}
	return nil, nil
}

func parseKubeletConfiguration(annotation string) (*v1alpha1.KubeletConfiguration, error) {
	kubelet := &v1beta1.KubeletConfiguration{}
	err := json.Unmarshal([]byte(annotation), kubelet)
	if err != nil {
		return nil, fmt.Errorf("parsing kubelet config from %s annotation, %w", v1.KubeletCompatibilityAnnotationKey, err)
	}
	return &v1alpha1.KubeletConfiguration{
		ClusterDNS:                  kubelet.ClusterDNS,
		MaxPods:                     kubelet.MaxPods,
		PodsPerCore:                 kubelet.PodsPerCore,
		SystemReserved:              kubelet.SystemReserved,
		KubeReserved:                kubelet.KubeReserved,
		EvictionSoft:                kubelet.EvictionSoft,
		EvictionHard:                kubelet.EvictionHard,
		EvictionSoftGracePeriod:     kubelet.EvictionSoftGracePeriod,
		EvictionMaxPodGracePeriod:   kubelet.EvictionMaxPodGracePeriod,
		ImageGCHighThresholdPercent: kubelet.ImageGCHighThresholdPercent,
		ImageGCLowThresholdPercent:  kubelet.ImageGCLowThresholdPercent,
		CPUCFSQuota:                 kubelet.CPUCFSQuota,
	}, nil
}

// WithDefaultFloat64 returns the float64 value of the supplied environment variable or, if not present,
// the supplied default value. If the float64 conversion fails, returns the default
func WithDefaultFloat64(key string, def float64) float64 {
	val, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return def
	}
	return f
}

func GetHashKubeletWithNodeClaim(nodeClaim *v1.NodeClaim, nodeClass *v1alpha1.OciNodeClass) (string, error) {
	kubelet, err := GetKubeletConfigurationWithNodeClaim(nodeClaim, nodeClass)
	if err != nil {
		return "", err
	}
	return fmt.Sprint(lo.Must(hashstructure.Hash(kubelet, hashstructure.FormatV2, &hashstructure.HashOptions{
		SlicesAsSets:    true,
		IgnoreZeroValue: true,
		ZeroNil:         true,
	}))), nil
}

func GetHashKubeletWithNodePool(nodePool *v1.NodePool, nodeClass *v1alpha1.OciNodeClass) (string, error) {
	kubelet, err := GetKubletConfigurationWithNodePool(nodePool, nodeClass)
	if err != nil {
		return "", err
	}
	return fmt.Sprint(lo.Must(hashstructure.Hash(kubelet, hashstructure.FormatV2, &hashstructure.HashOptions{
		SlicesAsSets:    true,
		IgnoreZeroValue: true,
		ZeroNil:         true,
	}))), nil
}
