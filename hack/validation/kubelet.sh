# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubelet Validation

# The regular expression adds validation for kubelet.kubeReserved and kubelet.systemReserved values of the map are resource.Quantity
# Quantity: https://github.com/kubernetes/apimachinery/blob/d82afe1e363acae0e8c0953b1bc230d65fdb50e2/pkg/api/resource/quantity.go#L100
# OciNodeClass Validation:
yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.kubelet.properties.kubeReserved.additionalProperties.pattern = "^(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?$"' -i pkg/apis/crds/karpenter.k8s.oracle_ocinodeclasses.yaml
yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.kubelet.properties.systemReserved.additionalProperties.pattern = "^(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?$"' -i pkg/apis/crds/karpenter.k8s.oracle_ocinodeclasses.yaml

# The regular expression is a validation for kubelet.evictionHard and kubelet.evictionSoft are percentage or a resource.Quantity
# Quantity: https://github.com/kubernetes/apimachinery/blob/d82afe1e363acae0e8c0953b1bc230d65fdb50e2/pkg/api/resource/quantity.go#L100
# OciNodeClass Validation:
yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.kubelet.properties.evictionHard.additionalProperties.pattern = "^((\d{1,2}(\.\d{1,2})?|100(\.0{1,2})?)%||(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?)$"' -i pkg/apis/crds/karpenter.k8s.oracle_ocinodeclasses.yaml
yq eval '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.kubelet.properties.evictionSoft.additionalProperties.pattern = "^((\d{1,2}(\.\d{1,2})?|100(\.0{1,2})?)%||(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?)$"' -i pkg/apis/crds/karpenter.k8s.oracle_ocinodeclasses.yaml
