package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

func pausedTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(infrav1.AddToScheme(scheme)).To(Succeed())
	Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	return scheme
}

func pausedTestCluster(paused bool) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-cluster", Namespace: "default"},
		Spec: clusterv1.ClusterSpec{
			Paused: ptr.To(paused),
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: infrav1.GroupVersion.Group,
				Kind:     "HarvesterCluster",
				Name:     "hv-cluster",
			},
		},
	}
}

func ownedHarvesterCluster() *infrav1.HarvesterCluster {
	return &infrav1.HarvesterCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hv-cluster",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       "owner-cluster",
				UID:        types.UID("owner-cluster-uid"),
			}},
		},
	}
}

var _ = Describe("Paused condition on HarvesterCluster (v1beta2 contract)", func() {
	newReconciler := func(objs ...client.Object) *HarvesterClusterReconciler {
		scheme := pausedTestScheme()
		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&infrav1.HarvesterCluster{}).
			Build()

		return &HarvesterClusterReconciler{Client: cl, Scheme: scheme}
	}

	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: "hv-cluster", Namespace: "default"}}

	It("sets Paused=True and skips reconciliation when the owner Cluster is paused", func(ctx SpecContext) {
		reconciler := newReconciler(pausedTestCluster(true), ownedHarvesterCluster())

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).ToNot(HaveOccurred())

		hvCluster := &infrav1.HarvesterCluster{}
		Expect(reconciler.Get(ctx, request.NamespacedName, hvCluster)).To(Succeed())

		Expect(meta.IsStatusConditionTrue(hvCluster.Status.Conditions, clusterv1.PausedCondition)).
			To(BeTrue(), "Paused condition must be True, conditions: %v", hvCluster.Status.Conditions)
		// Reconciliation must stop before touching Harvester: no connection condition.
		Expect(meta.FindStatusCondition(hvCluster.Status.Conditions, infrav1.HarvesterConnectionReadyCondition)).
			To(BeNil(), "reconciliation must not proceed while paused")
	})

	It("sets Paused=False and proceeds when the owner Cluster is not paused", func(ctx SpecContext) {
		reconciler := newReconciler(pausedTestCluster(false), ownedHarvesterCluster())

		// The reconciliation proceeds and fails further down (no identity secret): the
		// error is expected, only the Paused condition matters here.
		_, _ = reconciler.Reconcile(ctx, request)

		hvCluster := &infrav1.HarvesterCluster{}
		Expect(reconciler.Get(ctx, request.NamespacedName, hvCluster)).To(Succeed())

		pausedCondition := meta.FindStatusCondition(hvCluster.Status.Conditions, clusterv1.PausedCondition)
		Expect(pausedCondition).ToNot(BeNil(), "Paused condition must be present, conditions: %v", hvCluster.Status.Conditions)
		Expect(pausedCondition.Status).To(Equal(metav1.ConditionFalse))
	})
})

var _ = Describe("Paused condition on HarvesterMachine (v1beta2 contract)", func() {
	newReconciler := func(objs ...client.Object) *HarvesterMachineReconciler {
		scheme := pausedTestScheme()
		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&infrav1.HarvesterMachine{}).
			Build()

		return &HarvesterMachineReconciler{Client: cl, Scheme: scheme}
	}

	ownerMachine := func() *clusterv1.Machine {
		return &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner-machine",
				Namespace: "default",
				Labels:    map[string]string{clusterv1.ClusterNameLabel: "owner-cluster"},
			},
			Spec: clusterv1.MachineSpec{ClusterName: "owner-cluster"},
		}
	}

	ownedHarvesterMachine := func() *infrav1.HarvesterMachine {
		return &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hv-machine",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       "owner-machine",
					UID:        types.UID("owner-machine-uid"),
				}},
			},
		}
	}

	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: "hv-machine", Namespace: "default"}}

	It("sets Paused=True and returns before fetching the HarvesterCluster when the owner Cluster is paused", func(ctx SpecContext) {
		// No HarvesterCluster object exists: if the paused check ran after the
		// HarvesterCluster lookup, the reconciliation would error out.
		reconciler := newReconciler(pausedTestCluster(true), ownerMachine(), ownedHarvesterMachine())

		_, err := reconciler.Reconcile(ctx, request)
		Expect(err).ToNot(HaveOccurred())

		hvMachine := &infrav1.HarvesterMachine{}
		Expect(reconciler.Get(ctx, request.NamespacedName, hvMachine)).To(Succeed())

		Expect(meta.IsStatusConditionTrue(hvMachine.Status.Conditions, clusterv1.PausedCondition)).
			To(BeTrue(), "Paused condition must be True, conditions: %v", hvMachine.Status.Conditions)
	})

	It("sets Paused=False when the owner Cluster is not paused", func(ctx SpecContext) {
		// Reconciliation proceeds and fails on the missing HarvesterCluster: the error
		// is expected, only the Paused condition matters here.
		reconciler := newReconciler(pausedTestCluster(false), ownerMachine(), ownedHarvesterMachine())

		_, _ = reconciler.Reconcile(ctx, request)

		hvMachine := &infrav1.HarvesterMachine{}
		Expect(reconciler.Get(ctx, request.NamespacedName, hvMachine)).To(Succeed())

		pausedCondition := meta.FindStatusCondition(hvMachine.Status.Conditions, clusterv1.PausedCondition)
		Expect(pausedCondition).ToNot(BeNil(), "Paused condition must be present, conditions: %v", hvMachine.Status.Conditions)
		Expect(pausedCondition.Status).To(Equal(metav1.ConditionFalse))
	})
})

var _ = Describe("Deprecated terminal failure fields (v1beta2 contract)", func() {
	It("does not write FailureReason/FailureMessage when the identity secret is missing", func(ctx SpecContext) {
		scheme := pausedTestScheme()
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler := &HarvesterClusterReconciler{Client: cl, Scheme: scheme}

		hvCluster := ownedHarvesterCluster()
		_, err := reconciler.reconcileHarvesterConfig(ctx, hvCluster)
		Expect(err).To(HaveOccurred(), "the identity secret is missing")

		// v1beta2 removed terminal failures — v1beta1 has no failure fields at all,
		// so the error can only surface through the connection condition.
		Expect(meta.IsStatusConditionFalse(hvCluster.Status.Conditions, infrav1.HarvesterConnectionReadyCondition)).
			To(BeTrue(), "the failure must surface via the connection condition")
	})
})
