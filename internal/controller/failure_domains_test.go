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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// =============================================================================
// Tests for failure domain discovery and placement (issue #237)
// =============================================================================

func nodeWithLabels(name string, labels map[string]string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

var _ = Describe("failureDomainsFromNodes", func() {
	It("should publish one domain per zone when every host carries the zone label", func() {
		nodes := []corev1.Node{
			nodeWithLabels("host-1", map[string]string{zoneTopologyLabel: "zone-b"}),
			nodeWithLabels("host-2", map[string]string{zoneTopologyLabel: "zone-a"}),
			nodeWithLabels("host-3", map[string]string{zoneTopologyLabel: "zone-a"}),
		}

		domains := failureDomainsFromNodes(nodes)

		Expect(domains).To(HaveLen(2))
		Expect(domains[0].Name).To(Equal("zone-a"))
		Expect(domains[1].Name).To(Equal("zone-b"))
		Expect(domains[0].ControlPlane).To(HaveValue(BeTrue()))
		Expect(domains[0].Attributes).To(HaveKeyWithValue(failureDomainTopologyKeyAttribute, zoneTopologyLabel))
	})

	It("should fall back to one domain per host when a host misses the zone label", func() {
		nodes := []corev1.Node{
			nodeWithLabels("host-2", map[string]string{zoneTopologyLabel: "zone-a"}),
			nodeWithLabels("host-1", nil),
		}

		domains := failureDomainsFromNodes(nodes)

		Expect(domains).To(HaveLen(2))
		Expect(domains[0].Name).To(Equal("host-1"))
		Expect(domains[1].Name).To(Equal("host-2"))
		Expect(domains[0].Attributes).To(HaveKeyWithValue(failureDomainTopologyKeyAttribute, hostnameTopologyLabel))
	})

	It("should return nothing for an empty node list", func() {
		Expect(failureDomainsFromNodes(nil)).To(BeEmpty())
	})
})

var _ = Describe("effectiveFailureDomain", func() {
	It("should use the owner Machine failure domain assigned by the control plane provider", func() {
		scope := &Scope{
			Machine:          &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: "zone-a"}},
			HarvesterMachine: &infrav1.HarvesterMachine{},
		}

		Expect(effectiveFailureDomain(scope)).To(Equal("zone-a"))
	})

	It("should fall back to the HarvesterMachine field for direct users", func() {
		scope := &Scope{
			Machine: &clusterv1.Machine{},
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{FailureDomain: "zone-b"},
			},
		}

		Expect(effectiveFailureDomain(scope)).To(Equal("zone-b"))
	})

	It("should return empty without a machine or a domain", func() {
		scope := &Scope{HarvesterMachine: &infrav1.HarvesterMachine{}}

		Expect(effectiveFailureDomain(scope)).To(BeEmpty())
	})
})

var _ = Describe("buildAffinity with a failure domain", func() {
	newScope := func(failureDomain string, domains []clusterv1.FailureDomain, userAffinity *corev1.NodeAffinity) *Scope {
		return &Scope{
			Machine: &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: failureDomain}},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0"},
				Spec: infrav1.HarvesterMachineSpec{
					NodeAffinity: userAffinity,
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Status: infrav1.HarvesterClusterStatus{FailureDomains: domains},
			},
		}
	}

	It("should add a required node affinity on the domain topology key", func() {
		domains := []clusterv1.FailureDomain{
			{Name: "zone-a", Attributes: map[string]string{failureDomainTopologyKeyAttribute: zoneTopologyLabel}},
		}

		affinity := buildAffinity(newScope("zone-a", domains, nil))

		Expect(affinity.NodeAffinity).ToNot(BeNil())
		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(ContainElement(corev1.NodeSelectorRequirement{
			Key:      zoneTopologyLabel,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"zone-a"},
		}))
	})

	It("should fall back to the hostname label when the domain is not published", func() {
		affinity := buildAffinity(newScope("host-9", nil, nil))

		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(ContainElement(corev1.NodeSelectorRequirement{
			Key:      hostnameTopologyLabel,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"host-9"},
		}))
	})

	It("should append the constraint to every user provided required term", func() {
		userAffinity := &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "size", Operator: corev1.NodeSelectorOpIn, Values: []string{"large"}},
					}},
				},
			},
		}
		domains := []clusterv1.FailureDomain{
			{Name: "zone-a", Attributes: map[string]string{failureDomainTopologyKeyAttribute: zoneTopologyLabel}},
		}

		affinity := buildAffinity(newScope("zone-a", domains, userAffinity))

		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		Expect(terms).To(HaveLen(1))
		Expect(terms[0].MatchExpressions).To(HaveLen(2))
		Expect(terms[0].MatchExpressions[0].Key).To(Equal("size"))
		Expect(terms[0].MatchExpressions[1].Key).To(Equal(zoneTopologyLabel))
	})

	It("should leave the affinity unchanged when no failure domain is set", func() {
		affinity := buildAffinity(newScope("", nil, nil))

		Expect(affinity.NodeAffinity).To(BeNil())
	})
})
