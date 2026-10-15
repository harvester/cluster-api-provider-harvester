package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
	locutil "github.com/harvester/cluster-api-provider-harvester/util"
)

// reconcileTwice runs two reconciliations, as the controller does after the first one
// publishes the Paused condition (v1beta2 contract) and returns, and returns the error
// of the second.
func reconcileTwice(ctx context.Context, r reconcile.Reconciler, key types.NamespacedName) error {
	request := ctrl.Request{NamespacedName: key}
	_, _ = r.Reconcile(ctx, request)
	_, err := r.Reconcile(ctx, request)

	return err
}

// Deleting a cluster whose provisioning failed usually means deleting everything at
// once (kubectl delete -f on the template): the Cluster, the HarvesterCluster and the
// identity Secret get a deletion timestamp together, while the HarvesterMachines are
// still being deleted and need both to reach Harvester.
var _ = Describe("Deletion after a failed provisioning", func() {
	const identitySecretName = "hv-identity"

	identitySecret := func(kubeconfig string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: identitySecretName, Namespace: "default"},
			Data:       map[string][]byte{locutil.ConfigSecretDataKey: []byte(kubeconfig)},
		}
	}

	harvesterCluster := func(deleting bool) *infrav1.HarvesterCluster {
		hvCluster := ownedHarvesterCluster()
		hvCluster.Spec.TargetNamespace = "default"
		hvCluster.Spec.IdentitySecret = infrav1.SecretKey{Namespace: "default", Name: identitySecretName}
		hvCluster.UID = types.UID("hv-cluster-uid")
		hvCluster.Finalizers = []string{infrav1.ClusterFinalizer}

		if deleting {
			hvCluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		}

		return hvCluster
	}

	newClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(pausedTestScheme()).
			WithObjects(objs...).
			WithStatusSubresource(&infrav1.HarvesterCluster{}, &infrav1.HarvesterMachine{}).
			Build()
	}

	deleteScope := func(cl client.Client, hvCluster *infrav1.HarvesterCluster, hvObjects ...*lbv1beta1.LoadBalancer) (*ClusterScope, *hvfake.Clientset) {
		hvClient := hvfake.NewSimpleClientset()
		for _, lb := range hvObjects {
			_, err := hvClient.LoadbalancerV1beta1().LoadBalancers(lb.Namespace).Create(context.TODO(), lb, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		}

		return &ClusterScope{
			Ctx:              context.TODO(),
			Cluster:          pausedTestCluster(false),
			HarvesterCluster: hvCluster,
			Logger:           log.FromContext(context.TODO()),
			HarvesterClient:  hvClient,
			ReconcileClient:  cl,
		}, hvClient
	}

	It("waits for the Harvester load balancer to be gone before releasing addresses and deleting pools", func() {
		hvCluster := harvesterCluster(true)
		hvCluster.Spec.LoadBalancerConfig = infrav1.LoadBalancerConfig{IPAMType: infrav1.POOL, IpPoolRef: "lb-pool"}
		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})
		scope, hvClient := deleteScope(newClient(), hvCluster,
			&lbv1beta1.LoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "default"}})

		_, err := hvClient.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "lb-pool"},
			Status:     lbv1beta1.IPPoolStatus{Allocated: map[string]string{"172.16.3.65": "default/" + lbName}},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// The Harvester load balancer controller releases its address before the
		// LoadBalancer goes away: until then it is still there.
		hvClient.PrependReactor("delete", "loadbalancers", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})

		result, err := (&HarvesterClusterReconciler{}).ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(hvCluster.Finalizers).To(ContainElement(infrav1.ClusterFinalizer))

		pool, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "lb-pool", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(pool.Status.Allocated).To(HaveLen(1), "nothing is released while the load balancer still exists")
	})

	It("releases the reserved address before deleting the load balancer pool it created", func() {
		hvCluster := harvesterCluster(true)
		hvCluster.Spec.LoadBalancerConfig = infrav1.LoadBalancerConfig{IPAMType: infrav1.POOL, IpPoolRef: "created-pool"}
		conditions.Set(hvCluster, metav1.Condition{
			Type: infrav1.CustomIPPoolCreatedCondition, Status: metav1.ConditionTrue, Reason: infrav1.CustomIPPoolCreatedSuccessfullyReason,
		})
		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})
		scope, hvClient := deleteScope(newClient(), hvCluster)

		_, err := hvClient.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "created-pool"},
			Status:     lbv1beta1.IPPoolStatus{Allocated: map[string]string{"10.30.0.10": "default/" + lbName}},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// The Harvester IPPool webhook refuses to delete a pool with allocated addresses.
		hvClient.PrependReactor("delete", "ippools", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deleteAction, ok := action.(k8stesting.DeleteAction)
			Expect(ok).To(BeTrue())

			obj, getErr := hvClient.Tracker().Get(lbv1beta1.SchemeGroupVersion.WithResource("ippools"), "", deleteAction.GetName())
			pool, isPool := obj.(*lbv1beta1.IPPool)

			if getErr == nil && isPool && len(pool.Status.Allocated) > 0 {
				return true, nil, errors.New("can't delete pool before releasing all the allocated IP")
			}

			return false, nil, nil
		})

		_, err = (&HarvesterClusterReconciler{}).ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())

		_, err = hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "created-pool", metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the pool must be deleted, got %v", err)
	})

	It("does not touch the IP pool of a DHCP load balancer", func() {
		hvCluster := harvesterCluster(true)
		hvCluster.Spec.LoadBalancerConfig = infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP, IpPoolRef: "unused-pool"}
		scope, hvClient := deleteScope(newClient(), hvCluster)
		hvClient.PrependReactor("get", "ippools", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("ippools is forbidden")
		})

		_, err := (&HarvesterClusterReconciler{}).ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(hvCluster.Finalizers).ToNot(ContainElement(infrav1.ClusterFinalizer))
	})

	It("releases the load balancer address reserved for a cluster whose provisioning failed", func() {
		// The placeholder load balancer address is reserved by CAPHV before any control
		// plane machine exists; the Harvester load balancer, which would release it, is
		// never created when the provisioning fails first.
		hvCluster := harvesterCluster(true)
		hvCluster.Spec.LoadBalancerConfig = infrav1.LoadBalancerConfig{IPAMType: infrav1.POOL, IpPoolRef: "lb-pool"}
		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})
		scope, hvClient := deleteScope(newClient(), hvCluster)

		_, err := hvClient.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "lb-pool"},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.3.65": "default/" + lbName,
					"172.16.3.66": "default/another-cluster-lb",
				},
				Available: 8,
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		_, err = (&HarvesterClusterReconciler{}).ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())

		pool, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "lb-pool", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(pool.Status.Allocated).To(Equal(map[string]string{"172.16.3.66": "default/another-cluster-lb"}))
		Expect(pool.Status.Available).To(Equal(int64(9)))
	})

	It("returns an error instead of panicking when the identity Secret holds an invalid kubeconfig", func(ctx SpecContext) {
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "hv-machine",
				Namespace:         "default",
				Finalizers:        []string{infrav1.MachineFinalizer},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       "owner-machine",
					UID:        types.UID("owner-machine-uid"),
				}},
			},
		}
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner-machine",
				Namespace: "default",
				Labels:    map[string]string{clusterv1.ClusterNameLabel: "owner-cluster"},
			},
			Spec: clusterv1.MachineSpec{ClusterName: "owner-cluster"},
		}

		cl := newClient(pausedTestCluster(false), machine, harvesterCluster(false),
			identitySecret("not a kubeconfig"), hvMachine)
		r := &HarvesterMachineReconciler{Client: cl, Scheme: pausedTestScheme()}

		err := reconcileTwice(ctx, r, types.NamespacedName{Name: "hv-machine", Namespace: "default"})
		Expect(err).To(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKeyFromObject(hvMachine), hvMachine)).To(Succeed())
		Expect(hvMachine.Finalizers).To(ContainElement(infrav1.MachineFinalizer))
	})

	It("keeps the HarvesterCluster and its load balancer while machines of the cluster remain", func() {
		remaining := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cp-0",
				Namespace: "default",
				Labels:    map[string]string{clusterv1.ClusterNameLabel: "owner-cluster"},
			},
		}
		hvCluster := harvesterCluster(true)
		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})
		scope, hvClient := deleteScope(newClient(remaining), hvCluster,
			&lbv1beta1.LoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "default"}})

		r := &HarvesterClusterReconciler{}
		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(hvCluster.Finalizers).To(ContainElement(infrav1.ClusterFinalizer))

		_, err = hvClient.LoadbalancerV1beta1().LoadBalancers("default").Get(context.TODO(), lbName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred(), "the control plane load balancer must stay while machines remain")
	})

	It("completes the deletion when Harvester answers but its deployment is not available", func(ctx SpecContext) {
		// Every request gets NotFound: the Harvester deployment check of a normal
		// reconcile fails, the deletion of already absent objects succeeds.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
		}))
		defer server.Close()

		kubeconfig := "apiVersion: v1\nkind: Config\nclusters:\n- name: h\n  cluster:\n    server: " + server.URL +
			"\ncontexts:\n- name: h\n  context:\n    cluster: h\n    user: u\ncurrent-context: h\nusers:\n- name: u\n  user:\n    token: t\n"

		cl := newClient(pausedTestCluster(false), harvesterCluster(true), identitySecret(kubeconfig))
		r := &HarvesterClusterReconciler{Client: cl, Scheme: pausedTestScheme()}

		err := reconcileTwice(ctx, r, types.NamespacedName{Name: "hv-cluster", Namespace: "default"})
		Expect(err).ToNot(HaveOccurred())

		err = cl.Get(ctx, types.NamespacedName{Name: "hv-cluster", Namespace: "default"}, &infrav1.HarvesterCluster{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the HarvesterCluster must be gone, got %v", err)
	})

	It("does not protect the identity Secret before the HarvesterCluster has its own finalizer", func(ctx SpecContext) {
		// A finalizer on the Secret set before the HarvesterCluster has its own would
		// outlive a cluster deleted right away, without its deletion running.
		hvCluster := harvesterCluster(false)
		hvCluster.Finalizers = nil
		cl := newClient(identitySecret("unused"), hvCluster)
		scope, _ := deleteScope(cl, hvCluster)

		_, err := (&HarvesterClusterReconciler{Client: cl}).ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(hvCluster.Finalizers).To(ContainElement(infrav1.ClusterFinalizer))

		secret := &corev1.Secret{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: identitySecretName, Namespace: "default"}, secret)).To(Succeed())
		Expect(secret.Finalizers).To(BeEmpty())
	})

	It("protects the identity Secret once the HarvesterCluster has its finalizer", func(ctx SpecContext) {
		hvCluster := harvesterCluster(false)
		cl := newClient(identitySecret("unused"), hvCluster)
		scope, _ := deleteScope(cl, hvCluster)

		// The rest of the normal reconciliation does not matter here.
		_, _ = (&HarvesterClusterReconciler{Client: cl}).ReconcileNormal(scope)

		secret := &corev1.Secret{}
		Expect(cl.Get(ctx, types.NamespacedName{Name: identitySecretName, Namespace: "default"}, secret)).To(Succeed())
		Expect(secret.Finalizers).To(ConsistOf(infrav1.IdentitySecretFinalizerPrefix + "hv-cluster-uid"))
	})

	It("releases only its own finalizer from a shared identity Secret", func(ctx SpecContext) {
		secret := identitySecret("unused")
		secret.Finalizers = []string{
			infrav1.IdentitySecretFinalizerPrefix + "hv-cluster-uid",
			infrav1.IdentitySecretFinalizerPrefix + "other-cluster-uid",
		}
		hvCluster := harvesterCluster(true)
		cl := newClient(secret, hvCluster)
		scope, _ := deleteScope(cl, hvCluster)

		_, err := (&HarvesterClusterReconciler{}).ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
		Expect(secret.Finalizers).To(ConsistOf(infrav1.IdentitySecretFinalizerPrefix + "other-cluster-uid"))
	})
})
