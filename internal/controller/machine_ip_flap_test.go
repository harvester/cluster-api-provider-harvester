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

package controller

import (
	"context"

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

var _ = Describe("HarvesterMachine provisioned when the guest IP disappears for a while", func() {
	It("reports provisioned again once the IP is back", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}).Build()

		strategy := kubevirtv1.RunStrategyAlways
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-md-0", Namespace: "default", UID: "vm-uid"},
			Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-md-0", Namespace: "default"},
		}
		hvClient := hvfake.NewSimpleClientset(vm, vmi)
		logger := log.FromContext(context.TODO())

		// A running machine: provider ID set, ready and provisioned.
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-md-0", Namespace: "test-ns", Finalizers: []string{infrav1.MachineFinalizer}},
			Spec:       infrav1.HarvesterMachineSpec{ProviderID: "harvester://vm-uid"},
			Status: infrav1.HarvesterMachineStatus{
				Ready:          true,
				Initialization: infrav1.Initialization{Provisioned: true},
			},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type: infrav1.MachineCreatedCondition, Status: metav1.ConditionTrue, Reason: infrav1.MachineCreatedCondition,
		})

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

		// One reconcile as Reconcile runs it: ReconcileNormal, then the deferred patch.
		pass := func() {
			_, err := r.ReconcileNormal(scope)
			Expect(err).ToNot(HaveOccurred())
			markProvisionedOnceReady(scope.HarvesterMachine.Status.Ready, &scope.HarvesterMachine.Status.Initialization)
		}

		setGuestIP := func(ip string) {
			current, err := hvClient.KubevirtV1().VirtualMachineInstances("default").Get(context.TODO(), "test-md-0", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			current.Status.Interfaces = []kubevirtv1.VirtualMachineInstanceNetworkInterface{{Name: "nic-1", IP: ip}}
			_, err = hvClient.KubevirtV1().VirtualMachineInstances("default").Update(context.TODO(), current, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())
		}

		// The guest agent reports the NIC without an address for a while (while
		// the guest reconfigures its network, for instance).
		setGuestIP("")
		pass() // ready -> not ready
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
		Expect(scope.HarvesterMachine.Status.Initialization.Provisioned).To(BeTrue(),
			"provisioned is set once and never cleared")
		pass() // still no address, provider ID set: the end of ReconcileNormal sets ready again

		setGuestIP("172.16.3.42")
		pass()
		pass()

		Expect(scope.HarvesterMachine.Status.Ready).To(BeTrue())
		Expect(scope.HarvesterMachine.Status.Initialization.Provisioned).To(BeTrue(),
			"ready is true again, provisioned must follow, or a Machine that has not recorded it yet waits forever")
	})
})
