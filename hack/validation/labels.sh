#!/bin/bash

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

set -euo pipefail
# Labels Validation

# # Adding validation for nodepool

rule=$'self.all(x, x in
    [
        "karpenter.k8s.oracle/ocinodeclass",
        "karpenter.k8s.oracle/instance-shape-name",
        "karpenter.k8s.oracle/instance-cpu",
        "karpenter.k8s.oracle/instance-memory",
        "karpenter.k8s.oracle/instance-gpu",
        "karpenter.k8s.oracle/instance-network-bandwidth",
        "karpenter.k8s.oracle/instance-max-vnics",
        "karpenter.k8s.oracle/is-flexible"
    ]
    || !x.find("^([^/]+)").endsWith("karpenter.k8s.oracle")
)
'
# above regex: everything before the first '/' (any characters except '/' at the beginning of the string)

rule=${rule//\"/\\\"}            # escape double quotes
rule=${rule//$'\n'/}             # remove newlines
rule=$(echo "$rule" | tr -s ' ') # remove extra spaces

printf -v expr '.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.template.properties.metadata.properties.labels.x-kubernetes-validations +=
    [{"message": "label domain \\"karpenter.k8s.oracle\\" is restricted", "rule": "%s"}]' "$rule"
yq eval "${expr}" -i pkg/apis/crds/karpenter.sh_nodepools.yaml

printf -v expr '.spec.versions[1].schema.openAPIV3Schema.properties.spec.properties.template.properties.metadata.properties.labels.x-kubernetes-validations +=
    [{"message": "label domain \\"karpenter.k8s.oracle\\" is restricted", "rule": "%s"}]' "$rule"
yq eval "${expr}" -i pkg/apis/crds/karpenter.sh_nodepools.yaml