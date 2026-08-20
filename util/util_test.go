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

package util

import (
	"context"
	"encoding/base64"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

var (
	namespace             string
	name                  string
	expectedResultingName string
)

var _ = Describe("GenerateNameFromFirstComponentWithDigit", func() {
	BeforeEach(func() {
		namespace = "1test"
		name = "test-1"

		expectedResultingName = "a-1test-test-1"
	})

	It("Should return the right name", func() {
		name := GenerateRFC1035Name([]string{namespace, name})
		Expect(name).To(Equal(expectedResultingName))
	})
})

var _ = Describe("GenerateNameFromUpperCase", func() {
	BeforeEach(func() {
		namespace = "TeStNS"
		name = "test"

		expectedResultingName = "testns-test"
	})

	It("Should return the right name", func() {
		name := GenerateRFC1035Name([]string{namespace, name})
		Expect(name).To(Equal(expectedResultingName))
	})
})

var _ = Describe("GenerateNameLimitedTo63", func() {
	BeforeEach(func() {
		namespace = "1TeStNS123"
		name = "namewith20characters-namewith20characters-namewith20characters"

		expectedResultingName = "a-1testns123-namewith20characters-namewith20characters-namewith"
	})

	It("Should return the right name", func() {
		name := GenerateRFC1035Name([]string{namespace, name})
		Expect(name).To(Equal(expectedResultingName))
	})
})

var _ = Describe("CheckNamespacedName", func() {
	It("should accept valid namespace/name format", func() {
		Expect(CheckNamespacedName("default/production")).To(BeTrue())
	})

	It("should accept names with dots and underscores", func() {
		Expect(CheckNamespacedName("default/sles15-sp7-minimal-vm.x86_64-cloud-qu2.qcow2")).To(BeTrue())
	})

	It("should reject names without a slash", func() {
		Expect(CheckNamespacedName("production")).To(BeFalse())
	})

	It("should reject empty string", func() {
		Expect(CheckNamespacedName("")).To(BeFalse())
	})

	It("should reject names with uppercase", func() {
		Expect(CheckNamespacedName("Default/Production")).To(BeFalse())
	})

	It("should reject names with multiple slashes", func() {
		Expect(CheckNamespacedName("a/b/c")).To(BeFalse())
	})

	It("should reject names with spaces", func() {
		Expect(CheckNamespacedName("default/my name")).To(BeFalse())
	})
})

var _ = Describe("GetNamespacedName", func() {
	It("should split namespace/name correctly", func() {
		nn, err := GetNamespacedName("default/production", "fallback-ns")
		Expect(err).ToNot(HaveOccurred())
		Expect(nn.Namespace).To(Equal("default"))
		Expect(nn.Name).To(Equal("production"))
	})

	It("should use alternative namespace when only name is given", func() {
		nn, err := GetNamespacedName("production", "fallback-ns")
		Expect(err).ToNot(HaveOccurred())
		Expect(nn.Namespace).To(Equal("fallback-ns"))
		Expect(nn.Name).To(Equal("production"))
	})

	It("should return error for malformed reference", func() {
		_, err := GetNamespacedName("Invalid Name!", "fallback-ns")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("malformed reference"))
	})

	It("should handle names with dots and underscores in namespace/name format", func() {
		nn, err := GetNamespacedName("default/image.x86_64", "fallback-ns")
		Expect(err).ToNot(HaveOccurred())
		Expect(nn.Namespace).To(Equal("default"))
		Expect(nn.Name).To(Equal("image.x86_64"))
	})

	It("should handle names with dots when using fallback namespace", func() {
		nn, err := GetNamespacedName("my-resource.name", "fallback-ns")
		Expect(err).ToNot(HaveOccurred())
		Expect(nn.Namespace).To(Equal("fallback-ns"))
		Expect(nn.Name).To(Equal("my-resource.name"))
	})
})

var _ = Describe("RandomID", func() {
	It("should return a 5-character string matching [a-z]{3}[0-9][a-z]", func() {
		id := RandomID()
		Expect(id).To(MatchRegexp(`^[a-z]{3}[0-9][a-z]$`))
	})

	It("should return different IDs on subsequent calls", func() {
		ids := map[string]bool{}
		for range 10 {
			ids[RandomID()] = true
		}
		// With 26^4*10 = 4,569,760 combinations, 10 calls should almost always differ
		Expect(len(ids)).To(BeNumerically(">", 1))
	})
})

var _ = Describe("NewTrue", func() {
	It("should return a pointer to true", func() {
		ptr := NewTrue()
		Expect(ptr).ToNot(BeNil())
		Expect(*ptr).To(BeTrue())
	})
})

var _ = Describe("Filter", func() {
	It("should filter integers", func() {
		result := Filter([]int{1, 2, 3, 4, 5}, func(i int) bool { return i > 3 })
		Expect(result).To(Equal([]int{4, 5}))
	})

	It("should return nil for no matches", func() {
		result := Filter([]int{1, 2, 3}, func(i int) bool { return i > 10 })
		Expect(result).To(BeNil())
	})

	It("should filter strings", func() {
		result := Filter([]string{"foo", "bar", "baz"}, func(s string) bool { return s != "bar" })
		Expect(result).To(Equal([]string{"foo", "baz"}))
	})

	It("should handle empty slice", func() {
		result := Filter([]int{}, func(_ int) bool { return true })
		Expect(result).To(BeNil())
	})
})

var _ = Describe("ValidateB64Kubeconfig", func() {
	It("should validate a correct base64-encoded kubeconfig", func() {
		kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://example.com
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: dummy
`
		b64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))
		err := ValidateB64Kubeconfig(b64)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should reject invalid base64", func() {
		err := ValidateB64Kubeconfig("not-valid-base64!!!")
		Expect(err).To(HaveOccurred())
	})

	It("should reject invalid kubeconfig content", func() {
		b64 := base64.StdEncoding.EncodeToString([]byte("this is not yaml kubeconfig"))
		err := ValidateB64Kubeconfig(b64)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetSecretForHarvesterConfig", func() {
	It("should retrieve the secret referenced by the cluster's identity", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hv-identity",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"kubeconfig": []byte("test-kubeconfig-data"),
			},
		}

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				IdentitySecret: infrav1.SecretKey{
					Name:      "hv-identity",
					Namespace: "default",
				},
			},
		}

		result, err := GetSecretForHarvesterConfig(context.Background(), cluster, cl)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Name).To(Equal("hv-identity"))
		Expect(string(result.Data["kubeconfig"])).To(Equal("test-kubeconfig-data"))
	})

	It("should return error when the secret does not exist", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)

		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		cluster := &infrav1.HarvesterCluster{
			Spec: infrav1.HarvesterClusterSpec{
				IdentitySecret: infrav1.SecretKey{
					Name:      "nonexistent",
					Namespace: "default",
				},
			},
		}

		_, err := GetSecretForHarvesterConfig(context.Background(), cluster, cl)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetDataKeyFromConfigMap in util", func() {
	It("should return the value for an existing key", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
			Data:       map[string]string{"mykey": "myvalue"},
		}
		val, err := GetDataKeyFromConfigMap(cm, "mykey")
		Expect(err).ToNot(HaveOccurred())
		Expect(val).To(Equal("myvalue"))
	})

	It("should return an error for a missing key", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
			Data:       map[string]string{"other": "value"},
		}
		_, err := GetDataKeyFromConfigMap(cm, "missing")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing"))
	})
})
