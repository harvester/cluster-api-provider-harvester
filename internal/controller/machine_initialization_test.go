package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// The CRDs carry the cluster.x-k8s.io/v1beta2 contract label, so the CAPI core reads
// status.initialization.provisioned of a HarvesterMachine, not status.ready. Reconcile
// paths that only set status.ready used to leave the Machine Provisioning forever.
var _ = Describe("HarvesterMachine initialization.provisioned (v1beta2 contract)", func() {
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: "hv-machine", Namespace: "default"}}

	reconcileMachine := func(ctx SpecContext, status infrav1.HarvesterMachineStatus) *infrav1.HarvesterMachine {
		hvMachine := &infrav1.HarvesterMachine{
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
			Status: status,
		}
		ownerMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner-machine",
				Namespace: "default",
				Labels:    map[string]string{clusterv1.ClusterNameLabel: "owner-cluster"},
			},
			Spec: clusterv1.MachineSpec{ClusterName: "owner-cluster"},
		}

		scheme := pausedTestScheme()
		cl := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pausedTestCluster(false), ownerMachine, hvMachine).
			WithStatusSubresource(&infrav1.HarvesterMachine{}).
			Build()
		reconciler := &HarvesterMachineReconciler{Client: cl, Scheme: scheme}

		// The reconcile stops early (it publishes the Paused condition first, and no
		// HarvesterCluster exists): whatever path it takes, the status must be reconciled
		// at the single point where it is patched.
		_, _ = reconciler.Reconcile(ctx, request)

		patched := &infrav1.HarvesterMachine{}
		Expect(reconciler.Get(ctx, client.ObjectKeyFromObject(hvMachine), patched)).To(Succeed())

		return patched
	}

	It("reports provisioned for a ready machine whose initialization was never set", func(ctx SpecContext) {
		// The state observed on every machine of a stuck cluster: ready=true, initialization={}.
		hvMachine := reconcileMachine(ctx, infrav1.HarvesterMachineStatus{Ready: true})

		Expect(hvMachine.Status.Ready).To(BeTrue())
		Expect(hvMachine.Status.Initialization.Provisioned).To(BeTrue())
	})

	It("does not report provisioned for a machine that is not ready", func(ctx SpecContext) {
		hvMachine := reconcileMachine(ctx, infrav1.HarvesterMachineStatus{Ready: false})

		Expect(hvMachine.Status.Ready).To(BeFalse())
		Expect(hvMachine.Status.Initialization.Provisioned).To(BeFalse())
	})

	It("keeps provisioned once set, even when the machine is no longer ready", func(ctx SpecContext) {
		// provisioned only orchestrates the initial provisioning (Cluster API latches
		// InfrastructureProvisioned), so it is never cleared, as in CAPK and CAPV.
		hvMachine := reconcileMachine(ctx, infrav1.HarvesterMachineStatus{
			Ready:          false,
			Initialization: infrav1.Initialization{Provisioned: true},
		})

		Expect(hvMachine.Status.Ready).To(BeFalse())
		Expect(hvMachine.Status.Initialization.Provisioned).To(BeTrue())
	})
})
