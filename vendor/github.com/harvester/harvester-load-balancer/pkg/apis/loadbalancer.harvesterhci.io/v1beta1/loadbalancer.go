package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=lb;lbs,scope=Namespaced
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="WORKLOADTYPE",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="IPAM",type=string,JSONPath=`.spec.ipam`
// +kubebuilder:printcolumn:name="ADDRESS",type=string,JSONPath=`.status.address`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LoadBalancerSpec   `json:"spec"`
	Status            LoadBalancerStatus `json:"status,omitempty"`
}

type LoadBalancerSpec struct {
	// +optional
	Description string `json:"description,omitempty"`
	// +optional
	WorkloadType WorkloadType `json:"workloadType,omitempty"`
	// +optional
	IPAM IPAM `json:"ipam,omitempty"`
	// +optional
	IPPool    string     `json:"ipPool,omitempty"`
	Listeners []Listener `json:"listeners,omitempty"`
	// +optional
	BackendServerSelector map[string][]string `json:"backendServerSelector,omitempty"`
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

type LoadBalancerStatus struct {
	// +optional
	BackendServers []string `json:"backendServers,omitempty"`
	// +optional
	AllocatedAddress AllocatedAddress `json:"allocatedAddress,omitempty"`
	// +optional
	Address string `json:"address,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type AllocatedAddress struct {
	IPPool  string `json:"ipPool,omitempty"`
	IP      string `json:"ip,omitempty"`
	Mask    string `json:"mask,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

type Listener struct {
	// +optional
	Name        string          `json:"name"`
	Port        int32           `json:"port"`
	Protocol    corev1.Protocol `json:"protocol"`
	BackendPort int32           `json:"backendPort"`
}

type HealthCheck struct {
	Port uint `json:"port,omitempty"`
	// +optional
	SuccessThreshold uint `json:"successThreshold,omitempty"`
	// +optional
	FailureThreshold uint `json:"failureThreshold,omitempty"`
	// +optional
	PeriodSeconds uint `json:"periodSeconds,omitempty"`
	// +optional
	TimeoutSeconds uint `json:"timeoutSeconds,omitempty"`
}

type Condition struct {
	// Type of the condition.
	Type condition.Cond `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

const LoadBalancerReady condition.Cond = "Ready"

// +kubebuilder:validation:Enum=vm;cluster
type WorkloadType string

const (
	VM      WorkloadType = "vm"
	Cluster WorkloadType = "cluster"
)

// +kubebuilder:validation:Enum=pool;dhcp
type IPAM string

var (
	Pool IPAM = "pool"
	DHCP IPAM = "dhcp"
)
