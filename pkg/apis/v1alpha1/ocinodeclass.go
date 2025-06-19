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

package v1alpha1

import (
	"fmt"
	"github.com/awslabs/operatorpkg/status"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeSubnetsReady        = "SubnetsReady"
	ConditionTypeSecurityGroupsReady = "SecurityGroupsReady"
	ConditionTypeImageReady          = "ImageReady"
)

type OciNodeClassSpec struct {
	VcnId                 string                      `json:"vcnId"`
	ImageSelector         []ImageSelectorTerm         `json:"imageSelector"`
	SubnetSelector        []SubnetSelectorTerm        `json:"subnetSelector"`
	SecurityGroupSelector []SecurityGroupSelectorTerm `json:"securityGroupSelector,omitempty"`
	UserData              *string                     `json:"userData,omitempty"`
	PreInstallScript      *string                     `json:"preInstallScript,omitempty"`
	MetaData              map[string]string           `json:"metaData,omitempty"`
	ImageFamily           string                      `json:"imageFamily"`
	Tags                  map[string]string           `json:"tags,omitempty"`
	// Kubelet defines args to be used when configuring kubelet on provisioned nodes.
	// They are a subset of the upstream types, recognizing not all options may be supported.
	// Wherever possible, the types and names should reflect the upstream kubelet types.
	// +kubebuilder:validation:XValidation:message="imageGCHighThresholdPercent must be greater than imageGCLowThresholdPercent",rule="has(self.imageGCHighThresholdPercent) && has(self.imageGCLowThresholdPercent) ?  self.imageGCHighThresholdPercent > self.imageGCLowThresholdPercent  : true"
	// +kubebuilder:validation:XValidation:message="evictionSoft OwnerKey does not have a matching evictionSoftGracePeriod",rule="has(self.evictionSoft) ? self.evictionSoft.all(e, (e in self.evictionSoftGracePeriod)):true"
	// +kubebuilder:validation:XValidation:message="evictionSoftGracePeriod OwnerKey does not have a matching evictionSoft",rule="has(self.evictionSoftGracePeriod) ? self.evictionSoftGracePeriod.all(e, (e in self.evictionSoft)):true"
	// +optional
	Kubelet       *KubeletConfiguration `json:"kubelet,omitempty" hash:"ignore"`
	BootConfig    *BootConfig           `json:"bootConfig"`
	LaunchOptions *LaunchOptions        `json:"launchOptions,omitempty"`
	BlockDevices  []*VolumeAttributes   `json:"blockDevices,omitempty"`
	AgentList     []string              `json:"agentList,omitempty"`
}

// TODO add verification >50Gi <32TB
type VolumeAttributes struct {
	SizeInGBs int64 `json:"sizeInGBs"`
	VpusPerGB int64 `json:"vpusPerGB"`
}

type OciNodeClassStatus struct {
	// Subnets contains the current Subnet values that are available to the
	// cluster under the subnet selectors.
	// +optional
	Subnets []*Subnet `json:"subnets,omitempty"`
	// Images contains the current images detail that are available to the
	// cluster under the image spec.
	// +optional
	Images []*Image `json:"images,omitempty"`
	// SecurityGroups contains the current security detail that are available to the
	// cluster under the security group spec.
	// +optional
	SecurityGroups []*SecurityGroup `json:"securityGroups,omitempty"`
	// Conditions contains signals for health and readiness
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`
}

type Subnet struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Image struct {
	Id            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	CompartmentId string `json:"compartmentId,omitempty"`
}

type SecurityGroup struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type ImageSelectorTerm struct {
	Id            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	CompartmentId string `json:"compartmentId,omitempty"`
}

type SubnetSelectorTerm struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type SecurityGroupSelectorTerm struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type BootConfig struct {
	BootVolumeSizeInGBs int64 `json:"bootVolumeSizeInGBs"`
	BootVolumeVpusPerGB int64 `json:"bootVolumeVpusPerGB"`
}

type LaunchOptions struct {

	// Emulation type for the boot volume.
	// * `ISCSI` - ISCSI attached block storage device.
	// * `SCSI` - Emulated SCSI disk.
	// * `IDE` - Emulated IDE disk.
	// * `VFIO` - Direct attached Virtual Function storage. This is the default option for local data
	// volumes on platform images.
	// * `PARAVIRTUALIZED` - Paravirtualized disk. This is the default for boot volumes and remote block
	// storage volumes on platform images.
	BootVolumeType string `mandatory:"false" json:"bootVolumeType,omitempty"`

	// Firmware used to boot VM. Select the option that matches your operating system.
	// * `BIOS` - Boot VM using BIOS style firmware. This is compatible with both 32 bit and 64 bit operating
	// systems that boot using MBR style bootloaders.
	// * `UEFI_64` - Boot VM using UEFI style firmware compatible with 64 bit operating systems. This is the
	// default for platform images.
	Firmware string `mandatory:"false" json:"firmware,omitempty"`

	// Emulation type for the physical network interface card (NIC).
	// * `E1000` - Emulated Gigabit ethernet controller. Compatible with Linux e1000 network driver.
	// * `VFIO` - Direct attached Virtual Function network controller. This is the networking type
	// when you launch an instance using hardware-assisted (SR-IOV) networking.
	// * `PARAVIRTUALIZED` - VM instances launch with paravirtualized devices using VirtIO drivers.
	NetworkType string `mandatory:"false" json:"networkType,omitempty"`

	// Emulation type for volume.
	// * `ISCSI` - ISCSI attached block storage device.
	// * `SCSI` - Emulated SCSI disk.
	// * `IDE` - Emulated IDE disk.
	// * `VFIO` - Direct attached Virtual Function storage. This is the default option for local data
	// volumes on platform images.
	// * `PARAVIRTUALIZED` - Paravirtualized disk. This is the default for boot volumes and remote block
	// storage volumes on platform images.
	RemoteDataVolumeType string `mandatory:"false" json:"remoteDataVolumeType,omitempty"`

	// Whether to enable consistent volume naming feature. Defaults to false.
	IsConsistentVolumeNamingEnabled bool `mandatory:"false" json:"isConsistentVolumeNamingEnabled,omitempty"`
}

type KubeletConfiguration struct {
	// clusterDNS is a list of IP addresses for the cluster DNS server.
	// Note that not all providers may use all addresses.
	//+optional
	ClusterDNS []string `json:"clusterDNS,omitempty"`
	// MaxPods is an override for the maximum number of pods that can run on
	// a worker node instance.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MaxPods *int32 `json:"maxPods,omitempty"`
	// PodsPerCore is an override for the number of pods that can run on a worker node
	// instance based on the number of cpu cores. This value cannot exceed MaxPods, so, if
	// MaxPods is a lower value, that value will be used.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	PodsPerCore *int32 `json:"podsPerCore,omitempty"`
	// SystemReserved contains resources reserved for OS system daemons and kernel memory.
	// +kubebuilder:validation:XValidation:message="valid keys for systemReserved are ['cpu','memory','ephemeral-storage','pid']",rule="self.all(x, x=='cpu' || x=='memory' || x=='ephemeral-storage' || x=='pid')"
	// +kubebuilder:validation:XValidation:message="systemReserved value cannot be a negative resource quantity",rule="self.all(x, !self[x].startsWith('-'))"
	// +optional
	SystemReserved map[string]string `json:"systemReserved,omitempty"`
	// KubeReserved contains resources reserved for Kubernetes system components.
	// +kubebuilder:validation:XValidation:message="valid keys for kubeReserved are ['cpu','memory','ephemeral-storage','pid']",rule="self.all(x, x=='cpu' || x=='memory' || x=='ephemeral-storage' || x=='pid')"
	// +kubebuilder:validation:XValidation:message="kubeReserved value cannot be a negative resource quantity",rule="self.all(x, !self[x].startsWith('-'))"
	// +optional
	KubeReserved map[string]string `json:"kubeReserved,omitempty"`
	// EvictionHard is the map of signal names to quantities that define hard eviction thresholds
	// +kubebuilder:validation:XValidation:message="valid keys for evictionHard are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionHard map[string]string `json:"evictionHard,omitempty"`
	// EvictionSoft is the map of signal names to quantities that define soft eviction thresholds
	// +kubebuilder:validation:XValidation:message="valid keys for evictionSoft are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionSoft map[string]string `json:"evictionSoft,omitempty"`
	// EvictionSoftGracePeriod is the map of signal names to quantities that define grace periods for each eviction signal
	// +kubebuilder:validation:XValidation:message="valid keys for evictionSoftGracePeriod are ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available']",rule="self.all(x, x in ['memory.available','nodefs.available','nodefs.inodesFree','imagefs.available','imagefs.inodesFree','pid.available'])"
	// +optional
	EvictionSoftGracePeriod map[string]metav1.Duration `json:"evictionSoftGracePeriod,omitempty"`
	// EvictionMaxPodGracePeriod is the maximum allowed grace period (in seconds) to use when terminating pods in
	// response to soft eviction thresholds being met.
	// +optional
	EvictionMaxPodGracePeriod *int32 `json:"evictionMaxPodGracePeriod,omitempty"`
	// ImageGCHighThresholdPercent is the percent of disk usage after which image
	// garbage collection is always run. The percent is calculated by dividing this
	// field value by 100, so this field must be between 0 and 100, inclusive.
	// When specified, the value must be greater than ImageGCLowThresholdPercent.
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=100
	// +optional
	ImageGCHighThresholdPercent *int32 `json:"imageGCHighThresholdPercent,omitempty"`
	// ImageGCLowThresholdPercent is the percent of disk usage before which image
	// garbage collection is never run. Lowest disk usage to garbage collect to.
	// The percent is calculated by dividing this field value by 100,
	// so the field value must be between 0 and 100, inclusive.
	// When specified, the value must be less than imageGCHighThresholdPercent
	// +kubebuilder:validation:Minimum:=0
	// +kubebuilder:validation:Maximum:=100
	// +optional
	ImageGCLowThresholdPercent *int32 `json:"imageGCLowThresholdPercent,omitempty"`
	// CPUCFSQuota enables CPU CFS quota enforcement for containers that specify CPU limits.
	// +optional
	CPUCFSQuota *bool `json:"cpuCFSQuota,omitempty"`
}

// OciNodeClass is the Schema for the OciNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ocinodeclasses,scope=Cluster,categories=karpenter,shortName={ocinc,ocincs}
// +kubebuilder:subresource:status
type OciNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OciNodeClassSpec   `json:"spec,omitempty"`
	Status OciNodeClassStatus `json:"status,omitempty"`
}

func (in *OciNodeClass) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions(
		ConditionTypeImageReady,
		ConditionTypeSubnetsReady,
		ConditionTypeSecurityGroupsReady,
	).For(in)
}

func (in *OciNodeClass) GetConditions() []status.Condition {
	return in.Status.Conditions
}

func (in *OciNodeClass) SetConditions(conditions []status.Condition) {
	in.Status.Conditions = conditions
}

// OciNodeClassList contains a list of OciNodeClass
// +kubebuilder:object:root=true
type OciNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OciNodeClass `json:"items"`
}

// We need to bump the OciNodeClassHashVersion when we make an update to the OciNodeClass CRD under these conditions:
// 1. A field changes its default value for an existing field that is already hashed
// 2. A field is added to the hash calculation with an already-set value
// 3. A field is removed from the hash calculations
const OciNodeClassHashVersion = "v1"

func (in *OciNodeClass) Hash() string {
	return fmt.Sprint(lo.Must(hashstructure.Hash(in.Spec, hashstructure.FormatV2, &hashstructure.HashOptions{
		SlicesAsSets:    true,
		IgnoreZeroValue: true,
		ZeroNil:         true,
	})))
}
