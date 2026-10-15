package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr/funcr"
	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

// The bootstrap data carries the credentials a node joins the cluster with (RKE2 or
// kubeadm token): it must not reach the controller logs, at any verbosity.
var _ = Describe("Bootstrap data in the controller logs", func() {
	It("never logs the cloud-init user data", func() {
		const joinToken = "K10-join-token-that-must-stay-secret" //nolint:gosec // fake value the test looks for

		hvClient := hvfake.NewSimpleClientset(&harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-ed25519 AAAA test@test"},
		})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		bootstrapData := "write_files:\n  - path: /etc/rancher/rke2/config.yaml\n    content: \"token: " + joinToken + "\"\n"
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testBootstrapDataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte(bootstrapData)},
		}).Build()

		var logs strings.Builder

		logger := funcr.New(func(prefix, args string) { logs.WriteString(prefix + " " + args + "\n") }, funcr.Options{Verbosity: 10})

		dataSecretName := testBootstrapDataSecretName
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi", SSHKeyPair: "default/capi-ssh-key", Networks: []string{"default/production"},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"}},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		_, err := buildVMTemplate(scope, []diskInfo{{pvcName: "test-cp-0-disk-0-abc", index: 0}}, map[string]string{})
		Expect(err).ToNot(HaveOccurred())

		Expect(logs.String()).ToNot(ContainSubstring(joinToken))
	})
})
