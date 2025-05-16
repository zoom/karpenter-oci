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

package test

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/imdario/mergo"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/karpenter/pkg/test"
)

func OciNodeClass(overrides ...v1alpha1.OciNodeClass) *v1alpha1.OciNodeClass {
	options := v1alpha1.OciNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: v1alpha1.OciNodeClassSpec{
			Image: &v1alpha1.Image{
				Name:          "ubuntu",
				CompartmentId: "ocid1.compartment.oc1..aaaaaaaa",
			},
			VcnId:              "vcn_1",
			SecurityGroupNames: []string{"securityGroup-test1", "securityGroup-test2"},
			SubnetName:         "private-1",
			UserData:           common.String("#!/bin/bash"),
			ImageFamily:        v1alpha1.Ubuntu2204ImageFamily,
			Tags:               map[string]string{"test_key": "test_val"},
			BootConfig: &v1alpha1.BootConfig{
				BootVolumeSizeInGBs: 100,
				BootVolumeVpusPerGB: 10,
			},
			BlockDevices: []*v1alpha1.VolumeAttributes{{SizeInGBs: 100, VpusPerGB: 20}},
		},
	}
	for _, override := range overrides {
		if err := mergo.Merge(&options, override, mergo.WithOverride); err != nil {
			panic(fmt.Sprintf("Failed to merge settings: %s", err))
		}
	}

	return &v1alpha1.OciNodeClass{
		ObjectMeta: test.ObjectMeta(options.ObjectMeta),
		Spec:       options.Spec,
		Status:     options.Status,
	}
}

func OciNodeClassFieldIndexer(ctx context.Context) func(cache.Cache) error {
	return func(c cache.Cache) error {
		return c.IndexField(ctx, &v1.NodeClaim{}, "spec.nodeClassRef.name", func(obj client.Object) []string {
			nc := obj.(*v1.NodeClaim)
			if nc.Spec.NodeClassRef == nil {
				return []string{""}
			}
			return []string{nc.Spec.NodeClassRef.Name}
		})
	}
}
