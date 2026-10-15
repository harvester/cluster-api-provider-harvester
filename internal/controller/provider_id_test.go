package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

// workloadNodeServer serves a single Node of a workload cluster API and applies merge
// patches to it.
func workloadNodeServer(node *corev1.Node) (*httptest.Server, func() corev1.Node) {
	var mu sync.Mutex

	current := node.DeepCopy()

	write := func(w http.ResponseWriter, body any) {
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(body)).To(Succeed())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.URL.Path == "/api":
			write(w, metav1.APIVersions{TypeMeta: metav1.TypeMeta{Kind: "APIVersions"}, Versions: []string{"v1"}})
		case r.URL.Path == "/apis":
			write(w, metav1.APIGroupList{TypeMeta: metav1.TypeMeta{Kind: "APIGroupList"}})
		case r.URL.Path == "/api/v1":
			write(w, metav1.APIResourceList{
				TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList"},
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{{Name: "nodes", Kind: "Node", Verbs: []string{"get", "patch"}}},
			})
		case r.URL.Path == "/api/v1/nodes/"+current.Name && r.Method == http.MethodGet:
			current.APIVersion, current.Kind = "v1", "Node"
			write(w, current)
		case r.URL.Path == "/api/v1/nodes/"+current.Name && r.Method == http.MethodPatch:
			var patch struct {
				Spec struct {
					ProviderID *string         `json:"providerID"`
					Taints     *[]corev1.Taint `json:"taints"`
				} `json:"spec"`
			}

			Expect(json.NewDecoder(r.Body).Decode(&patch)).To(Succeed())

			if patch.Spec.ProviderID != nil {
				current.Spec.ProviderID = *patch.Spec.ProviderID
			}

			if patch.Spec.Taints != nil {
				current.Spec.Taints = *patch.Spec.Taints
			}

			write(w, current)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, func() corev1.Node {
		mu.Lock()
		defer mu.Unlock()

		return *current.DeepCopy()
	}
}

var _ = Describe("Provider ID of a machine whose node registered first", func() {
	It("uses the Harvester provider ID instead of waiting for another component to set it", func() {
		// In HA, the second and third control plane nodes register while the first one
		// already serves the workload API: the node exists, without provider ID yet.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1"},
			Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
				Key: "node.cloudprovider.kubernetes.io/uninitialized", Value: "true", Effect: corev1.TaintEffectNoSchedule,
			}}},
		}

		server, currentNode := workloadNodeServer(node)
		defer server.Close()

		kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: w\n  cluster:\n    server: " + server.URL +
			"\ncontexts:\n- name: w\n  context:\n    cluster: w\n    user: u\ncurrent-context: w\nusers:\n- name: u\n  user:\n    token: t\n"

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-kubeconfig", Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte(kubeconfig)},
			},
		).Build()

		strategy := kubevirtv1.RunStrategyAlways
		hvClient := hvfake.NewSimpleClientset(
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default", UID: "vm-uid-1"},
				Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
			},
			&kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default"},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{{IP: "172.16.3.43", Name: "nic-1"}},
				},
			},
		)

		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "test-ns", Finalizers: []string{infrav1.MachineFinalizer}},
			Status:     infrav1.HarvesterMachineStatus{Ready: true},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type: infrav1.MachineCreatedCondition, Status: metav1.ConditionTrue, Reason: infrav1.MachineCreatedCondition,
		})

		logger := log.FromContext(context.TODO())
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"}},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		_, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())

		Expect(hvMachine.Spec.ProviderID).To(Equal("harvester://vm-uid-1"))
		Expect(currentNode().Spec.ProviderID).To(Equal("harvester://vm-uid-1"))
	})

	It("adopts the provider ID another component set first on the node", func() {
		// A cluster still running the RKE2 embedded cloud controller: it set rke2://
		// before CAPHV got to the node. The node provider ID cannot change any more.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1"},
			Spec:       corev1.NodeSpec{ProviderID: "rke2://test-cp-1"},
		}

		server, currentNode := workloadNodeServer(node)
		defer server.Close()

		kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: w\n  cluster:\n    server: " + server.URL +
			"\ncontexts:\n- name: w\n  context:\n    cluster: w\n    user: u\ncurrent-context: w\nusers:\n- name: u\n  user:\n    token: t\n"

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-kubeconfig", Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte(kubeconfig)},
			},
		).Build()

		strategy := kubevirtv1.RunStrategyAlways
		hvClient := hvfake.NewSimpleClientset(
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default", UID: "vm-uid-1"},
				Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
			},
			&kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default"},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{{IP: "172.16.3.43", Name: "nic-1"}},
				},
			},
		)

		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "test-ns", Finalizers: []string{infrav1.MachineFinalizer}},
			Status:     infrav1.HarvesterMachineStatus{Ready: true},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type: infrav1.MachineCreatedCondition, Status: metav1.ConditionTrue, Reason: infrav1.MachineCreatedCondition,
		})

		logger := log.FromContext(context.TODO())
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"}},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		_, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())

		Expect(hvMachine.Spec.ProviderID).To(Equal("rke2://test-cp-1"), "the Machine must match its Node")
		Expect(currentNode().Spec.ProviderID).To(Equal("rke2://test-cp-1"))
	})

	It("checks a provider ID mismatch again only every few minutes", func() {
		// A cluster still running the RKE2 embedded cloud controller: it set rke2://
		// before CAPHV got to the node. The node provider ID cannot change any more.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1"},
			Spec:       corev1.NodeSpec{ProviderID: "rke2://test-cp-1"},
		}

		server, currentNode := workloadNodeServer(node)
		defer server.Close()

		kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: w\n  cluster:\n    server: " + server.URL +
			"\ncontexts:\n- name: w\n  context:\n    cluster: w\n    user: u\ncurrent-context: w\nusers:\n- name: u\n  user:\n    token: t\n"

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-kubeconfig", Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte(kubeconfig)},
			},
		).Build()

		strategy := kubevirtv1.RunStrategyAlways
		hvClient := hvfake.NewSimpleClientset(
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default", UID: "vm-uid-1"},
				Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
			},
			&kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "default"},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{{IP: "172.16.3.43", Name: "nic-1"}},
				},
			},
		)

		// The machine provider ID was chosen before the node registered.
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "test-ns", Finalizers: []string{infrav1.MachineFinalizer}},
			Spec:       infrav1.HarvesterMachineSpec{ProviderID: "harvester://vm-uid-1"},
			Status:     infrav1.HarvesterMachineStatus{Ready: true},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type: infrav1.MachineCreatedCondition, Status: metav1.ConditionTrue, Reason: infrav1.MachineCreatedCondition,
		})

		logger := log.FromContext(context.TODO())
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"}},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueTimeLong))
		Expect(conditions.IsFalse(hvMachine, infrav1.NodeProviderIDMatchesCondition)).To(BeTrue())
		Expect(currentNode().Spec.ProviderID).To(Equal("rke2://test-cp-1"))
	})

	It("reports a node whose provider ID differs from the machine one instead of treating it as initialized", func() {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-2"},
			Spec:       corev1.NodeSpec{ProviderID: "rke2://test-cp-2"},
		}

		server, _ := workloadNodeServer(node)
		defer server.Close()

		kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: w\n  cluster:\n    server: " + server.URL +
			"\ncontexts:\n- name: w\n  context:\n    cluster: w\n    user: u\ncurrent-context: w\nusers:\n- name: u\n  user:\n    token: t\n"

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-kubeconfig", Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte(kubeconfig)},
		}).Build()

		logger := log.FromContext(context.TODO())
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-2", Namespace: "test-ns"},
			Spec:       infrav1.HarvesterMachineSpec{ProviderID: "harvester://vm-uid-2"},
		}
		scope := &Scope{
			Ctx:              context.TODO(),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"}},
			HarvesterMachine: hvMachine,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		Expect(r.initializeWorkloadNode(scope)).To(BeFalse())

		condition := conditions.Get(hvMachine, infrav1.NodeProviderIDMatchesCondition)
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal(infrav1.NodeProviderIDMismatchReason))
		Expect(condition.Message).To(And(ContainSubstring("rke2://test-cp-2"), ContainSubstring("harvester://vm-uid-2")))
		Expect(strings.Contains(condition.Message, "cloudController")).To(BeTrue(), "the message must point to the fix")
	})
})
