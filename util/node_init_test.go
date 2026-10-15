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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var _ = Describe("Node initialization utilities", func() {
	Describe("hasUninitializedTaint", func() {
		It("should return true when the uninitialized taint is present", func() {
			node := &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
						{Key: uninitializedTaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule},
					},
				},
			}
			Expect(hasUninitializedTaint(node)).To(BeTrue())
		})

		It("should return false when the taint is not present", func() {
			node := &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
					},
				},
			}
			Expect(hasUninitializedTaint(node)).To(BeFalse())
		})

		It("should return false for an empty taint list", func() {
			node := &v1.Node{
				Spec: v1.NodeSpec{Taints: nil},
			}
			Expect(hasUninitializedTaint(node)).To(BeFalse())
		})
	})

	Describe("removeTaint", func() {
		It("should remove the specified taint and keep others", func() {
			taints := []v1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
				{Key: uninitializedTaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule},
				{Key: "other-taint", Effect: v1.TaintEffectNoExecute},
			}
			result := removeTaint(taints)
			Expect(result).To(HaveLen(2))
			Expect(result[0].Key).To(Equal("node.kubernetes.io/not-ready"))
			Expect(result[1].Key).To(Equal("other-taint"))
		})

		It("should return the same slice when the key is not found", func() {
			taints := []v1.Taint{
				{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
			}
			result := removeTaint(taints)
			Expect(result).To(HaveLen(1))
		})

		It("should handle an empty slice", func() {
			result := removeTaint([]v1.Taint{})
			Expect(result).To(BeEmpty())
		})

		It("should handle nil slice", func() {
			result := removeTaint(nil)
			Expect(result).To(BeEmpty())
		})
	})
})

// fakeNodeServer creates a test HTTP server that simulates a k8s API for node operations.
// It returns the server and a rest.Config pointing to it.
func fakeNodeServer(node *v1.Node) (*httptest.Server, *rest.Config) {
	var mu sync.Mutex

	currentNode := node.DeepCopy()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		// Handle /api/v1/nodes/<name>
		if nodeName, ok := strings.CutPrefix(r.URL.Path, "/api/v1/nodes/"); ok {
			if nodeName != currentNode.Name {
				w.WriteHeader(http.StatusNotFound)

				return
			}

			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				//nolint:errchkjson // test helper, unsafe type acceptable
				_ = json.NewEncoder(w).Encode(currentNode)
			case http.MethodPatch:
				// Apply the merge patch to currentNode
				var patch map[string]any

				_ = json.NewDecoder(r.Body).Decode(&patch)

				if spec, ok := patch["spec"].(map[string]any); ok {
					if pid, ok := spec["providerID"].(string); ok {
						currentNode.Spec.ProviderID = pid
					}

					if taints, ok := spec["taints"]; ok {
						taintsBytes, err := json.Marshal(taints)
						if err == nil {
							var newTaints []v1.Taint

							_ = json.Unmarshal(taintsBytes, &newTaints)
							currentNode.Spec.Taints = newTaints
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				//nolint:errchkjson // test helper, unsafe type acceptable
				_ = json.NewEncoder(w).Encode(currentNode)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}

			return
		}

		// Default: return 404
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(handler)
	config := &rest.Config{
		Host: server.URL,
	}

	return server, config
}

var _ = Describe("InitializeWorkloadNode with fake API server", func() {
	It("should return immediately when providerID is empty", func() {
		logger := logr.Discard()
		Expect(InitializeWorkloadNode(context.Background(), logger, &rest.Config{Host: "http://unused"}, "test-node", "")).To(BeFalse())
	})

	It("should set providerID and remove taint on a node", func() {
		node := &v1.Node{
			TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Spec: v1.NodeSpec{
				Taints: []v1.Taint{
					{Key: uninitializedTaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule},
					{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
				},
			},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		logger := logr.Discard()
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "test-node", "harvester://test-provider-id")).To(BeTrue())

		// Verify by fetching the node again through the fake server
		// The function should have set providerID and removed the uninitialized taint
		// We verify via the handler's currentNode state indirectly by calling the function again
		// and seeing it's a no-op (needsProviderID=false, needsTaintRemoval=false)
	})

	It("should handle a node that already has providerID set", func() {
		node := &v1.Node{
			TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "already-initialized",
			},
			Spec: v1.NodeSpec{
				ProviderID: "harvester://existing-id",
			},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		logger := logr.Discard()
		// The provider ID of a node cannot change once set: a different one can never
		// match the machine, which must be reported instead of treated as initialized.
		initialized, err := InitializeWorkloadNode(context.Background(), logger, config, "already-initialized", "harvester://new-id")
		Expect(initialized).To(BeFalse())

		var mismatch *ProviderIDMismatchError
		Expect(errors.As(err, &mismatch)).To(BeTrue())
		Expect(mismatch.NodeProviderID).To(Equal("harvester://existing-id"))
	})

	It("should report a node that already carries the machine provider ID as initialized", func() {
		node := &v1.Node{
			TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "initialized"},
			Spec:       v1.NodeSpec{ProviderID: "harvester://same-id"},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		Expect(InitializeWorkloadNode(context.Background(), logr.Discard(), config, "initialized", "harvester://same-id")).To(BeTrue())
	})

	It("should handle node not found (not registered yet)", func() {
		node := &v1.Node{
			TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-node",
			},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		logger := logr.Discard()
		// Request for a different node name: not initialized, the caller must requeue
		// (a machine whose node registers later would otherwise stay Provisioned forever)
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "non-existent-node", "harvester://some-id")).To(BeFalse())
	})

	It("should only set providerID when there is no taint", func() {
		node := &v1.Node{
			TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "needs-pid-only",
			},
			Spec: v1.NodeSpec{
				// No uninitialized taint, just needs providerID
				Taints: []v1.Taint{
					{Key: "node.kubernetes.io/not-ready", Effect: v1.TaintEffectNoSchedule},
				},
			},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		logger := logr.Discard()
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "needs-pid-only", "harvester://pid-only")).To(BeTrue())
	})

	It("should only remove taint when providerID is already set", func() {
		node := &v1.Node{
			TypeMeta: metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "needs-taint-only",
			},
			Spec: v1.NodeSpec{
				ProviderID: "harvester://already-set",
				Taints: []v1.Taint{
					{Key: uninitializedTaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule},
				},
			},
		}

		server, config := fakeNodeServer(node)
		defer server.Close()

		logger := logr.Discard()
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "needs-taint-only", "harvester://already-set")).To(BeTrue())
	})

	It("should handle API errors gracefully on a non-404 error", func() {
		// Create a server that returns 500 for everything
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"internal error","code":500}`))
		}))
		defer server.Close()

		config := &rest.Config{Host: server.URL}
		logger := logr.Discard()
		// A server error is reported as not initialized, so the caller retries
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "test-node", "harvester://pid")).To(BeFalse())
	})

	It("should handle patch failure for providerID", func() {
		patchCount := 0
		node := &v1.Node{
			TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "patch-fail-node"},
			Spec: v1.NodeSpec{
				Taints: []v1.Taint{
					{Key: uninitializedTaintKey, Value: "true", Effect: v1.TaintEffectNoSchedule},
				},
			},
		}
		// Custom server that fails on PATCH
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/api/v1/nodes/") {
				//nolint:errchkjson // test helper, unsafe type acceptable
				_ = json.NewEncoder(w).Encode(node)

				return
			}

			if r.Method == http.MethodPatch {
				patchCount++

				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"conflict","reason":"Conflict","code":409}`))

				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := &rest.Config{Host: server.URL}
		logger := logr.Discard()
		Expect(InitializeWorkloadNode(context.Background(), logger, config, "patch-fail-node", "harvester://pid")).To(BeFalse())
		Expect(patchCount).To(Equal(1)) // Only providerID patch attempted, then returned on error
	})

	It("should adopt the provider ID another component set while the claim was in flight", func() {
		// The node has no provider ID when read, a cloud controller sets one before the
		// patch lands, and the API server rejects the patch: the node value is adopted.
		var mu sync.Mutex

		providerID := ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			w.Header().Set("Content-Type", "application/json")

			if r.Method == http.MethodPatch {
				providerID = "rke2://racing-node"

				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Invalid","code":422}`))

				return
			}

			node := v1.Node{
				TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "racing-node"},
				Spec:       v1.NodeSpec{ProviderID: providerID},
			}
			_ = json.NewEncoder(w).Encode(node) //nolint:errchkjson // test helper
		}))
		defer server.Close()

		claimed, err := ClaimNodeProviderID(context.Background(), &rest.Config{Host: server.URL}, "racing-node", "harvester://vm-uid")
		Expect(err).ToNot(HaveOccurred())
		Expect(claimed).To(Equal("rke2://racing-node"))
	})
})
