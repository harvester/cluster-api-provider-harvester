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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

const (
	// uninitializedTaintKey is the taint added by kubelet when cloudProviderName=external.
	uninitializedTaintKey = "node.cloudprovider.kubernetes.io/uninitialized"
)

// InitializeWorkloadNode sets the providerID and removes the cloud-provider
// uninitialized taint on a workload cluster node. This bypasses the
// chicken-and-egg problem where the cloud-provider-harvester pod cannot
// schedule because CNI is blocked by the uninitialized taint.
//
// This is a best-effort operation: errors are logged as warnings and reported as
// false so the caller retries. The only error returned is a ProviderIDMismatchError,
// which retrying cannot fix.
func InitializeWorkloadNode(ctx context.Context, logger logr.Logger, workloadConfig *rest.Config, nodeName, providerID string) (bool, error) {
	if providerID == "" {
		return false, nil
	}

	clientset, err := kubernetes.NewForConfig(workloadConfig)
	if err != nil {
		logger.Info("Warning: failed to create workload client for node init", "error", err)

		return false, nil
	}

	nodes := clientset.CoreV1().Nodes()

	nodeProviderID, err := claimNodeProviderID(ctx, nodes, nodeName, providerID)
	if err != nil {
		logger.Info("Warning: failed to set providerID on workload node", "error", err, "node", nodeName, "providerID", providerID)

		return false, nil
	}

	if nodeProviderID == "" {
		// Node not yet registered in workload cluster: report it so the caller requeues
		return false, nil
	}

	// A Node provider ID cannot be changed once set, and Cluster API matches the
	// Machine to its Node by provider ID: a different one set first by another
	// component can never match.
	if nodeProviderID != providerID {
		return false, &ProviderIDMismatchError{Node: nodeName, NodeProviderID: nodeProviderID, MachineProviderID: providerID}
	}

	node, err := nodes.Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		logger.Info("Warning: failed to get workload node for init", "error", err, "node", nodeName)

		return false, nil
	}

	if !hasUninitializedTaint(node) {
		return true, nil
	}

	taintsJSON, err := json.Marshal(removeTaint(node.Spec.Taints))
	if err != nil {
		logger.Info("Warning: failed to marshal taints", "error", err, "node", nodeName)

		return false, nil
	}

	patch := fmt.Sprintf(`{"spec":{"taints":%s}}`, taintsJSON)

	_, err = nodes.Patch(ctx, nodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		logger.Info("Warning: failed to remove uninitialized taint", "error", err, "node", nodeName)

		return false, nil
	}

	logger.Info("Removed cloud-provider uninitialized taint", "node", nodeName)

	return true, nil
}

// hasUninitializedTaint returns true if the node has the cloud-provider uninitialized taint.
func hasUninitializedTaint(node *v1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == uninitializedTaintKey {
			return true
		}
	}

	return false
}

// removeTaint returns a copy of the taint slice with the uninitialized taint key removed.
func removeTaint(taints []v1.Taint) []v1.Taint {
	result := make([]v1.Taint, 0, len(taints))

	for _, t := range taints {
		if t.Key != uninitializedTaintKey {
			result = append(result, t)
		}
	}

	return result
}

// ProviderIDMismatchError reports a workload Node whose provider ID differs from the
// provider ID of its machine.
type ProviderIDMismatchError struct {
	Node              string
	NodeProviderID    string
	MachineProviderID string
}

func (e *ProviderIDMismatchError) Error() string {
	return fmt.Sprintf("node %s has provider ID %s instead of %s, set first by another component "+
		"(with RKE2 and an external cloud provider, disable the embedded cloud controller: "+
		"serverConfig.disableComponents.kubernetesComponents: [cloudController])",
		e.Node, e.NodeProviderID, e.MachineProviderID)
}

// ClaimNodeProviderID sets providerID on the workload Node nodeName when it has none,
// and returns the provider ID the Node carries afterwards: providerID, or the one
// another component (a cloud controller) set first. The Node provider ID cannot be
// changed once set, so the machine must adopt the value of the Node. It returns ""
// without error when the Node has not registered yet.
func ClaimNodeProviderID(ctx context.Context, workloadConfig *rest.Config, nodeName, providerID string) (string, error) {
	clientset, err := kubernetes.NewForConfig(workloadConfig)
	if err != nil {
		return "", errors.Wrap(err, "unable to create a workload cluster client")
	}

	return claimNodeProviderID(ctx, clientset.CoreV1().Nodes(), nodeName, providerID)
}

func claimNodeProviderID(ctx context.Context, nodes corev1client.NodeInterface, nodeName, providerID string) (string, error) {
	node, err := nodes.Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", nil
	}

	if err != nil {
		return "", errors.Wrapf(err, "unable to get workload node %s", nodeName)
	}

	if node.Spec.ProviderID != "" {
		return node.Spec.ProviderID, nil
	}

	patch := fmt.Sprintf(`{"spec":{"providerID":%q}}`, providerID)

	_, patchErr := nodes.Patch(ctx, nodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	if patchErr == nil {
		return providerID, nil
	}

	// Another component may have set it in the meantime: adopt its value.
	node, err = nodes.Get(ctx, nodeName, metav1.GetOptions{})
	if err == nil && node.Spec.ProviderID != "" {
		return node.Spec.ProviderID, nil
	}

	return "", errors.Wrapf(patchErr, "unable to set the provider ID of workload node %s", nodeName)
}
