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
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

const (
	zoneTopologyLabel     = "topology.kubernetes.io/zone"
	hostnameTopologyLabel = "kubernetes.io/hostname"

	// failureDomainTopologyKeyAttribute records, on each published failure
	// domain, the node label the machine controller must schedule on to land
	// in that domain.
	failureDomainTopologyKeyAttribute = "topologyKey"
)

// failureDomainsFromNodes derives the failure domains of a Harvester cluster
// from its hosts: one domain per topology.kubernetes.io/zone value when every
// host carries the label, otherwise one domain per host so that all hosts stay
// reachable. Domains are sorted by name and marked suitable for control plane
// machines.
func failureDomainsFromNodes(nodes []corev1.Node) []clusterv1.FailureDomain {
	if len(nodes) == 0 {
		return nil
	}

	topologyKey := zoneTopologyLabel
	names := map[string]struct{}{}

	for _, node := range nodes {
		if node.Labels[zoneTopologyLabel] == "" {
			topologyKey = hostnameTopologyLabel

			break
		}
	}

	for _, node := range nodes {
		if topologyKey == zoneTopologyLabel {
			names[node.Labels[zoneTopologyLabel]] = struct{}{}
		} else {
			names[node.Name] = struct{}{}
		}
	}

	domains := make([]clusterv1.FailureDomain, 0, len(names))
	for name := range names {
		domains = append(domains, clusterv1.FailureDomain{
			Name:         name,
			ControlPlane: ptr.To(true),
			Attributes:   map[string]string{failureDomainTopologyKeyAttribute: topologyKey},
		})
	}

	sort.Slice(domains, func(i, j int) bool { return domains[i].Name < domains[j].Name })

	return domains
}

// reconcileFailureDomains discovers the Harvester hosts and publishes the
// resulting failure domains in the HarvesterCluster status. Discovery errors
// only log: failure domains are an enhancement and must not block the
// infrastructure reconciliation.
func reconcileFailureDomains(scope *ClusterScope) {
	nodes, err := scope.HarvesterClient.CoreV1().Nodes().List(scope.Ctx, metav1.ListOptions{})
	if err != nil {
		scope.Logger.Info("Warning: unable to list Harvester hosts for failure domain discovery", "error", err)

		return
	}

	scope.HarvesterCluster.Status.FailureDomains = failureDomainsFromNodes(nodes.Items)
}

// effectiveFailureDomain returns the failure domain the machine must land in:
// the one assigned by CAPI on the owner Machine (the control plane provider
// picks it from the published domains), or the HarvesterMachine field for
// machines pinned directly by the user.
func effectiveFailureDomain(hvScope *Scope) string {
	if hvScope.Machine != nil && hvScope.Machine.Spec.FailureDomain != "" {
		return hvScope.Machine.Spec.FailureDomain
	}

	return hvScope.HarvesterMachine.Spec.FailureDomain
}

// failureDomainNodeSelector returns the node label and value a machine pinned
// to the given failure domain must schedule on, based on the topology key the
// cluster controller recorded on the published domain. An unpublished domain
// name falls back to a host name constraint.
func failureDomainNodeSelector(cluster *infrav1.HarvesterCluster, failureDomain string) (key, value string) {
	for _, domain := range cluster.Status.FailureDomains {
		if domain.Name == failureDomain {
			if k := domain.Attributes[failureDomainTopologyKeyAttribute]; k != "" {
				return k, failureDomain
			}

			break
		}
	}

	return hostnameTopologyLabel, failureDomain
}
