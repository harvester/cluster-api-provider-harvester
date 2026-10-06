/*
Copyright 2025 SUSE.

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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// MachineFinalizer allows ReconcileHarvesterMachine to clean up resources associated with HarvesterMachine before
	// removing it from the apiserver.
	MachineFinalizer = "harvestermachine.infrastructure.cluster.x-k8s.io/finalizer"

	// MachineFinalizerLegacy is the old finalizer name without path segment, kept for migration.
	MachineFinalizerLegacy = "harvestermachine.infrastructure.cluster.x-k8s.io"
)

const (
	// MachineCreatedCondition documents that the machine has been created.
	MachineCreatedCondition string = "MachineCreated"

	// MachineNotFoundReason documents that the machine was not found.
	MachineNotFoundReason = "MachineNotFound"

	// VMProvisioningReadyCondition documents VM creation and provisioning status.
	VMProvisioningReadyCondition string = "VMProvisioningReady"
	// VMProvisioningInProgressReason documents that VM provisioning is in progress.
	VMProvisioningInProgressReason = "VMProvisioningInProgress"
	// VMProvisioningFailedReason documents that VM provisioning has failed.
	VMProvisioningFailedReason = "VMProvisioningFailed"
	// VMProvisioningReadyReason documents that VM provisioning is complete.
	VMProvisioningReadyReason = "VMProvisioningReady"

	// NodeProviderIDMatchesCondition reports whether the workload cluster Node of the
	// machine carries the machine provider ID. Cluster API matches the Machine to its
	// Node by provider ID, and a Node provider ID cannot be changed once set.
	NodeProviderIDMatchesCondition string = "NodeProviderIDMatches"
	// NodeProviderIDMatchesReason is used when the Node carries the machine provider ID.
	NodeProviderIDMatchesReason = "NodeProviderIDMatches"
	// NodeProviderIDMismatchReason is used when another component set a different
	// provider ID on the Node first.
	NodeProviderIDMismatchReason = "NodeProviderIDMismatch"

	// VMRunningCondition documents whether the VM is running.
	VMRunningCondition string = "VMRunning"
	// VMRunningReason documents that the VM is running.
	VMRunningReason = "VMRunning"
	// VMNotRunningReason documents that the VM is not yet running.
	VMNotRunningReason = "VMNotRunning"

	// VMIPAllocatedCondition documents that a static IP has been allocated for the VM.
	VMIPAllocatedCondition string = "VMIPAllocated"
	// VMIPAllocationFailedReason documents that IP allocation failed.
	VMIPAllocationFailedReason = "VMIPAllocationFailed"
	// VMIPPoolExhaustedReason documents that the IP pool has no available addresses.
	VMIPPoolExhaustedReason = "VMIPPoolExhausted"
	// VMIPAllocatedReason documents that an IP was successfully allocated.
	VMIPAllocatedReason = "VMIPAllocated"
)

// HarvesterMachineSpec defines the desired state of HarvesterMachine.
type HarvesterMachineSpec struct {
	// ProviderID will be the ID of the VM in the provider (Harvester).
	// This is set by the Cloud provider on the Workload cluster node and replicated by CAPI.
	// +optional
	ProviderID string `json:"providerID,omitempty"`

	// FailureDomain defines the zone or failure domain where this VM should be.
	// +optional
	FailureDomain string `json:"failureDomain,omitempty"`

	// CPU is the number of CPU to assign to the VM.
	CPU uint32 `json:"cpu"`

	// Memory is the memory size to assign to the VM (should be similar to pod.spec.containers.resources.limits).
	Memory string `json:"memory"`

	// SSHUser is the user that should be used to connect to the VMs using SSH.
	SSHUser string `json:"sshUser"`

	// SSHKeyPair is the name of the SSH key pair to use for SSH access to the VM (this keyPair should be created in Harvester).
	// The reference can be in the format "namespace/name" or just "name" if the object is in the same namespace as the HarvesterMachine.
	SSHKeyPair string `json:"sshKeyPair"`

	// Volumes is a list of Volumes to attach to the VM
	Volumes []Volume `json:"volumes"`

	// Networks is a list of Networks to attach to the VM.
	// Each item in the list can have the format "namespace/name" or just "name" if the object is in the same namespace as the HarvesterMachine.
	Networks []string `json:"networks"`

	// NodeAffinity gives the possibility to select preferred nodes for VM scheduling on Harvester. This works exactly like Pods.
	// +optional
	NodeAffinity *corev1.NodeAffinity `json:"nodeAffinity,omitempty"`

	// WorkloadAffinity gives the possibility to define affinity rules with other workloads running on Harvester.
	// +optional
	WorkloadAffinity *corev1.PodAffinity `json:"workloadAffinity,omitempty"`

	// NetworkConfig is the static network configuration for this specific machine.
	// If set, this takes precedence over the cluster-level VMNetworkConfig IP pool allocation.
	// +optional
	NetworkConfig *NetworkConfig `json:"networkConfig,omitempty"`

	// VMNetworkConfig is the pool-based network configuration for this machine.
	// When set, it fully replaces the cluster-level spec.vmNetworkConfig of the
	// HarvesterCluster for this machine: the IP is allocated from the pools
	// referenced here, and the gateway, subnet mask and DNS settings defined here
	// are written to the machine cloud-init. Set it in a HarvesterMachineTemplate
	// to give one machine type (for example the workers) a network configuration
	// different from the rest of the cluster, such as a dedicated network with its
	// own gateway. When unset, the cluster-level configuration applies. The inline
	// ipPool variant is not supported at the machine level; use ipPoolRef or
	// ipPoolRefs. Mutually exclusive with networkConfig.
	// +optional
	VMNetworkConfig *VMNetworkConfig `json:"vmNetworkConfig,omitempty"`

	// Firmware selects the firmware used to boot the VM. When unset, the VM
	// keeps booting with the current default (BIOS).
	// +optional
	Firmware *Firmware `json:"firmware,omitempty"`

	// TPM attaches an emulated Trusted Platform Module device to the VM.
	// Together with UEFI Secure Boot, this is the building block for measured
	// or attested boot setups.
	// +optional
	TPM *TPM `json:"tpm,omitempty"`
}

// Firmware describes the firmware configuration of a VM.
type Firmware struct {
	// EFI boots the VM with UEFI firmware instead of the default BIOS.
	EFI bool `json:"efi,omitempty"`

	// SecureBoot enables UEFI Secure Boot: the VM boots with
	// SecureBoot-enabled OVMF ROMs and the SMM CPU feature is turned on.
	// Requires efi to be true. The guest image must carry a signed bootloader.
	// +optional
	SecureBoot bool `json:"secureBoot,omitempty"`
}

// TPM describes the emulated TPM device of a VM.
type TPM struct {
	// Enabled attaches an emulated TPM device to the VM.
	Enabled bool `json:"enabled,omitempty"`

	// Persistent keeps the TPM state across reboots.
	// +optional
	Persistent bool `json:"persistent,omitempty"`
}

// NetworkConfig defines static network configuration for a VM.
type NetworkConfig struct {
	// Address is the static IP address for the VM (e.g. "172.16.3.40").
	Address string `json:"address"`

	// Gateway is the gateway IP address.
	Gateway string `json:"gateway"`

	// DNSServers is a list of DNS server IP addresses.
	// +optional
	DNSServers []string `json:"dnsServers,omitempty"`

	// DNSSearch is a list of DNS search domains.
	// +optional
	DNSSearch []string `json:"dnsSearch,omitempty"`
}

// Volume defines a volume that should be attached to the VM.
type Volume struct {
	// VolumeType is the type of volume to attach.
	// Choose between: "storageClass" or "image"
	VolumeType VolumeType `json:"volumeType"`

	// ImageName is the name of the image to use if the volumeType is "image"
	// ImageName can be in the format "namespace/name" or just "name" if the object is in the same namespace as the HarvesterMachine.
	// +optional
	ImageName string `json:"imageName,omitempty"`

	// StorageClass is the name of the storage class to be used if the volumeType is "storageClass"
	StorageClass string `json:"storageClass,omitempty"`

	// VolumeSize is the desired size of the volume. This satisfies to standard Kubernetes *resource.Quantity syntax.
	// Examples: 40.5Gi, 30M, etc. are valid
	// +optional
	VolumeSize *resource.Quantity `json:"volumeSize,omitempty"`

	// BootOrder is an integer that determines the order of priority of volumes for booting the VM.
	// If absent, the sequence with which volumes appear in the manifest will be used.
	// +optional
	BootOrder int `json:"bootOrder,omitempty"`
}

// VolumeType is an enum string. It can only take the values: "storageClass" or "image".
// +kubebuilder:validation:Enum=storageClass;image
type VolumeType string

// Initialization tracks provisioning state for the CAPI v1beta2 contract
// (status.initialization.provisioned). It is published alongside status.ready (the v1beta1
// contract field) and kept in sync, so a single v1alpha1 object satisfies whichever contract
// CAPI selects from the CRD's cluster.x-k8s.io/<version> label.
type Initialization struct {
	// Provisioned shows if the resource has been provisioned.
	Provisioned bool `json:"provisioned,omitempty"`
}

// HarvesterMachineStatus defines the observed state of HarvesterMachine.
type HarvesterMachineStatus struct {
	// Ready is true when the provider resource is ready.
	Ready bool `json:"ready,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`
	// Initialization provides the CAPI v1beta2 contract readiness field
	// (status.initialization.provisioned), kept in sync with status.ready (v1beta1 contract).
	Initialization Initialization `json:"initialization,omitempty"`

	// AllocatedIPAddress is the IP address allocated from the VM IP pool for this machine.
	// +optional
	AllocatedIPAddress string `json:"allocatedIPAddress,omitempty"`

	// AllocatedPoolRef is the name of the IPPool from which AllocatedIPAddress was allocated.
	// Used for accurate release when multiple pools are configured.
	// +optional
	AllocatedPoolRef string `json:"allocatedPoolRef,omitempty"`

	// FailureDomain reports the failure domain the machine was placed in
	// (CAPI contract field, mirrored to Machine.status.failureDomain).
	// +optional
	FailureDomain string `json:"failureDomain,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// HarvesterMachine is the Schema for the harvestermachines API.
type HarvesterMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HarvesterMachineSpec   `json:"spec,omitempty"`
	Status HarvesterMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HarvesterMachineList contains a list of HarvesterMachine.
type HarvesterMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []HarvesterMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HarvesterMachine{}, &HarvesterMachineList{})
}

// GetConditions returns the set of conditions for this object.
// CAPI v1.12+ standard signature returning k8s-aligned []metav1.Condition.
func (m *HarvesterMachine) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *HarvesterMachine) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}
