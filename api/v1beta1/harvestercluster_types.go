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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// ClusterFinalizer allows ReconcileHarvesterCluster to clean up resources associated with HarvesterCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "harvester.infrastructure.cluster.x-k8s.io/finalizer"

	// ClusterFinalizerLegacy is the old finalizer name without path segment, kept for migration.
	ClusterFinalizerLegacy = "harvester.infrastructure.cluster.x-k8s.io"

	// IdentitySecretFinalizerPrefix prefixes the finalizer a HarvesterCluster sets on its
	// identity Secret, followed by the HarvesterCluster UID. The controllers need the
	// Secret to delete the cluster resources in Harvester, so it must not disappear before
	// the HarvesterCluster is deleted. One finalizer per HarvesterCluster lets several
	// clusters share a Secret and release it independently.
	IdentitySecretFinalizerPrefix = "harvester.infrastructure.cluster.x-k8s.io/identity-"
	// DHCP is one of the possible values for the IPAMType field in the LoadBalancerConfig.
	DHCP = "dhcp"
	// POOL is one of the possible values for the IPAMType field in the LoadBalancerConfig.
	POOL = "pool"
)

const (
	// LoadBalancerReadyCondition documents the status of the load balancer in Harvester.
	LoadBalancerReadyCondition string = "LoadBalancerReady"
	// LoadBalancerNotReadyReason documents the reason why the load balancer is not ready.
	// metav1.Condition.Reason must match ^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$ (PascalCase, no spaces).
	LoadBalancerNotReadyReason = "LoadBalancerNotReady"
	// LoadBalancerNoBackendMachineReason documents that there are no machines matching the load balancer configuration.
	LoadBalancerNoBackendMachineReason = "LoadBalancerNoBackendMachine"
	// LoadBalancerHealthcheckFailedReason documents the reason why the load balancer is not ready.
	LoadBalancerHealthcheckFailedReason = "LoadBalancerHealthcheckFailed"
	// CustomIPPoolCreatedCondition documents if a custom IP Pool was created in Harvester.
	CustomIPPoolCreatedCondition string = "CustomIPPoolCreated"
	// CustomPoolCreationInHarvesterFailedReason documents the reason why a custom pool was unable to be created.
	CustomPoolCreationInHarvesterFailedReason = "CustomPoolCreationInHarvesterFailed"
	// CustomIPPoolCreatedSuccessfullyReason documents the reason why Custom IP Pool was created.
	CustomIPPoolCreatedSuccessfullyReason = "CustomIPPoolCreatedSuccessfully"

	// CloudProviderConfigReadyCondition documents the status of the cloud provider configuration in Harvester.
	CloudProviderConfigReadyCondition string = "CloudProviderConfigReady"
	// CloudProviderConfigNotReadyReason documents the reason why the cloud provider configuration is not ready.
	CloudProviderConfigNotReadyReason = "CloudProviderConfigNotReady"
	// CloudProviderConfigGenerationFailedReason documents the reason why the cloud provider configuration generation failed.
	CloudProviderConfigGenerationFailedReason = "CloudProviderConfigGenerationFailed"
	// CloudProviderConfigGeneratedSuccessfullyReason documents the reason why the cloud provider configuration was generated.
	CloudProviderConfigGeneratedSuccessfullyReason = "CloudProviderConfigGeneratedSuccessfully"

	// InfrastructureReadyCondition documents that all infrastructure components are provisioned.
	InfrastructureReadyCondition string = "InfrastructureReady"
	// InfrastructureProvisioningInProgressReason documents that infrastructure provisioning is in progress.
	InfrastructureProvisioningInProgressReason = "InfrastructureProvisioningInProgress"
	// InfrastructureProvisioningFailedReason documents that infrastructure provisioning has failed.
	InfrastructureProvisioningFailedReason = "InfrastructureProvisioningFailed"
	// InfrastructureReadyReason documents that all infrastructure components are ready.
	InfrastructureReadyReason = "InfrastructureReady"

	// TargetNamespaceReadyCondition indicates the target namespace in Harvester exists and is accessible.
	TargetNamespaceReadyCondition string = "TargetNamespaceReady"
	// TargetNamespaceNotReadyReason documents that the target namespace does not exist or is not accessible.
	TargetNamespaceNotReadyReason = "TargetNamespaceNotReady"
	// TargetNamespaceReadyReason documents that the target namespace exists and is accessible.
	TargetNamespaceReadyReason = "TargetNamespaceReady"

	// HarvesterConnectionReadyCondition indicates successful connection/authentication to Harvester API.
	HarvesterConnectionReadyCondition string = "HarvesterConnectionReady"
	// HarvesterConnectionFailedReason documents that connection to Harvester API failed.
	HarvesterConnectionFailedReason = "HarvesterConnectionFailed"
	// HarvesterAuthenticationFailedReason documents that authentication to Harvester API failed.
	HarvesterAuthenticationFailedReason = "HarvesterAuthenticationFailed"
	// HarvesterConnectionReadyReason documents that connection and authentication to Harvester API is successful.
	HarvesterConnectionReadyReason = "HarvesterConnectionReady"

	// VMIPPoolReadyCondition documents the status of the VM IP pool for static IP allocation.
	VMIPPoolReadyCondition string = "VMIPPoolReady"
	// VMIPPoolCreationFailedReason documents that the VM IP pool creation failed.
	VMIPPoolCreationFailedReason = "VMIPPoolCreationFailed"
	// VMIPPoolReadyReason documents that the VM IP pool is ready.
	VMIPPoolReadyReason = "VMIPPoolReady"
	// VMIPPoolCreatedByControllerCondition documents that the VM IP pool was created by the controller
	// (not pre-existing). Only pools with this condition are deleted on cluster deletion.
	VMIPPoolCreatedByControllerCondition string = "VMIPPoolCreatedByController"
	// VMIPPoolCreatedByControllerReason documents that the VM IP pool was created by the controller.
	VMIPPoolCreatedByControllerReason = "VMIPPoolCreatedByController"

	// FleetIntegrationReadyCondition documents whether Fleet labels have been propagated after Turtles import.
	FleetIntegrationReadyCondition string = "FleetIntegrationReady"
	// FleetIntegrationNotApplicableReason documents that Fleet integration is not applicable (no auto-import label).
	FleetIntegrationNotApplicableReason = "FleetIntegrationNotApplicable"
	// FleetIntegrationInProgressReason documents that Fleet integration is in progress (waiting for Rancher resources).
	FleetIntegrationInProgressReason = "FleetIntegrationInProgress"
	// FleetIntegrationReadyReason documents that Fleet labels have been successfully propagated.
	FleetIntegrationReadyReason = "FleetIntegrationReady"
	// FleetIntegrationFailedReason documents that Fleet integration failed.
	FleetIntegrationFailedReason = "FleetIntegrationFailed"
)

const (
	// InitMachineCreatedCondition documents the status of the init machine in Harvester.
	InitMachineCreatedCondition string = "InitMachineCreated"
	// InitMachineNotYetCreatedReason documents the reason why the init machine is not ready.
	InitMachineNotYetCreatedReason = "InitMachineNotYetCreated"
)

// HarvesterClusterSpec defines the desired state of HarvesterCluster.
type HarvesterClusterSpec struct {
	// Server is the url to connect to Harvester.
	// +optional
	Server string `json:"server,omitempty"`

	// IdentitySecret is the name of the Secret containing HarvesterKubeConfig file.
	IdentitySecret SecretKey `json:"identitySecret"`

	// LoadBalancerConfig describes how the load balancer should be created in Harvester.
	LoadBalancerConfig LoadBalancerConfig `json:"loadBalancerConfig"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// TargetNamespace is the namespace on the Harvester cluster where VMs, Load Balancers, etc. should be created.
	TargetNamespace string `json:"targetNamespace"`

	// UpdateCloudProviderConfig if not empty, will trigger the generation of the cloud provider configuration.
	// It needs a reference to a ConfigMap containing the cloud provider deployment manifests, that are used by a ClusterResourceSet.
	// +optional
	UpdateCloudProviderConfig UpdateCloudProviderConfig `json:"updateCloudProviderConfig,omitempty"`

	// VMNetworkConfig is the network configuration for VMs that use static IPs from a pool.
	// +optional
	VMNetworkConfig *VMNetworkConfig `json:"vmNetworkConfig,omitempty"`
}

// VMNetworkConfig describes the network configuration for VM static IP allocation.
type VMNetworkConfig struct {
	// IPPoolRef is a reference to an existing IPPool in Harvester for VM IP allocation.
	// When multiple pools are needed, use IPPoolRefs instead.
	// Mutually exclusive with IPPool.
	// +optional
	IPPoolRef string `json:"ipPoolRef,omitempty"`

	// IPPoolRefs is a list of references to existing IPPools in Harvester.
	// Pools are tried in order: if a pool is exhausted, allocation falls back
	// to the next one. A pool whose selector designates a network (the IPPool
	// spec.selector.network field in Harvester) is only used for machines
	// attached to that network, so distinct pools can serve, for example, the
	// control-plane and worker networks of one cluster; a pool with no
	// selector network is used for any machine.
	// +optional
	IPPoolRefs []string `json:"ipPoolRefs,omitempty"`

	// IPPool defines a new IPPool to create in Harvester for VM IP allocation.
	// Mutually exclusive with IPPoolRef/IPPoolRefs.
	// +optional
	IPPool *IpPool `json:"ipPool,omitempty"`

	// Gateway is the gateway IP address for the VM network.
	Gateway string `json:"gateway"`

	// SubnetMask is the subnet mask for the VM network (e.g. "255.255.0.0").
	SubnetMask string `json:"subnetMask"`

	// DNSServers is a list of DNS server IP addresses.
	// +optional
	DNSServers []string `json:"dnsServers,omitempty"`

	// DNSSearch is a list of DNS search domains.
	// +optional
	DNSSearch []string `json:"dnsSearch,omitempty"`
}

// GetIPPoolRefs returns all configured IPPool references in priority order.
// It merges IPPoolRefs and IPPoolRef for backward compatibility.
func (v *VMNetworkConfig) GetIPPoolRefs() []string {
	if len(v.IPPoolRefs) > 0 {
		return v.IPPoolRefs
	}

	if v.IPPoolRef != "" {
		return []string{v.IPPoolRef}
	}

	return nil
}

// SecretKey is a reference to a Secret which stores Identity information for the Target Harvester Cluster.
type SecretKey struct {
	// Namespace is the namespace in which the required Identity Secret should be found.
	Namespace string `json:"namespace"`

	// Name is the name of the required Identity Secret.
	Name string `json:"name"`
}

// LoadBalancerConfig describes how the load balancer should be created in Harvester.
type LoadBalancerConfig struct {
	// IPAMType is the configuration of IP addressing for the control plane load balancer.
	// This can take two values, either "dhcp" or "ippool".
	IPAMType IPAMType `json:"ipamType"`

	// IpPoolRef is a reference to an existing IpPool object in Harvester's cluster.
	// This field is mutually exclusive with "ipPool".
	IpPoolRef string `json:"ipPoolRef,omitempty"`

	// IpPool defines a new IpPool that will be added to Harvester.
	// This field is mutually exclusive with "IpPoolRef".
	IpPool IpPool `json:"ipPool,omitempty"`

	// Listeners is a list of listeners that should be created on the load balancer.
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`

	// Description is a description of the load balancer that should be created.
	// +optional
	Description string `json:"description,omitempty"`
}

// IPAMType describes the way the LoadBalancer IP should be created, using DHCP or using an IPPool defined in Harvester.
// +kubebuilder:validation:Enum:=dhcp;pool
type IPAMType string

// IpPool is a description of a new IPPool to be created in Harvester.
type IpPool struct {
	// VMNetwork is the name of an existing VM Network in Harvester where the IPPool should exist.
	// The reference can have the format "namespace/name" or just "name" if the object is in the same namespace as the HarvesterCluster.
	VMNetwork string `json:"vmNetwork"`

	// Subnet is a string describing the subnet that should be used by the IP Pool, it should have the CIDR Format of an IPv4 Address.
	// e.g. 172.17.1.0/24.
	Subnet string `json:"subnet"`

	// Gateway is the IP Address that should be used by the Gateway on the Subnet. It should be a valid address inside the subnet.
	// e.g. 172.17.1.1.
	Gateway string `json:"gateway"`

	// RangeStart is the first IP Address that should be used by the IP Pool.
	// +optional
	RangeStart string `json:"rangeStart,omitempty"`

	// RangeEnd is the last IP Address that should be used by the IP Pool.
	// +optional
	RangeEnd string `json:"rangeEnd,omitempty"`
}

// Listener is a description of a new Listener to be created on the Load Balancer.
type Listener struct {
	// Name is the name of the listener.
	Name string `json:"name"`

	// Port is the port that the listener should listen on.
	Port int32 `json:"port"`

	// Protocol is the protocol that the listener should use, either TCP or UDP.
	// +kubebuilder:validation:Enum:=TCP;UDP
	Protocol corev1.Protocol `json:"protocol"`

	// TargetPort is the port that the listener should forward traffic to.
	BackendPort int32 `json:"backendPort"`
}

// UpdateCloudProviderConfig is a reference to a ConfigMap containing the cloud provider deployment manifests.
// If you want to generate the cloud provider configuration, the cloud config will need a Harvester Endpoint.
// This is provider by `HarvesterCluster.Spec.ControlPlaneEndpoint`.
// Beware this does not work with an endpoint that uses a Rancher proxy!
type UpdateCloudProviderConfig struct {
	// ManifestsConfigMapNamespace is the namespace in which the required ConfigMap should be found.
	ManifestsConfigMapNamespace string `json:"manifestsConfigMapNamespace"`

	// ManifestsConfigMapName is the name of the required ConfigMap.
	ManifestsConfigMapName string `json:"manifestsConfigMapName"`

	// ManifestsConfigMapKey is the key in the ConfigMap that contains the cloud provider deployment manifests.
	ManifestsConfigMapKey string `json:"manifestsConfigMapKey"`

	// CloudConfigCredentialsSecretName is the name of the secret containing the cloud provider credentials.
	CloudConfigCredentialsSecretName string `json:"cloudConfigCredentialsSecretName"`

	// CloudConfigCredentialsSecretKey is the key in the secret that contains the cloud provider credentials.
	CloudConfigCredentialsSecretKey string `json:"cloudConfigCredentialsSecretKey"`
}

// HarvesterClusterStatus defines the observed state of HarvesterCluster.
type HarvesterClusterStatus struct {
	// Ready describes if the Harvester Cluster can be considered ready for machine creation.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Conditions defines current service state of the Harvester cluster.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Initialization provides observations of the HarvesterCluster initialization process.
	// Provisioned is the CAPI v1beta2 contract field (status.initialization.provisioned) that
	// the Cluster controller reads to determine infrastructure readiness. It is kept in sync with
	// status.ready (the v1beta1 contract field) so a single v1alpha1 object satisfies both contracts.
	// +optional
	Initialization Initialization `json:"initialization,omitempty"`

	// FailureDomains is the list of failure domains discovered on the target
	// Harvester cluster, published for the CAPI contract so control plane
	// machines can be spread across them. Domains are Harvester host
	// topology.kubernetes.io/zone labels when every host carries one, and the
	// host names otherwise; each domain records the node label to schedule on
	// in its attributes.
	// +optional
	FailureDomains []clusterv1.FailureDomain `json:"failureDomains,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready for HarvesterMachine"
// +kubebuilder:printcolumn:name="Server",type="string",JSONPath=".spec.server",description="Server is the address of the Harvester endpoint"
// +kubebuilder:printcolumn:name="ControlPlaneEndpoint",type="string",JSONPath=".spec.controlPlaneEndpoint[0]",description="API Endpoint",priority=1

// HarvesterCluster is the Schema for the harvesterclusters API.
type HarvesterCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HarvesterClusterSpec   `json:"spec"`
	Status HarvesterClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HarvesterClusterList contains a list of HarvesterCluster.
type HarvesterClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []HarvesterCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HarvesterCluster{}, &HarvesterClusterList{})
}

// GetConditions returns the set of conditions for this object.
// CAPI v1.12+ uses k8s-standard []metav1.Condition rather than its own
// clusterv1.Conditions type. This signature satisfies the conditions.Setter
// interface from sigs.k8s.io/cluster-api/util/conditions.
func (m *HarvesterCluster) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *HarvesterCluster) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}
