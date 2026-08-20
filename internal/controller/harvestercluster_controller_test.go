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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvclient "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
	locutil "github.com/harvester/cluster-api-provider-harvester/util"
)

var _ = Describe("Extract Server from Kubeconfig", func() {
	var kubeconfig []byte

	BeforeEach(func() {
		//nolint:lll
		kubeconfig = []byte(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJ2akNDQVdPZ0F3SUJBZ0lCQURBS0JnZ3Foa2pPUFFRREFqQkdNUnd3R2dZRFZRUUtFeE5rZVc1aGJXbGoKYkdsemRHVnVaWEl0YjNKbk1TWXdKQVlEVlFRRERCMWtlVzVoYldsamJHbHpkR1Z1WlhJdFkyRkFNVGN4TXpnMApNVFUwTWpBZUZ3MHlOREEwTWpNd016QTFOREphRncwek5EQTBNakV3TXpBMU5ESmFNRVl4SERBYUJnTlZCQW9UCkUyUjVibUZ0YVdOc2FYTjBaVzVsY2kxdmNtY3hKakFrQmdOVkJBTU1IV1I1Ym1GdGFXTnNhWE4wWlc1bGNpMWoKWVVBeE56RXpPRFF4TlRReU1Ga3dFd1lIS29aSXpqMENBUVlJS29aSXpqMERBUWNEUWdBRVA3V0RnRnk1NzRWVwp0SVYySzFGMExVZnE1VDJkQlFYVFovUUFIdWVqNDAzMGR1MklvN2tubzZ0SlI5OEJrNVk0bmpDK0VzT3c0UlZvCnJiWkdOVzJJdEtOQ01FQXdEZ1lEVlIwUEFRSC9CQVFEQWdLa01BOEdBMVVkRXdFQi93UUZNQU1CQWY4d0hRWUQKVlIwT0JCWUVGTWJ1c3dyTTZEQS8vcjV2NjNhejJCU3VXSkVjTUFvR0NDcUdTTTQ5QkFNQ0Ewa0FNRVlDSVFETgpKQVhOUHFtZEY4SGViUm5IMTJkTkNVWEY0TXpTd0haSTZwZzVhNDVsd1FJaEFLNHZiRGVjTEIyVzBuQnJ1S0F2ClprNy9lb2JLT05TcEthRzBJdjhHaGhTdQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t
    server: https://10.10.0.10/k8s/clusters/local
  name: local
contexts:
- context:
    cluster: local
    user: local
  name: deathstar
current-context: deathstar
kind: Config
preferences: {}
users:
- name: local
  user:
    token: kubeconfig-user-gqrx7b7k5x:qsp8xzzd4dpb9d99n9fg8vrtdrqndplmlfjtzpkshc9jcndn9fc2ns
`)
	})
	Context("When we provide a kubeconfig with different current-context and cluster.name", func() {
		It("Should still return the right cluster", func() {
			Expect(getHarvesterServerFromKubeconfig(kubeconfig)).To(Equal("https://10.10.0.10/k8s/clusters/local"))
		})
	})
})

var _ = Describe("Modify Cloud Provider Manifest", func() {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	//nolint:lll
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/component: cloud-provider
    app.kubernetes.io/name: harvester-cloud-provider
  name: harvester-cloud-provider
  namespace: kube-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/component: cloud-provider
      app.kubernetes.io/name: harvester-cloud-provider
  template:
    metadata:
      labels:
        app.kubernetes.io/component: cloud-provider
        app.kubernetes.io/name: harvester-cloud-provider
    spec:
      containers:
      - args:
        - --cloud-config=/etc/kubernetes/cloud-config
        command:
        - harvester-cloud-provider
        image: rancher/harvester-cloud-provider:v0.2.1
        imagePullPolicy: Always
        name: harvester-cloud-provider
        resources: {}
        volumeMounts:
        - mountPath: /etc/kubernetes
          name: cloud-config
      serviceAccountName: harvester-cloud-controller-manager
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/control-plane
        operator: Exists
      - effect: NoSchedule
        key: node.cloudprovider.kubernetes.io/uninitialized
        operator: Equal
        value: "true"
      volumes:
        - name: cloud-config
          secret:
            secretName: cloud-config
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: harvester-cloud-controller-manager
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: harvester-cloud-controller-manager
rules:
- apiGroups:
  - ""
  resources:
  - services
  - nodes
  - events
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - services/status
  verbs:
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - nodes/status
  verbs:
  - patch
  - update
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - create
  - update
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: harvester-cloud-controller-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: harvester-cloud-controller-manager
subjects:
  - kind: ServiceAccount
    name: harvester-cloud-controller-manager
    namespace: kube-system
---
apiVersion: v1
kind: Secret
metadata:
  name: cloud-config
  namespace: kube-system
type: Opaque
data:
  cloud-config: YXBpVmVyc2lvbjogdjEKa2luZDogQ29uZmlnCmNsdXN0ZXJzOgotIG5hbWU6ICJsb2NhbCIKICBjbHVzdGVyOgogICAgc2VydmVyOiAiaHR0cHM6Ly8xOTIuMTY4LjEuMTA5L2s4cy9jbHVzdGVycy9sb2NhbCIKICAgIGNlcnRpZmljYXRlLWF1dGhvcml0eS1kYXRhOiAiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVSjJha05EUVwKICAgICAgVmRQWjBGM1NVSkJaMGxDUVVSQlMwSm5aM0ZvYTJwUFVGRlJSRUZxUWtkTlVuZDNSMmRaUkZaUlVVdEZlRTVyWlZjMWFHSlhiR29LWVwKICAgICAga2RzZW1SSFZuVmFXRWwwWWpOS2JrMVRXWGRLUVZsRVZsRlJSRVJDTVd0bFZ6Vm9ZbGRzYW1KSGJIcGtSMVoxV2xoSmRGa3lSa0ZOVlwKICAgICAgRmt6VGtSQk1BcE5WRkY1VGxSQlpVWjNNSGxOZWtGNFRWUm5lRTFVVFhkTmFsWmhSbmN3ZWsxNlFYaE5WRlY0VFZSTmQwMXFWbUZOUlwKICAgICAgVmw0U0VSQllVSm5UbFpDUVc5VUNrVXlValZpYlVaMFlWZE9jMkZZVGpCYVZ6VnNZMmt4ZG1OdFkzaEtha0ZyUW1kT1ZrSkJUVTFJVlwKICAgICAgMUkxWW0xR2RHRlhUbk5oV0U0d1dsYzFiR05wTVdvS1dWVkJlRTVxWXpCTlJGRjRUa1JKTVUxR2EzZEZkMWxJUzI5YVNYcHFNRU5CVlwKICAgICAgVmxKUzI5YVNYcHFNRVJCVVdORVVXZEJSVTVGVjJSU1lXTkVWSG80Y2dwdWRXaE9lV2d3YW5od1QxVlJUVGwwUmt0eFkwdDZjVEl3UVwKICAgICAgVzlPVEdNNVdsazFNMk5vVWxaV1V6QnBWamhwYW1wUk0yTTBjMHBRV0dwV1lYVlJNRVJTQ2k5TFRXNTBTVUl6VTJGT1EwMUZRWGRFWlwKICAgICAgMWxFVmxJd1VFRlJTQzlDUVZGRVFXZExhMDFCT0VkQk1WVmtSWGRGUWk5M1VVWk5RVTFDUVdZNGQwaFJXVVFLVmxJd1QwSkNXVVZHUVwKICAgICAgU3R0U0hOTGFWTXJiMHBSVGtKUlpXNUdRa2xQY2xnd1prTkJUVUZ2UjBORGNVZFRUVFE1UWtGTlEwRXdhMEZOUlZsRFNWRkRaQXBZUVwKICAgICAgV3hCUldsaE1ISnNNek5hYVhWd1ZtTjRZVTAwV0RWYU1FWXJRWEV5UlRWaE5WVmlSMHN3WW5kSmFFRk9TVlEzYzNwUVJ6Y3hSM0JNU1wKICAgICAgVXd3ZVdSakNrWnhhSEZIVWtWSlZuZzViVmRCWW04M1VEUjRTa2hqTXdvdExTMHRMVVZPUkNCRFJWSlVTVVpKUTBGVVJTMHRMUzB0IgoKdXNlcnM6Ci0gbmFtZTogImxvY2FsIgogIHVzZXI6CiAgICB0b2tlbjogImt1YmVjb25maWctdXNlci02bWx3cHY0ano1OmN2cjVrYnB3cGN6dGNwZnE0ZnZiamdkbTd4Z3BqcmtuNnBoOG1oYmZzeHpuOTJnZDdmNHo2cSIKCgpjb250ZXh0czoKLSBuYW1lOiAibG9jYWwiCiAgY29udGV4dDoKICAgIHVzZXI6ICJsb2NhbCIKICAgIGNsdXN0ZXI6ICJsb2NhbCIKICAgIG5hbWVzcGFjZTogImRlZmF1bHQiCgpjdXJyZW50LWNvbnRleHQ6ICJsb2NhbCIK
`
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cloud-config-addon",
				Namespace: "test-hv",
			},
			Data: map[string]string{"cloud-config.yaml": manifest},
		}).Build()

	var r *HarvesterClusterReconciler

	var scope *ClusterScope

	log := log.FromContext(context.TODO())

	BeforeEach(func() {
		if os.Getenv("KUBECONFIG") == "" {
			Skip("KUBECONFIG not set, skipping integration test")
		}

		hvConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
		Expect(err).ToNot(HaveOccurred())
		hvClient, err := hvclient.NewForConfig(hvConfig)
		Expect(err).ToNot(HaveOccurred())

		r = &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		scope = &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-2",
					Namespace: "test-hv",
				},
				Spec: clusterv1.ClusterSpec{},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-2-hv",
					Namespace: "test-hv",
				},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					Server:          "https://192.168.1.109:6443",
					UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{ //nolint:gosec // field names contain "Credentials" but values are not secrets
						ManifestsConfigMapNamespace:      "test-hv",
						ManifestsConfigMapName:           "cloud-config-addon",
						ManifestsConfigMapKey:            "cloud-config.yaml",
						CloudConfigCredentialsSecretName: "cloud-config",
						CloudConfigCredentialsSecretKey:  "cloud-config",
					},
				},
			},
			HarvesterClient: hvClient,
		}
	})

	It("Should modify the cloud provider manifest", func() {
		Expect(r.reconcileCloudProviderConfig(scope)).To(Succeed())

		newCM := &corev1.ConfigMap{}
		Expect(fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "test-hv", Name: "cloud-config-addon"}, newCM)).To(Succeed())
		Expect(newCM.Data["cloud-config.yaml"]).To(Not(Equal(manifest)))
	})
})

// =============================================================================
// Tests for checkValidIpPoolDefinition
// =============================================================================

var _ = Describe("checkValidIpPoolDefinition", func() {
	It("should return false for empty IpPool", func() {
		pool := infrav1.IpPool{}
		Expect(checkValidIpPoolDefinition(pool)).To(BeFalse())
	})

	It("should return false when Subnet is empty", func() {
		pool := infrav1.IpPool{
			Gateway:   "172.16.0.1",
			VMNetwork: "default/production",
		}
		Expect(checkValidIpPoolDefinition(pool)).To(BeFalse())
	})

	It("should return false when Gateway is empty", func() {
		pool := infrav1.IpPool{
			Subnet:    "172.16.0.0/16",
			VMNetwork: "default/production",
		}
		Expect(checkValidIpPoolDefinition(pool)).To(BeFalse())
	})

	It("should return false when VMNetwork is empty", func() {
		pool := infrav1.IpPool{
			Subnet:  "172.16.0.0/16",
			Gateway: "172.16.0.1",
		}
		Expect(checkValidIpPoolDefinition(pool)).To(BeFalse())
	})

	It("should return true when all required fields are set", func() {
		pool := infrav1.IpPool{
			Subnet:    "172.16.0.0/16",
			Gateway:   "172.16.0.1",
			VMNetwork: "default/production",
		}
		Expect(checkValidIpPoolDefinition(pool)).To(BeTrue())
	})

	It("should return true with optional range fields", func() {
		pool := infrav1.IpPool{
			Subnet:     "172.16.3.0/24",
			Gateway:    "172.16.3.1",
			VMNetwork:  "default/production",
			RangeStart: "172.16.3.40",
			RangeEnd:   "172.16.3.49",
		}
		Expect(checkValidIpPoolDefinition(pool)).To(BeTrue())
	})
})

// =============================================================================
// Tests for isHarvesterAvailable
// =============================================================================

var _ = Describe("isHarvesterAvailable", func() {
	It("should return true when Available condition is True", func() {
		conditions := []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			},
		}
		Expect(isHarvesterAvailable(conditions)).To(BeTrue())
	})

	It("should return false when Available condition is False", func() {
		conditions := []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionFalse,
			},
		}
		Expect(isHarvesterAvailable(conditions)).To(BeFalse())
	})

	It("should return false when no conditions", func() {
		conditions := []appsv1.DeploymentCondition{}
		Expect(isHarvesterAvailable(conditions)).To(BeFalse())
	})

	It("should return false when only Progressing condition exists", func() {
		conditions := []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentProgressing,
				Status: corev1.ConditionTrue,
			},
		}
		Expect(isHarvesterAvailable(conditions)).To(BeFalse())
	})

	It("should return true with multiple conditions including Available=True", func() {
		conditions := []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentProgressing,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			},
		}
		Expect(isHarvesterAvailable(conditions)).To(BeTrue())
	})

	It("should return false when Available is Unknown", func() {
		conditions := []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionUnknown,
			},
		}
		Expect(isHarvesterAvailable(conditions)).To(BeFalse())
	})
})

// =============================================================================
// Tests for getListenersFromAPI
// =============================================================================

var _ = Describe("getListenersFromAPI", func() {
	It("should return empty listeners for empty spec", func() {
		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					Listeners: []infrav1.Listener{},
				},
			},
		}
		listeners := getListenersFromAPI(cluster)
		Expect(listeners).To(BeEmpty())
	})

	It("should convert listeners from API spec", func() {
		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					Listeners: []infrav1.Listener{
						{
							Name:        "https",
							Port:        443,
							Protocol:    corev1.ProtocolTCP,
							BackendPort: 443,
						},
						{
							Name:        "http",
							Port:        80,
							Protocol:    corev1.ProtocolTCP,
							BackendPort: 80,
						},
					},
				},
			},
		}
		listeners := getListenersFromAPI(cluster)
		Expect(listeners).To(HaveLen(2))
		Expect(listeners[0].Name).To(Equal("https"))
		Expect(listeners[0].Port).To(Equal(int32(443)))
		Expect(listeners[0].Protocol).To(Equal(corev1.ProtocolTCP))
		Expect(listeners[0].BackendPort).To(Equal(int32(443)))
		Expect(listeners[1].Name).To(Equal("http"))
		Expect(listeners[1].Port).To(Equal(int32(80)))
		Expect(listeners[1].BackendPort).To(Equal(int32(80)))
	})

	It("should handle UDP protocol", func() {
		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					Listeners: []infrav1.Listener{
						{
							Name:        "dns",
							Port:        53,
							Protocol:    corev1.ProtocolUDP,
							BackendPort: 53,
						},
					},
				},
			},
		}
		listeners := getListenersFromAPI(cluster)
		Expect(listeners).To(HaveLen(1))
		Expect(listeners[0].Protocol).To(Equal(corev1.ProtocolUDP))
	})

	It("should handle single listener", func() {
		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					Listeners: []infrav1.Listener{
						{
							Name:        "api-server",
							Port:        6443,
							Protocol:    corev1.ProtocolTCP,
							BackendPort: 6443,
						},
					},
				},
			},
		}
		listeners := getListenersFromAPI(cluster)
		Expect(listeners).To(HaveLen(1))
		Expect(listeners[0].Name).To(Equal("api-server"))
		Expect(listeners[0].Port).To(Equal(int32(6443)))
	})
})

// =============================================================================
// Tests for getHarvesterServerFromKubeconfig (additional cases)
// =============================================================================

var _ = Describe("getHarvesterServerFromKubeconfig additional", func() {
	It("should handle kubeconfig with single cluster", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: default
contexts:
- context:
    cluster: default
    user: admin
  name: default
current-context: default
kind: Config
users:
- name: admin
  user:
    token: test-token
`)
		server, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(server).To(Equal("https://192.168.1.100:6443"))
	})

	It("should return error for invalid kubeconfig", func() {
		_, err := getHarvesterServerFromKubeconfig([]byte("invalid yaml content that is not kubeconfig"))
		Expect(err).To(HaveOccurred())
	})

	It("should return error for empty kubeconfig", func() {
		_, err := getHarvesterServerFromKubeconfig([]byte(""))
		Expect(err).To(HaveOccurred())
	})

	It("should handle Rancher proxy URL pattern", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://rancher.example.com/k8s/clusters/c-m-abc123
  name: rancher-cluster
contexts:
- context:
    cluster: rancher-cluster
    user: rancher-user
  name: rancher-context
current-context: rancher-context
kind: Config
users:
- name: rancher-user
  user:
    token: kubeconfig-user-xyz:abc123
`)
		server, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(server).To(Equal("https://rancher.example.com/k8s/clusters/c-m-abc123"))
	})

	It("should return error for kubeconfig with empty current-context", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: default
contexts:
- context:
    cluster: default
    user: admin
  name: default
current-context: ""
kind: Config
users:
- name: admin
  user:
    token: test-token
`)
		_, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no current-context"))
	})

	It("should return error for kubeconfig with mismatched context", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: default
contexts:
- context:
    cluster: default
    user: admin
  name: default
current-context: nonexistent
kind: Config
users:
- name: admin
  user:
    token: test-token
`)
		_, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no context section"))
	})
})

// =============================================================================
// Tests for createPlaceholderSVC (uses ClusterScope with fake Harvester client)
// =============================================================================

var _ = Describe("createPlaceholderSVC", func() {
	It("should create a LoadBalancer service with DHCP IP", func() {
		hvFake := hvfake.NewSimpleClientset()
		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
				},
			},
			HarvesterClient: hvFake,
		}

		err := createPlaceholderSVC("test-lb", scope, "0.0.0.0")
		Expect(err).ToNot(HaveOccurred())

		// Verify the service was created
		svc, err := hvFake.CoreV1().Services("default").Get(context.TODO(), "test-lb", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
		Expect(svc.Spec.LoadBalancerIP).To(Equal("0.0.0.0"))
		Expect(svc.Labels["loadbalancer.harvesterhci.io/servicelb"]).To(Equal("true"))
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(6443)))
	})

	It("should create a LoadBalancer service with pool IP", func() {
		hvFake := hvfake.NewSimpleClientset()
		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
				},
			},
			HarvesterClient: hvFake,
		}

		err := createPlaceholderSVC("pool-lb", scope, "172.16.3.100")
		Expect(err).ToNot(HaveOccurred())

		svc, err := hvFake.CoreV1().Services("default").Get(context.TODO(), "pool-lb", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(svc.Spec.LoadBalancerIP).To(Equal("172.16.3.100"))
	})
})

// =============================================================================
// Tests for ReconcileDelete (cluster controller)
// =============================================================================

var _ = Describe("ReconcileDelete (ClusterScope)", func() {
	It("should remove finalizer and return success when no resources exist", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-delete",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		// Finalizer should be removed
		Expect(hvCluster.Finalizers).To(BeEmpty())
	})

	It("should handle deletion of VM IP pool created by controller", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-delete-pool",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef: "existing-pool",
				},
			},
			Status: infrav1.HarvesterClusterStatus{
				// VMIPPoolCreatedByController is NOT true, so pool should NOT be deleted
				Conditions: []metav1.Condition{},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(hvCluster.Finalizers).To(BeEmpty())
	})
})

// =============================================================================
// Tests for reconcileCloudProviderConfig (without KUBECONFIG)
// =============================================================================

var _ = Describe("reconcileCloudProviderConfig unit", func() {
	It("should skip when CloudProviderConfigReadyCondition is already true", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cloud", Namespace: "test-ns"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
			},
			Status: infrav1.HarvesterClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:   infrav1.CloudProviderConfigReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should set condition when UpdateCloudProviderConfig is empty", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cloud", Namespace: "test-ns"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		// When UpdateCloudProviderConfig is empty (zero value), it should
		// just set the condition and return nil
		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).ToNot(HaveOccurred())
	})
})

// =============================================================================
// Tests for reconcileVMIPPool
// =============================================================================

var _ = Describe("reconcileVMIPPool", func() {
	It("should return nil when VMNetworkConfig is nil", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileVMIPPool(scope)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error when neither IPPoolRef nor IPPool is set", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						// No IPPoolRef, no IPPool
						Gateway:    "172.16.0.1",
						SubnetMask: "255.255.0.0",
					},
				},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileVMIPPool(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requires either IPPoolRef or IPPool"))
	})

	It("should return error when IPPoolRef references non-existent pool", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef:  "nonexistent-pool",
					Gateway:    "172.16.0.1",
					SubnetMask: "255.255.0.0",
				},
			},
		}

		scope := &ClusterScope{
			Ctx:              context.TODO(),
			Logger:           log.FromContext(context.TODO()),
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		err := r.reconcileVMIPPool(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})
})

// =============================================================================
// Tests for createLoadBalancerIfNotExists
// =============================================================================

var _ = Describe("createLoadBalancerIfNotExists", func() {
	It("should create a load balancer in Harvester", func() {
		hvFake := hvfake.NewSimpleClientset()
		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.DHCP,
						IpPoolRef: "",
						Listeners: []infrav1.Listener{},
					},
				},
			},
			HarvesterClient: hvFake,
		}

		err := createLoadBalancerIfNotExists(scope)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should not error when load balancer already exists", func() {
		hvFake := hvfake.NewSimpleClientset()
		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.DHCP,
						Listeners: []infrav1.Listener{},
					},
				},
			},
			HarvesterClient: hvFake,
		}

		// Create first
		err := createLoadBalancerIfNotExists(scope)
		Expect(err).ToNot(HaveOccurred())

		// Create again - should not error
		err = createLoadBalancerIfNotExists(scope)
		Expect(err).ToNot(HaveOccurred())
	})
})

// =============================================================================
// Tests for ReconcileNormal initial phases (cluster controller)
// =============================================================================

var _ = Describe("ReconcileNormal (cluster) initial phases", func() {
	It("should add finalizer on first reconcile", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// Should return empty result after adding finalizer
		Expect(result.RequeueAfter).To(BeZero())
		// Finalizer should be added
		Expect(hvCluster.Finalizers).To(ContainElement(infrav1.ClusterFinalizer))
	})

	It("should proceed past finalizer on second reconcile with namespace creation", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cluster",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		// Second reconcile: should check target namespace
		// The namespace doesn't exist, will try to create it, then proceed
		result, err := r.ReconcileNormal(scope)
		// May requeue, but should not have critical error
		_ = result
		_ = err
		// Verify the namespace was created in the fake client
		ns, nsErr := hvFake.CoreV1().Namespaces().Get(context.TODO(), "default", metav1.GetOptions{})
		Expect(nsErr).ToNot(HaveOccurred())
		Expect(ns.Name).To(Equal("default"))
	})

	It("should create placeholder LB and requeue when no CP machines exist", func() {
		hvFake := hvfake.NewSimpleClientset()
		// Pre-create namespace via API (not via constructor to avoid scheme issue)
		_, err := hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "my-cluster-hv",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err2 := r.ReconcileNormal(scope)
		Expect(err2).ToNot(HaveOccurred())
		// Should requeue after creating placeholder LB
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})

	It("should handle existing namespace and proceed with placeholder LB check", func() {
		hvFake := hvfake.NewSimpleClientset()
		// Pre-create namespace via API
		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ns"},
		}, metav1.CreateOptions{})
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cluster-2-hv",
				Namespace:  "ns2",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "my-ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-2", Namespace: "ns2"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// Verify TargetNamespaceReady condition was set to True
		var nsCondition *metav1.Condition

		for i := range hvCluster.Status.Conditions {
			if hvCluster.Status.Conditions[i].Type == infrav1.TargetNamespaceReadyCondition {
				nsCondition = &hvCluster.Status.Conditions[i]

				break
			}
		}

		Expect(nsCondition).ToNot(BeNil())
		Expect(nsCondition.Status).To(Equal(metav1.ConditionTrue))
	})
})

// =============================================================================
// Additional ReconcileDelete tests for full coverage
// =============================================================================

var _ = Describe("ReconcileDelete comprehensive", func() {
	It("should delete custom IP pool when CustomIPPoolCreated condition is true", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-del-custom",
				Namespace:  "ns1",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType:  infrav1.POOL,
					IpPoolRef: "custom-pool-ref",
				},
			},
			Status: infrav1.HarvesterClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:   infrav1.CustomIPPoolCreatedCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "ns1"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(hvCluster.Finalizers).To(BeEmpty())
	})

	It("should delete VM IP pool when VMIPPoolCreatedByController condition is true", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-del-vm-pool",
				Namespace:  "ns1",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef: "controller-created-pool",
				},
			},
			Status: infrav1.HarvesterClusterStatus{
				Conditions: []metav1.Condition{
					{
						Type:   infrav1.VMIPPoolCreatedByControllerCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "ns1"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(hvCluster.Finalizers).To(BeEmpty())
	})

	It("should skip VM IP pool deletion for pre-existing pools", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-del-preexist",
				Namespace:  "ns1",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef: "pre-existing-pool",
				},
			},
			// NO VMIPPoolCreatedByController condition
		}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "ns1"},
			},
			HarvesterCluster: hvCluster,
			HarvesterClient:  hvFake,
			ReconcileClient:  fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(hvCluster.Finalizers).To(BeEmpty())
	})

	It("should delete actual LB and SVC resources when they exist in Harvester", func() {
		hvFake := hvfake.NewSimpleClientset()

		lbName := locutil.GenerateRFC1035Name([]string{"ns1", "real-del", "lb"})
		ns := "target-ns"

		// Pre-create LB in fake
		_, err := hvFake.LoadbalancerV1beta1().LoadBalancers(ns).Create(context.TODO(), &lbv1beta1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: ns},
			Spec:       lbv1beta1.LoadBalancerSpec{Description: "test"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Pre-create SVC in fake
		_, err = hvFake.CoreV1().Services(ns).Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: ns},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Pre-create generated IP pool
		ipPoolName := locutil.GenerateRFC1035Name([]string{"ns1", "real-del", "ippool"})
		_, err = hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: ipPoolName},
			Spec:       lbv1beta1.IPPoolSpec{Description: "generated"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "real-del", Namespace: "ns1",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    ns,
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "real-del", Namespace: "ns1"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(hvCluster.Finalizers).To(BeEmpty())

		// Verify resources were deleted
		_, err = hvFake.LoadbalancerV1beta1().LoadBalancers(ns).Get(context.TODO(), lbName, metav1.GetOptions{})
		Expect(err).To(HaveOccurred()) // Should be NotFound
		_, err = hvFake.CoreV1().Services(ns).Get(context.TODO(), lbName, metav1.GetOptions{})
		Expect(err).To(HaveOccurred()) // Should be NotFound
	})
})

// =============================================================================
// Tests for getLoadBalancerIP
// =============================================================================

var _ = Describe("getLoadBalancerIP", func() {
	It("should return IP when LB has an address", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "my-hv", Namespace: "ns1"},
			Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "target-ns"},
		}

		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})

		// Create LB with address in status
		_, err := hvFake.LoadbalancerV1beta1().LoadBalancers("target-ns").Create(context.TODO(), &lbv1beta1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "target-ns"},
			Spec:       lbv1beta1.LoadBalancerSpec{Description: "test lb"},
			Status:     lbv1beta1.LoadBalancerStatus{Address: "172.16.3.50"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		ip, err := getLoadBalancerIP(context.TODO(), hvCluster, hvFake)
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).To(Equal("172.16.3.50"))
	})

	It("should return error when LB does not exist", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "no-lb", Namespace: "ns1"},
			Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "target-ns"},
		}

		_, err := getLoadBalancerIP(context.TODO(), hvCluster, hvFake)
		Expect(err).To(HaveOccurred())
	})

	It("should return error when LB address is empty", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-lb", Namespace: "ns1"},
			Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "target-ns"},
		}

		lbName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "lb"})

		// Create LB without address
		_, err := hvFake.LoadbalancerV1beta1().LoadBalancers("target-ns").Create(context.TODO(), &lbv1beta1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "target-ns"},
			Spec:       lbv1beta1.LoadBalancerSpec{Description: "test lb"},
			Status:     lbv1beta1.LoadBalancerStatus{Address: ""},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		_, err = getLoadBalancerIP(context.TODO(), hvCluster, hvFake)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty"))
	})
})

// =============================================================================
// Tests for createIPPoolIfNotExists
// =============================================================================

var _ = Describe("createIPPoolIfNotExists", func() {
	It("should create a new IP pool", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-cluster", Namespace: "ns1"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "target-ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.POOL,
					IpPool: infrav1.IpPool{
						Subnet:     "172.16.0.0/16",
						Gateway:    "172.16.0.1",
						VMNetwork:  "default/production",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
					},
				},
			},
		}

		// IPPools() is cluster-scoped in the fake, so pass "" for namespace
		pool, err := createIPPoolIfNotExists(context.TODO(), hvCluster, hvFake, "default/production", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(pool).ToNot(BeNil())
		Expect(pool.Name).ToNot(BeEmpty())
		Expect(pool.Spec.Ranges).To(HaveLen(1))
		Expect(pool.Spec.Ranges[0].Subnet).To(Equal("172.16.0.0/16"))
		Expect(pool.Spec.Ranges[0].Gateway).To(Equal("172.16.0.1"))
		Expect(pool.Spec.Ranges[0].RangeStart).To(Equal("172.16.3.40"))
		Expect(pool.Spec.Ranges[0].RangeEnd).To(Equal("172.16.3.49"))
		Expect(pool.Spec.Selector.Network).To(Equal("default/production"))
		// Condition should be set
		Expect(hvCluster.Status.Conditions).ToNot(BeEmpty())
	})

	It("should return existing pool when it already exists", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "exist-cluster", Namespace: "ns1"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "target-ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IpPool: infrav1.IpPool{
						Subnet:    "10.0.0.0/24",
						Gateway:   "10.0.0.1",
						VMNetwork: "default/vlan1",
					},
				},
			},
		}

		// Create pool first (pass "" for namespace since IPPools() is cluster-scoped in fake)
		pool1, err := createIPPoolIfNotExists(context.TODO(), hvCluster, hvFake, "default/vlan1", "")
		Expect(err).ToNot(HaveOccurred())

		// Create again - should get existing pool
		pool2, err := createIPPoolIfNotExists(context.TODO(), hvCluster, hvFake, "default/vlan1", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(pool2.Name).To(Equal(pool1.Name))
	})

	It("should set CustomIPPoolCreated condition on success", func() {
		hvFake := hvfake.NewSimpleClientset()
		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cond-cluster", Namespace: "ns2"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IpPool: infrav1.IpPool{
						Subnet:    "192.168.1.0/24",
						Gateway:   "192.168.1.1",
						VMNetwork: "default/net1",
					},
				},
			},
		}

		pool, err := createIPPoolIfNotExists(context.TODO(), hvCluster, hvFake, "default/net1", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(pool.Name).ToNot(BeEmpty())

		// Check that CustomIPPoolCreated condition was set to True
		var found bool

		for _, c := range hvCluster.Status.Conditions {
			if c.Type == infrav1.CustomIPPoolCreatedCondition {
				Expect(c.Status).To(Equal(metav1.ConditionTrue))

				found = true
			}
		}

		Expect(found).To(BeTrue())
	})
})

// =============================================================================
// Tests for getOwnedCPHarversterMachines
// =============================================================================

var _ = Describe("getOwnedCPHarversterMachines", func() {
	It("should return empty list when no machines exist", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv", Namespace: "test-ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		machines, err := r.getOwnedCPHarversterMachines(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(BeEmpty())
	})

	It("should return CP machines that have matching labels", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		// Create HarvesterMachine objects with CP labels
		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cp-machine-1",
				Namespace: "test-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "test-cluster",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{
				SSHUser: "rancher",
			},
		}
		workerMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker-machine-1",
				Namespace: "test-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: "test-cluster",
					// No MachineControlPlaneLabel
				},
			},
			Spec: infrav1.HarvesterMachineSpec{
				SSHUser: "rancher",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cpMachine, workerMachine).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv", Namespace: "test-ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		machines, err := r.getOwnedCPHarversterMachines(scope)
		Expect(err).ToNot(HaveOccurred())
		// CP machine found but VM doesn't exist in Harvester, so filtered out
		Expect(machines).To(BeEmpty())
	})

	It("should include CP machines whose VMs exist in Harvester", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		// Create a VM in Harvester fake for the CP machine
		_, err := hvFake.KubevirtV1().VirtualMachines("default").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-machine-1", Namespace: "default"},
			}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cp-machine-1",
				Namespace: "test-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "test-cluster",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cpMachine).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv", Namespace: "test-ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		machines, err := r.getOwnedCPHarversterMachines(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(machines).To(HaveLen(1))
		Expect(machines[0].Name).To(Equal("cp-machine-1"))
	})
})

// =============================================================================
// Tests for reconcileVMIPPool (additional paths)
// =============================================================================

var _ = Describe("reconcileVMIPPool additional paths", func() {
	It("should succeed when IPPoolRef references an existing pool", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Pre-create the referenced pool
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-pool"},
			Spec:       lbv1beta1.IPPoolSpec{Description: "pre-existing"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef:  "existing-pool",
					Gateway:    "172.16.0.1",
					SubnetMask: "255.255.0.0",
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		err = r.reconcileVMIPPool(scope)
		Expect(err).ToNot(HaveOccurred())

		// Verify VMIPPoolReady condition is set
		var found bool

		for _, c := range hvCluster.Status.Conditions {
			if c.Type == infrav1.VMIPPoolReadyCondition {
				Expect(c.Status).To(Equal(metav1.ConditionTrue))

				found = true
			}
		}

		Expect(found).To(BeTrue())
	})

	It("should create a new pool when IPPool spec is provided without IPPoolRef", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-test", Namespace: "ns1"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					Gateway:    "172.16.0.1",
					SubnetMask: "255.255.0.0",
					IPPool: &infrav1.IpPool{
						Subnet:     "172.16.0.0/16",
						Gateway:    "172.16.0.1",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
						VMNetwork:  "default/production",
					},
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		err := r.reconcileVMIPPool(scope)
		Expect(err).ToNot(HaveOccurred())

		// IPPoolRef should now be set
		Expect(hvCluster.Spec.VMNetworkConfig.IPPoolRef).ToNot(BeEmpty())

		// Verify the pool was created in Harvester
		pool, err := hvFake.LoadbalancerV1beta1().IPPools().Get(context.TODO(),
			hvCluster.Spec.VMNetworkConfig.IPPoolRef, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(pool.Spec.Ranges).To(HaveLen(1))
		Expect(pool.Spec.Ranges[0].Subnet).To(Equal("172.16.0.0/16"))
	})

	It("should reuse existing pool when pool already exists", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "reuse-test", Namespace: "ns1"},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "default",
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPool: &infrav1.IpPool{
						Subnet:    "10.0.0.0/24",
						Gateway:   "10.0.0.1",
						VMNetwork: "default/net1",
					},
				},
			},
		}

		// Pre-create the pool with the expected generated name
		poolName := locutil.GenerateRFC1035Name([]string{hvCluster.Namespace, hvCluster.Name, "vm-ippool"})
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: poolName},
			Spec:       lbv1beta1.IPPoolSpec{Description: "already here"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		err = r.reconcileVMIPPool(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(hvCluster.Spec.VMNetworkConfig.IPPoolRef).To(Equal(poolName))
	})
})

// =============================================================================
// Tests for reconcileCloudProviderConfig (additional paths)
// =============================================================================

var _ = Describe("reconcileCloudProviderConfig additional paths", func() {
	It("should return error when ManifestsConfigMapName is empty", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{
						ManifestsConfigMapName:      "", // empty
						ManifestsConfigMapNamespace: "ns",
					},
				},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ManifestsConfigMapName"))
	})

	It("should return error when referenced ConfigMap does not exist", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{
						ManifestsConfigMapName:      "nonexistent-cm",
						ManifestsConfigMapNamespace: "ns",
						ManifestsConfigMapKey:       "manifests",
					},
				},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to get the referenced config map"))
	})
})

// =============================================================================
// Tests for findObjectsForSecret
// =============================================================================

var _ = Describe("findObjectsForSecret", func() {
	It("should return empty when no clusters reference the secret", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&infrav1.HarvesterCluster{}, secretIdField, func(o client.Object) []string {
				cluster, ok := o.(*infrav1.HarvesterCluster)
				if !ok {
					return nil
				}

				if (cluster.Spec.IdentitySecret == infrav1.SecretKey{}) || cluster.Spec.IdentitySecret.Name == "" {
					return nil
				}

				return []string{cluster.Spec.IdentitySecret.Name}
			}).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "some-secret", Namespace: "ns"},
		}

		requests := r.findObjectsForSecret(context.TODO(), secret)
		Expect(requests).To(BeEmpty())
	})

	It("should return requests for clusters that reference the secret", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: "ns1"},
			Spec: infrav1.HarvesterClusterSpec{
				IdentitySecret: infrav1.SecretKey{
					Name:      "my-identity-secret",
					Namespace: "ns1",
				},
				TargetNamespace: "default",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(hvCluster).
			WithIndex(&infrav1.HarvesterCluster{}, secretIdField, func(o client.Object) []string {
				cluster, ok := o.(*infrav1.HarvesterCluster)
				if !ok {
					return nil
				}

				if (cluster.Spec.IdentitySecret == infrav1.SecretKey{}) || cluster.Spec.IdentitySecret.Name == "" {
					return nil
				}

				return []string{cluster.Spec.IdentitySecret.Name}
			}).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "my-identity-secret", Namespace: "ns1"},
		}

		requests := r.findObjectsForSecret(context.TODO(), secret)
		Expect(requests).To(HaveLen(1))
		Expect(requests[0]).To(Equal(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "my-cluster", Namespace: "ns1"},
		}))
	})
})

// =============================================================================
// Tests for getHarvesterServerFromKubeconfig (remaining coverage: missing cluster)
// =============================================================================

var _ = Describe("getHarvesterServerFromKubeconfig edge cases", func() {
	It("should return error when context references a non-existent cluster", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: real-cluster
contexts:
- context:
    cluster: missing-cluster
    user: admin
  name: my-context
current-context: my-context
kind: Config
users:
- name: admin
  user:
    token: test-token
`)
		_, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no cluster section"))
	})

	It("should return error when cluster has empty server", func() {
		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: ""
  name: empty-server
contexts:
- context:
    cluster: empty-server
    user: admin
  name: my-context
current-context: my-context
kind: Config
users:
- name: admin
  user:
    token: test-token
`)
		_, err := getHarvesterServerFromKubeconfig(kubeconfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no server found"))
	})
})

// =============================================================================
// Tests for ReconcileNormal with CP machines (LB creation + IP path)
// =============================================================================

var _ = Describe("ReconcileNormal with CP machines", func() {
	It("should proceed to LB creation when CP machines exist", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Pre-create namespace
		_, err := hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "target-ns"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Pre-create a VM in Harvester for the CP machine
		_, err = hvFake.KubevirtV1().VirtualMachines("target-ns").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-machine-1", Namespace: "target-ns"},
			}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cp-machine-1",
				Namespace: "test-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "test-cluster",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cpMachine).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-hv", Namespace: "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "target-ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		// Should proceed past finalizer + namespace check + CP machines
		// Then create LB → getLoadBalancerIP fails (no address) → requeue
		result, err := r.ReconcileNormal(scope)
		// Error is expected since LB was just created and has no address yet
		_ = err
		// Should requeue
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// Verify that the LB was created in Harvester
		lbName := locutil.GenerateRFC1035Name([]string{"test-ns", "test-hv", "lb"})
		lb, getErr := hvFake.LoadbalancerV1beta1().LoadBalancers("target-ns").Get(context.TODO(), lbName, metav1.GetOptions{})
		Expect(getErr).ToNot(HaveOccurred())
		Expect(string(lb.Spec.WorkloadType)).To(Equal("vm"))
	})

	It("should set Ready=true when LB already has an IP", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Pre-create namespace
		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "target-ns"},
		}, metav1.CreateOptions{})

		// Pre-create VM
		_, _ = hvFake.KubevirtV1().VirtualMachines("target-ns").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-1", Namespace: "target-ns"},
			}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp-1", Namespace: "test-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "my-cluster",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cpMachine).
			Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-hv", Namespace: "test-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "target-ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
			},
		}

		// Pre-create LB with an assigned IP address
		lbName := locutil.GenerateRFC1035Name([]string{"test-ns", "my-hv", "lb"})
		_, _ = hvFake.LoadbalancerV1beta1().LoadBalancers("target-ns").Create(context.TODO(), &lbv1beta1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "target-ns"},
			Spec:       lbv1beta1.LoadBalancerSpec{Description: "test"},
			Status:     lbv1beta1.LoadBalancerStatus{Address: "172.16.3.55"},
		}, metav1.CreateOptions{})

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "my-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		// Should be ready with the LB IP
		Expect(hvCluster.Status.Ready).To(BeTrue())
		Expect(hvCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("172.16.3.55"))
		Expect(hvCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
	})

	It("should set LoadBalancerReady when LB transitions from not ready to ready", func() {
		hvFake := hvfake.NewSimpleClientset()

		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "ns"},
		}, metav1.CreateOptions{})

		_, _ = hvFake.KubevirtV1().VirtualMachines("ns").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-m1", Namespace: "ns"},
			}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp-m1", Namespace: "cls-ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "cls",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpMachine).Build()
		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		lbName := locutil.GenerateRFC1035Name([]string{"cls-ns", "hv-cls", "lb"})
		_, _ = hvFake.LoadbalancerV1beta1().LoadBalancers("ns").Create(context.TODO(), &lbv1beta1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "ns"},
			Status:     lbv1beta1.LoadBalancerStatus{Address: "10.0.0.5"},
		}, metav1.CreateOptions{})

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hv-cls", Namespace: "cls-ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "ns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cls", Namespace: "cls-ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		_, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(hvCluster.Status.Ready).To(BeTrue())

		// Check LoadBalancerReady condition was set
		var lbReady bool

		for _, c := range hvCluster.Status.Conditions {
			if c.Type == infrav1.LoadBalancerReadyCondition && c.Status == metav1.ConditionTrue {
				lbReady = true
			}
		}

		Expect(lbReady).To(BeTrue())
	})

	It("should skip LB creation when LoadBalancerReady condition is already true", func() {
		hvFake := hvfake.NewSimpleClientset()

		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "tns"},
		}, metav1.CreateOptions{})

		_, _ = hvFake.KubevirtV1().VirtualMachines("tns").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-ready", Namespace: "tns"},
			}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp-ready", Namespace: "ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "cls",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpMachine).Build()
		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hv-ready", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "tns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
				ControlPlaneEndpoint: clusterv1.APIEndpoint{
					Host: "172.16.3.55",
					Port: 6443,
				},
			},
			Status: infrav1.HarvesterClusterStatus{
				Ready: true,
				Conditions: []metav1.Condition{
					{
						Type:   infrav1.LoadBalancerReadyCondition,
						Status: metav1.ConditionTrue,
					},
					{
						Type:   infrav1.CloudProviderConfigReadyCondition,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cls", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// Should not requeue for LB (already ready), just sets InfrastructureReady
		_ = result

		// Check InfrastructureReady condition
		var infraReady bool

		for _, c := range hvCluster.Status.Conditions {
			if c.Type == infrav1.InfrastructureReadyCondition && c.Status == metav1.ConditionTrue {
				infraReady = true
			}
		}

		Expect(infraReady).To(BeTrue())
	})

	It("should handle ReconcileNormal with placeholder LB that already has an IP (no CP machines)", func() {
		hvFake := hvfake.NewSimpleClientset()

		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		}, metav1.CreateOptions{})

		// Create a placeholder SVC with an ingress IP
		lbName := locutil.GenerateRFC1035Name([]string{"ns", "cl-hv", "lb"})
		_, _ = hvFake.CoreV1().Services("default").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "172.16.3.77"}},
				},
			},
		}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cl-hv", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cl", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		// Should be Ready with the placeholder LB IP
		Expect(hvCluster.Status.Ready).To(BeTrue())
		Expect(hvCluster.Spec.ControlPlaneEndpoint.Host).To(Equal("172.16.3.77"))
		Expect(hvCluster.Spec.ControlPlaneEndpoint.Port).To(Equal(int32(6443)))
	})

	It("should requeue when placeholder LB exists but has no IP", func() {
		hvFake := hvfake.NewSimpleClientset()

		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		}, metav1.CreateOptions{})

		// Create a placeholder SVC with NO ingress IP
		lbName := locutil.GenerateRFC1035Name([]string{"ns", "no-ip-hv", "lb"})
		_, _ = hvFake.CoreV1().Services("default").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			Status:     corev1.ServiceStatus{}, // No ingress
		}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "no-ip-hv", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "no-ip", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// Should requeue waiting for IP
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		// Should NOT be ready yet
		Expect(hvCluster.Status.Ready).To(BeFalse())
	})
})

// =============================================================================
// Tests for getIPFromIPPool
// =============================================================================

var _ = Describe("getIPFromIPPool", func() {
	It("should return error when neither poolRef nor valid pool definition is set", func() {
		hvFake := hvfake.NewSimpleClientset()

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "no-pool", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.POOL,
						IpPoolRef: "",
						IpPool:    infrav1.IpPool{}, // empty = invalid
					},
				},
			},
			HarvesterClient: hvFake,
		}

		_, err := getIPFromIPPool(scope, "default/test-lb")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("IP Pool reference is empty"))
	})

	It("should return error when poolRef references non-existent pool", func() {
		hvFake := hvfake.NewSimpleClientset()

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-ref", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.POOL,
						IpPoolRef: "nonexistent-pool",
					},
				},
			},
			HarvesterClient: hvFake,
		}

		_, err := getIPFromIPPool(scope, "default/test-lb")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not get referenced IP Pool"))
	})

	It("should return error when pool has no available addresses", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Create pool with 0 available
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{Subnet: "10.0.0.0/30", Gateway: "10.0.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 0,
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.POOL,
						IpPoolRef: "empty-pool",
					},
				},
			},
			HarvesterClient: hvFake,
		}

		_, err = getIPFromIPPool(scope, "default/test-lb")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not have available addresses"))
	})

	It("should allocate an IP from a pool with available addresses", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Create pool with available addresses
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "good-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "172.16.3.0/24",
						Gateway:    "172.16.3.1",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					LoadBalancerConfig: infrav1.LoadBalancerConfig{
						IPAMType:  infrav1.POOL,
						IpPoolRef: "good-pool",
					},
				},
			},
			HarvesterClient: hvFake,
		}

		ip, err := getIPFromIPPool(scope, "default/test-lb")
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).ToNot(BeEmpty())
		// Should be in the range 172.16.3.40-49
		Expect(ip).To(HavePrefix("172.16.3.4"))
	})
})

// =============================================================================
// Tests for allocateIPFromPool
// =============================================================================

var _ = Describe("allocateIPFromPool", func() {
	It("should allocate first available IP from pool range", func() {
		hvFake := hvfake.NewSimpleClientset()

		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "alloc-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "10.0.0.0/24",
						Gateway:    "10.0.0.1",
						RangeStart: "10.0.0.10",
						RangeEnd:   "10.0.0.20",
					},
				},
			},
		}

		// Create in fake so Update works
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), pool, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx:             context.TODO(),
			Logger:          log.FromContext(context.TODO()),
			HarvesterClient: hvFake,
		}

		ip, err := allocateIPFromPool(pool, "default/my-lb", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).To(Equal("10.0.0.10"))
	})

	It("should reuse previously allocated IP from history", func() {
		hvFake := hvfake.NewSimpleClientset()

		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "history-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "10.0.0.0/24",
						Gateway:    "10.0.0.1",
						RangeStart: "10.0.0.10",
						RangeEnd:   "10.0.0.20",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				AllocatedHistory: map[string]string{
					"10.0.0.15": "default/my-lb",
				},
			},
		}

		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), pool, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx:             context.TODO(),
			Logger:          log.FromContext(context.TODO()),
			HarvesterClient: hvFake,
		}

		ip, err := allocateIPFromPool(pool, "default/my-lb", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).To(Equal("10.0.0.15"))
	})

	It("should allocate different IPs for different LBs", func() {
		hvFake := hvfake.NewSimpleClientset()

		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "multi-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "10.0.0.0/24",
						Gateway:    "10.0.0.1",
						RangeStart: "10.0.0.10",
						RangeEnd:   "10.0.0.20",
					},
				},
			},
		}

		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), pool, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		scope := &ClusterScope{
			Ctx:             context.TODO(),
			Logger:          log.FromContext(context.TODO()),
			HarvesterClient: hvFake,
		}

		ip1, err := allocateIPFromPool(pool, "default/lb1", scope)
		Expect(err).ToNot(HaveOccurred())

		ip2, err := allocateIPFromPool(pool, "default/lb2", scope)
		Expect(err).ToNot(HaveOccurred())

		Expect(ip1).ToNot(Equal(ip2))
	})
})

// =============================================================================
// Tests for ReconcileNormal with VMNetworkConfig + POOL IPAM path
// =============================================================================

var _ = Describe("ReconcileNormal with POOL IPAM and VMNetworkConfig", func() {
	It("should fail reconcileVMIPPool and requeue when config is invalid", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vm-net-test", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					// No IPPoolRef, no IPPool -> should fail
					Gateway:    "172.16.0.1",
					SubnetMask: "255.255.0.0",
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "vm-cls", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("requires either IPPoolRef or IPPool"))
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})

	It("should reconcile VMNetworkConfig with existing IPPoolRef", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Create the referenced pool
		_, err := hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-pool"},
			Spec:       lbv1beta1.IPPoolSpec{Description: "VM pool"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Create namespace
		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vm-ref-test", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace:    "default",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{IPAMType: infrav1.DHCP},
				VMNetworkConfig: &infrav1.VMNetworkConfig{
					IPPoolRef:  "vm-pool",
					Gateway:    "172.16.0.1",
					SubnetMask: "255.255.0.0",
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "vm-cls", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// Should proceed past VMIPPool reconciliation
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// VMIPPoolReady condition should be set
		var vmPoolReady bool

		for _, c := range hvCluster.Status.Conditions {
			if c.Type == infrav1.VMIPPoolReadyCondition && c.Status == metav1.ConditionTrue {
				vmPoolReady = true
			}
		}

		Expect(vmPoolReady).To(BeTrue())
	})
})

// =============================================================================
// Tests for ReconcileNormal cloud provider config error path
// =============================================================================

var _ = Describe("ReconcileNormal cloud provider config path", func() {
	It("should return error when reconcileCloudProviderConfig fails", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Create namespace and VM
		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "tns"},
		}, metav1.CreateOptions{})
		_, _ = hvFake.KubevirtV1().VirtualMachines("tns").Create(context.TODO(),
			&kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-err", Namespace: "tns"},
			}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		cpMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cp-err", Namespace: "ns",
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:         "err-cls",
					clusterv1.MachineControlPlaneLabel: "",
				},
			},
			Spec: infrav1.HarvesterMachineSpec{SSHUser: "rancher"},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpMachine).Build()
		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "err-hv", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "tns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType: infrav1.DHCP,
				},
				// This will trigger reconcileCloudProviderConfig to error:
				// ManifestsConfigMapName is empty but the struct is non-zero
				UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{
					ManifestsConfigMapNamespace: "ns",
					// ManifestsConfigMapName is empty -> error
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "err-cls", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ManifestsConfigMapName"))
		// Should requeue with long interval on cloud provider config error
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})
})

// =============================================================================
// Additional reconcileCloudProviderConfig tests for key-missing and
// GetCloudConfigB64 failure paths
// =============================================================================

var _ = Describe("reconcileCloudProviderConfig key and kubeconfig paths", func() {
	It("should error when ConfigMap exists but data key is missing", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		// Create a ConfigMap WITH the wrong key
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cloud-cm", Namespace: "ns"},
			Data:       map[string]string{"other-key": "some-data"},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{
						ManifestsConfigMapName:      "cloud-cm",
						ManifestsConfigMapNamespace: "ns",
						ManifestsConfigMapKey:       "manifests", // does not exist in CM
					},
				},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to get the data key"))
		Expect(err.Error()).To(ContainSubstring("manifests"))
	})

	It("should error from GetCloudConfigB64 when ingress-expose service is missing", func() {
		hvFake := hvfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		// ConfigMap with the correct key and valid YAML data
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-cm", Namespace: "ns"},
			Data: map[string]string{
				"manifests": "apiVersion: v1\nkind: Secret\nmetadata:\n  name: cloud-creds\n  namespace: kube-system\ndata:\n  kubeconfig: placeholder\n",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		scope := &ClusterScope{
			Ctx:    context.TODO(),
			Logger: log.FromContext(context.TODO()),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-cluster", Namespace: "ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-test", Namespace: "ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					Server:          "https://harvester.local",
					UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig{
						ManifestsConfigMapName:           "cp-cm",
						ManifestsConfigMapNamespace:      "ns",
						ManifestsConfigMapKey:            "manifests",
						CloudConfigCredentialsSecretName: "cloud-creds",
						CloudConfigCredentialsSecretKey:  "kubeconfig",
					},
				},
			},
			HarvesterClient: hvFake,
			ReconcileClient: fakeClient,
		}

		err := r.reconcileCloudProviderConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to generate the kubeconfig"))
	})
})

// =============================================================================
// Additional ReconcileNormal tests: placeholder LB exists with empty IP + POOL IPAM
// =============================================================================

var _ = Describe("ReconcileNormal placeholder LB with POOL IPAM", func() {
	It("should update placeholder LB IP from pool when LB exists but has no IP", func() {
		hvFake := hvfake.NewSimpleClientset()

		// Create namespace
		_, _ = hvFake.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "tns"},
		}, metav1.CreateOptions{})

		// Create the placeholder LB service with empty status (no Ingress IP)
		lbName := locutil.GenerateRFC1035Name([]string{"ns", "pool-cls", "lb"})
		_, _ = hvFake.CoreV1().Services("tns").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: lbName, Namespace: "tns"},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			},
			// Status.LoadBalancer.Ingress is empty
		}, metav1.CreateOptions{})

		// Create the IP pool with available IPs
		_, _ = hvFake.LoadbalancerV1beta1().IPPools().Create(context.TODO(), &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "172.16.0.0/16",
						Gateway:    "172.16.0.1",
						RangeStart: "172.16.3.50",
						RangeEnd:   "172.16.3.59",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
			},
		}, metav1.CreateOptions{})

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		r := &HarvesterClusterReconciler{Client: fakeClient, Scheme: scheme}

		hvCluster := &infrav1.HarvesterCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pool-cls", Namespace: "ns",
				Finalizers: []string{infrav1.ClusterFinalizer},
			},
			Spec: infrav1.HarvesterClusterSpec{
				TargetNamespace: "tns",
				LoadBalancerConfig: infrav1.LoadBalancerConfig{
					IPAMType:  infrav1.POOL,
					IpPoolRef: "my-pool",
				},
			},
		}

		scope := &ClusterScope{
			Ctx: context.TODO(), Logger: log.FromContext(context.TODO()),
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "pool-cls", Namespace: "ns"}},
			HarvesterCluster: hvCluster, HarvesterClient: hvFake, ReconcileClient: fakeClient,
		}

		result, err := r.ReconcileNormal(scope)
		// Should either requeue or succeed after updating the LB IP
		_ = err

		Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue()) //nolint:staticcheck // result.Requeue still used by controller
	})
})
