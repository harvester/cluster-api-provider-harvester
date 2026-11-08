package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=pool;pools,scope=Cluster
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="RANGES",type=string,JSONPath=`.spec.ranges`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.selector.priority`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type IPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IPPoolSpec   `json:"spec"`
	Status            IPPoolStatus `json:"status,omitempty"`
}

type IPPoolSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	Ranges []Range `json:"ranges"`
	// +optional
	Selector Selector `json:"selector"`
}

// Range refers to github.com/containernetworking/plugins/plugins/ipam/host-local/backend/allocator.Range
type Range struct {
	RangeStart string `json:"rangeStart,omitempty"` // The first ip, inclusive
	RangeEnd   string `json:"rangeEnd,omitempty"`   // The last ip, inclusive
	Subnet     string `json:"subnet"`
	Gateway    string `json:"gateway,omitempty"`
}

type Selector struct {
	// +optional
	Priority uint32 `json:"priority,omitempty"`
	// +optional
	Network string `json:"network,omitempty"`
	// +optional
	Scope []Tuple `json:"scope,omitempty"`
}

type Tuple struct {
	Project      string `json:"project,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	GuestCluster string `json:"guestCluster,omitempty"`
}

type IPPoolStatus struct {
	Total int64 `json:"total"`

	Available int64 `json:"available"`

	LastAllocated string `json:"lastAllocated"`
	// +optional
	Allocated map[string]string `json:"allocated,omitempty"`
	// +optional
	AllocatedHistory map[string]string `json:"allocatedHistory,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

var (
	IPPoolReady condition.Cond = "Ready"
)
