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

// Package util provides utility functions for the project.
package util

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

var yamlString = `apiVersion: v1
kind: Secret
metadata:
  name: test-secret
  namespace: default
  type: Opaque
data:
  username: aGVsbG8K
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: default
data:
  key1: value1
  key2: value2
`

const lbIPAddress = "10.0.0.1"

var _ = Describe("GetKubeconfigFromClusterAndCheck", func() {
	var (
		hvKubeconfigB64        string
		resultingKubeconfigB64 string
		err                    error
		saName                 string
		harvesterServerURL     string
		kubeconfigBytes        []byte
		hvRESTConfig           *rest.Config
		hvClient               *versioned.Clientset
	)

	BeforeEach(func() {
		namespace = "default"
		saName = "test-1"
		hvKubeconfigB64 = os.Getenv("HV_KUBECONFIG_B64")
		harvesterServerURL = os.Getenv("HV_SERVER_URL")

		if hvKubeconfigB64 == "" {
			Skip("HV_KUBECONFIG_B64 not set, skipping integration test")
		}
	})

	It("Should return the right name", func() {
		// Build a clientset from the kubeconfig
		kubeconfigBytes, err = base64.StdEncoding.DecodeString(hvKubeconfigB64)
		Expect(err).ToNot(HaveOccurred())

		hvRESTConfig, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
		Expect(err).ToNot(HaveOccurred())

		hvClient, err = versioned.NewForConfig(hvRESTConfig)
		Expect(err).ToNot(HaveOccurred())

		// Use the GetCloudConfigB64 function and get the resulting cloud-config B64 encoded string
		resultingKubeconfigB64, err = GetCloudConfigB64(context.TODO(), hvClient, saName, namespace, harvesterServerURL)
		Expect(err).ToNot(HaveOccurred())

		// Decode the resulting cloud-config B64 encoded string and validate it
		err = ValidateB64Kubeconfig(resultingKubeconfigB64)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("GetConfigMapsFromYAML", func() {
	var secrets []*corev1.Secret

	indexes := make([]int, 0, maxNumberOfSecrets)

	It("Should return the right name", func() {
		objectsFromYAML, err := GetSerializedObjects(yamlString)
		Expect(err).ToNot(HaveOccurred())

		secrets, indexes, err = GetSecrets(objectsFromYAML)
		Expect(err).ToNot(HaveOccurred())

		Expect(secrets).To(HaveLen(1))
		Expect(secrets[0].Name).To(Equal("test-secret"))
		Expect(secrets[0].Namespace).To(Equal("default"))
		Expect(secrets[0].Data).To(HaveLen(1))
		Expect(string(secrets[0].Data["username"])).To(Equal("hello\n"))
		Expect(indexes).To(HaveLen(1))
		Expect(indexes[0]).To(Equal(0))
	})
})

// Tests a change in a ConfigMap in YAML.
var _ = Describe("ChangeValueInConfigMapInYAML", func() {
	var (
		secretName      string
		secretNamespace string
		key             string
		value           []byte
	)

	BeforeEach(func() {
		secretName = "test-secret"
		secretNamespace = "default"
		key = "username"
		value = []byte("new-value")
	})

	It("Should return the right name", func() {
		// Get the modified YAML string
		modifiedYAMLString, err := ModifyYAMlString(yamlString, secretName, secretNamespace, key, value)
		Expect(err).ToNot(HaveOccurred())
		Expect(modifiedYAMLString).To(ContainSubstring("bmV3LXZhbHVl"))
		documents := strings.Split(modifiedYAMLString, "---")
		Expect(documents).To(HaveLen(2))

		var secret *corev1.Secret

		for _, document := range documents {
			if strings.Contains(document, "Secret") {
				err = yaml.Unmarshal([]byte(document), &secret)
				Expect(err).ToNot(HaveOccurred())
				Expect(secret.Data["username"]).To(Equal([]byte("new-value")))
			}
		}
	})
})

var _ = Describe("AddSecretToConfigMap", func() {
	It("Should return a YAML with an additional Secret", func() {
		// Get the modified YAML string with the new secret
		modifiedYAMLString, err := ModifyYAMlString(yamlString, "cloud-config", "test-hv", "cloud-config", []byte("new-value"))
		Expect(err).ToNot(HaveOccurred())
		Expect(modifiedYAMLString).To(Equal(`apiVersion: v1
data:
  username: aGVsbG8K
kind: Secret
metadata:
  name: test-secret
  namespace: default
  type: Opaque

---
apiVersion: v1
data:
  key1: value1
  key2: value2
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: default

---
apiVersion: v1
data:
  cloud-config: bmV3LXZhbHVl
kind: Secret
metadata:
  name: cloud-config
  namespace: test-hv
type: Opaque
`))
	})
})

type TestStruct struct {
	Spec map[string][]byte `yaml:"spec"`
}

var _ = Describe("buildKubeconfigFromSecret", func() {
	It("should build a valid kubeconfig from secret data", func() {
		secret := &corev1.Secret{
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("test-token"),
				corev1.ServiceAccountRootCAKey: []byte("test-ca-data"),
			},
		}
		result, err := buildKubeconfigFromSecret(secret, "test-ns", "https://example.com:6443")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring("server: https://example.com:6443"))
		Expect(result).To(ContainSubstring("test-token"))
	})

	It("should return error when token is missing", func() {
		secret := &corev1.Secret{
			Data: map[string][]byte{
				corev1.ServiceAccountRootCAKey: []byte("test-ca-data"),
			},
		}
		_, err := buildKubeconfigFromSecret(secret, "test-ns", "https://example.com:6443")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("token not found"))
	})

	It("should return error when ca.crt is missing", func() {
		secret := &corev1.Secret{
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey: []byte("test-token"),
			},
		}
		_, err := buildKubeconfigFromSecret(secret, "test-ns", "https://example.com:6443")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ca.crt not found"))
	})

	It("should return error when secret data is nil", func() {
		secret := &corev1.Secret{
			Data: nil,
		}
		_, err := buildKubeconfigFromSecret(secret, "test-ns", "https://example.com:6443")
		Expect(err).To(HaveOccurred())
	})

	It("should produce a kubeconfig with the correct namespace", func() {
		secret := &corev1.Secret{
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("my-token"),
				corev1.ServiceAccountRootCAKey: []byte("my-ca"),
			},
		}
		result, err := buildKubeconfigFromSecret(secret, "production", "https://harvester.local:6443")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring("production"))
		Expect(result).To(ContainSubstring("harvester.local"))
	})
})

var _ = Describe("GetDataKeyFromConfigMap in cloud_provider", func() {
	It("should return the value for an existing key", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
			Data:       map[string]string{"mykey": "myvalue"},
		}
		val, err := GetDataKeyFromConfigMap(cm, "mykey")
		Expect(err).ToNot(HaveOccurred())
		Expect(val).To(Equal("myvalue"))
	})

	It("should return error for missing key", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
			Data:       map[string]string{"other": "value"},
		}
		_, err := GetDataKeyFromConfigMap(cm, "missing")
		Expect(err).To(HaveOccurred())
	})

	It("should return error when data is empty", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-cm"},
			Data:       map[string]string{},
		}
		_, err := GetDataKeyFromConfigMap(cm, "key")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("key"))
	})
})

var _ = Describe("createServiceAccountIfNotExists", func() {
	It("should create a service account when it does not exist", func() {
		hvClient := hvfake.NewSimpleClientset()
		err := createServiceAccountIfNotExists(context.TODO(), hvClient, "test-sa", "default")
		Expect(err).ToNot(HaveOccurred())

		// Verify the SA was created
		sa, err := hvClient.CoreV1().ServiceAccounts("default").Get(context.TODO(), "test-sa", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(sa.Name).To(Equal("test-sa"))
	})

	It("should not error if the service account already exists", func() {
		hvClient := hvfake.NewSimpleClientset()
		// Pre-create the SA via the API so it's in the tracker
		_, err := hvClient.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-sa", Namespace: "default"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		err = createServiceAccountIfNotExists(context.TODO(), hvClient, "existing-sa", "default")
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("createServiceAccountIfNotExists error handling", func() {
	It("should propagate non-NotFound errors from Get", func() {
		hvClient := hvfake.NewSimpleClientset()
		// Add a reactor that makes Get for ServiceAccounts return an internal error
		hvClient.PrependReactor("get", "serviceaccounts", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("injected API error")
		})
		err := createServiceAccountIfNotExists(context.TODO(), hvClient, "test-sa", "default")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("injected API error"))
	})
})

var _ = Describe("createClusterRoleBindingIfNotExists error handling", func() {
	It("should propagate non-NotFound errors from Get", func() {
		hvClient := hvfake.NewSimpleClientset()
		hvClient.PrependReactor("get", "clusterrolebindings", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("injected CRB error")
		})
		err := createClusterRoleBindingIfNotExists(context.TODO(), hvClient, "test-crb", "default")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("injected CRB error"))
	})
})

var _ = Describe("createClusterRoleBindingIfNotExists", func() {
	It("should create a cluster role binding when it does not exist", func() {
		hvClient := hvfake.NewSimpleClientset()
		err := createClusterRoleBindingIfNotExists(context.TODO(), hvClient, "test-crb", "default")
		Expect(err).ToNot(HaveOccurred())

		// Verify the CRB was created
		crb, err := hvClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), "test-crb", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(crb.Name).To(Equal("test-crb"))
		Expect(crb.Subjects).To(HaveLen(1))
		Expect(crb.Subjects[0].Name).To(Equal("test-crb"))
		Expect(crb.Subjects[0].Namespace).To(Equal("default"))
		Expect(crb.RoleRef.Name).To(Equal(cloudProviderRoleName))
	})

	It("should not error if the cluster role binding already exists", func() {
		hvClient := hvfake.NewSimpleClientset()
		// Pre-create the CRB via the API so it's in the tracker
		_, err := hvClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-crb"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "existing-crb", Namespace: "default"}},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: cloudProviderRoleName},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		err = createClusterRoleBindingIfNotExists(context.TODO(), hvClient, "existing-crb", "default")
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("getKubeConfig", func() {
	It("should return error when service account does not exist", func() {
		hvClient := hvfake.NewSimpleClientset()
		_, err := getKubeConfig(context.TODO(), hvClient, "nonexistent", "default", "https://harvester.local:6443")
		Expect(err).To(HaveOccurred())
	})

	It("should build a kubeconfig when all resources exist", func() {
		hvClient := hvfake.NewSimpleClientset()

		// Create the service account
		_, err := hvClient.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa", Namespace: "default"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Pre-create the secret with token and ca.crt data
		// getKubeConfig will try to create "test-sa-token" and get AlreadyExists, then Get it
		_, err = hvClient.CoreV1().Secrets("default").Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sa-token",
				Namespace: "default",
			},
			Type: corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("fake-token-data"),
				corev1.ServiceAccountRootCAKey: []byte("fake-ca-data"),
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Create the ingress-expose service in kube-system with VIP annotation
		_, err = hvClient.CoreV1().Services("kube-system").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress-expose",
				Namespace: "kube-system",
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: lbIPAddress,
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		result, err := getKubeConfig(context.TODO(), hvClient, "test-sa", "default", "https://harvester.local:6443")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeEmpty())

		// Verify the result is valid base64
		err = ValidateB64Kubeconfig(result)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error when ingress-expose service does not exist", func() {
		hvClient := hvfake.NewSimpleClientset()

		// Create the service account
		_, err := hvClient.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa2", Namespace: "default"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Create the secret
		_, err = hvClient.CoreV1().Secrets("default").Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa2-token", Namespace: "default"},
			Type:       corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("token"),
				corev1.ServiceAccountRootCAKey: []byte("ca"),
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// No ingress-expose service - should fail
		_, err = getKubeConfig(context.TODO(), hvClient, "test-sa2", "default", "https://harvester.local:6443")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ingress-expose"))
	})

	It("should use provided server URL when VIP annotation has no valid IP", func() {
		hvClient := hvfake.NewSimpleClientset()

		// Create SA
		_, err := hvClient.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa3", Namespace: "default"},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Create secret
		_, err = hvClient.CoreV1().Secrets("default").Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa3-token", Namespace: "default"},
			Type:       corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("token"),
				corev1.ServiceAccountRootCAKey: []byte("ca"),
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Service with no VIP annotation
		_, err = hvClient.CoreV1().Services("kube-system").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress-expose",
				Namespace: "kube-system",
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: lbIPAddress,
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		result, err := getKubeConfig(context.TODO(), hvClient, "test-sa3", "default", net.JoinHostPort(lbIPAddress, "6443"))
		Expect(err).ToNot(HaveOccurred())

		// Decode and check it uses the original URL
		decoded, _ := base64.StdEncoding.DecodeString(result)
		Expect(string(decoded)).To(ContainSubstring(lbIPAddress))
	})
})

var _ = Describe("GetCloudConfigB64", func() {
	It("should return error when getKubeConfig fails due to missing SA", func() {
		hvClient := hvfake.NewSimpleClientset()
		// Don't pre-create SA - createServiceAccountIfNotExists will create it,
		// but getKubeConfig will fail because there's no token secret or ingress-expose service
		_, err := GetCloudConfigB64(context.TODO(), hvClient, "fail-sa", "default", "https://harvester.local:6443")
		Expect(err).To(HaveOccurred())
	})

	It("should create SA, CRB, and return kubeconfig", func() {
		hvClient := hvfake.NewSimpleClientset()

		// Pre-create the secret that getKubeConfig will look up
		_, err := hvClient.CoreV1().Secrets("default").Create(context.TODO(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cloud-sa-token", Namespace: "default"},
			Type:       corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				corev1.ServiceAccountTokenKey:  []byte("test-token"),
				corev1.ServiceAccountRootCAKey: []byte("test-ca"),
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Create ingress-expose service
		_, err = hvClient.CoreV1().Services("kube-system").Create(context.TODO(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress-expose",
				Namespace: "kube-system",
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP: lbIPAddress,
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		result, err := GetCloudConfigB64(context.TODO(), hvClient, "cloud-sa", "default", "https://harvester.local:6443")
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeEmpty())

		// Verify SA was created
		sa, err := hvClient.CoreV1().ServiceAccounts("default").Get(context.TODO(), "cloud-sa", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(sa.Name).To(Equal("cloud-sa"))

		// Verify CRB was created
		crb, err := hvClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), "cloud-sa", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(crb.RoleRef.Name).To(Equal(cloudProviderRoleName))
	})
})
