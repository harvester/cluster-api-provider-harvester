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

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

// =============================================================================
// Tests for per-NIC network configuration (spec.networkConfigs parallel to
// spec.networks, cluster-level vmNetworkConfig.interfaces)
// =============================================================================

func multiNICPool(name, network, start, end, subnet, gateway string) *lbv1beta1.IPPool {
	return &lbv1beta1.IPPool{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: lbv1beta1.IPPoolSpec{
			Selector: lbv1beta1.Selector{Network: network},
			Ranges: []lbv1beta1.Range{
				{RangeStart: start, RangeEnd: end, Subnet: subnet, Gateway: gateway},
			},
		},
		Status: lbv1beta1.IPPoolStatus{Allocated: map[string]string{}},
	}
}

var _ = Describe("effectiveNICConfigs", func() {
	It("should resolve each NIC from the machine-level networkConfigs", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a", "default/net-b"},
					NetworkConfigs: []infrav1.NetworkInterfaceConfig{
						{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
						{IPPoolRefs: []string{"pool-b"}, Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
		}

		cfgs := effectiveNICConfigs(scope)
		Expect(cfgs).To(HaveLen(2))
		Expect(cfgs[0].PoolRefs).To(Equal([]string{"pool-a"}))
		Expect(cfgs[0].Gateway).To(Equal("172.16.0.1"))
		Expect(cfgs[0].SubnetMask).To(Equal("255.255.255.0"))
		Expect(cfgs[1].PoolRefs).To(Equal([]string{"pool-b"}))
		Expect(cfgs[1].Gateway).To(Equal("172.17.0.1"))
		Expect(cfgs[1].SubnetMask).To(Equal("255.255.0.0"))
	})

	It("should prefer machine networkConfigs over cluster interfaces for the same index", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a", "default/net-b"},
					NetworkConfigs: []infrav1.NetworkInterfaceConfig{
						{IPPoolRef: "pool-machine", Gateway: "10.0.0.1", SubnetMask: "255.255.255.0"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "pool-cluster",
						Gateway:    "172.16.0.1",
						SubnetMask: "255.255.0.0",
						Interfaces: []infrav1.NetworkInterfaceConfig{
							{IPPoolRef: "pool-cluster", Gateway: "172.16.0.1", SubnetMask: "255.255.0.0"},
							{IPPoolRef: "pool-cluster-b", Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
						},
					},
				},
			},
		}

		cfgs := effectiveNICConfigs(scope)
		Expect(cfgs).To(HaveLen(2))
		// index 0: machine-level entry wins
		Expect(cfgs[0].PoolRefs).To(Equal([]string{"pool-machine"}))
		Expect(cfgs[0].Gateway).To(Equal("10.0.0.1"))
		// index 1: cluster-level interface applies
		Expect(cfgs[1].PoolRefs).To(Equal([]string{"pool-cluster-b"}))
		Expect(cfgs[1].Gateway).To(Equal("172.17.0.1"))
	})

	It("should apply the top-level vmNetworkConfig to the first NIC only", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a", "default/net-b"},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "pool-a",
						Gateway:    "172.16.0.1",
						SubnetMask: "255.255.0.0",
					},
				},
			},
		}

		cfgs := effectiveNICConfigs(scope)
		Expect(cfgs).To(HaveLen(2))
		Expect(cfgs[0].PoolRefs).To(Equal([]string{"pool-a"}))
		Expect(cfgs[1].PoolRefs).To(BeEmpty()) // DHCP
	})
})

var _ = Describe("allocateVMIP with per-NIC pools", func() {
	It("should allocate one IP per NIC from the pool of each NIC's network", func() {
		poolA := multiNICPool("pool-a", "default/net-a", "172.16.3.40", "172.16.3.49", "172.16.0.0/16", "172.16.0.1")
		poolB := multiNICPool("pool-b", "default/net-b", "172.17.3.40", "172.17.3.49", "172.17.0.0/16", "172.17.0.1")
		hvClient := hvfake.NewSimpleClientset(poolA, poolB)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a", "default/net-b"},
					NetworkConfigs: []infrav1.NetworkInterfaceConfig{
						{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
						{IPPoolRef: "pool-b", Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		Expect(r.allocateVMIP(scope)).To(Succeed())

		allocs := scope.HarvesterMachine.Status.AllocatedIPAddresses
		Expect(allocs).To(HaveLen(2))
		Expect(allocs[0].Network).To(Equal("default/net-a"))
		Expect(allocs[0].PoolRef).To(Equal("pool-a"))
		Expect(allocs[0].IPAddress).To(HavePrefix("172.16.3."))
		Expect(allocs[1].Network).To(Equal("default/net-b"))
		Expect(allocs[1].PoolRef).To(Equal("pool-b"))
		Expect(allocs[1].IPAddress).To(HavePrefix("172.17.3."))

		// legacy single-IP fields mirror the first NIC
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(Equal(allocs[0].IPAddress))
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("pool-a"))
	})

	It("should skip pools bound to another network for a given NIC", func() {
		// pool-wrong is bound to net-b but listed for NIC 0 (net-a): it must be
		// skipped, allocation falls back to pool-a.
		poolWrong := multiNICPool("pool-wrong", "default/net-b", "172.17.3.40", "172.17.3.49", "172.17.0.0/16", "172.17.0.1")
		poolA := multiNICPool("pool-a", "", "172.16.3.40", "172.16.3.49", "172.16.0.0/16", "172.16.0.1")
		hvClient := hvfake.NewSimpleClientset(poolWrong, poolA)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a"},
					NetworkConfigs: []infrav1.NetworkInterfaceConfig{
						{IPPoolRefs: []string{"pool-wrong", "pool-a"}, Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		Expect(r.allocateVMIP(scope)).To(Succeed())
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("pool-a"))
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(HavePrefix("172.16.3."))
	})

	It("should return an error when a NIC has no matching pool", func() {
		poolWrong := multiNICPool("pool-wrong", "default/net-b", "172.17.3.40", "172.17.3.49", "172.17.0.0/16", "172.17.0.1")
		hvClient := hvfake.NewSimpleClientset(poolWrong)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a"},
					NetworkConfigs: []infrav1.NetworkInterfaceConfig{
						{IPPoolRef: "pool-wrong", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("matches the NIC network"))
	})

	It("should be idempotent across reconciles with multiple NICs", func() {
		poolA := multiNICPool("pool-a", "default/net-a", "172.16.3.40", "172.16.3.49", "172.16.0.0/16", "172.16.0.1")
		poolB := multiNICPool("pool-b", "default/net-b", "172.17.3.40", "172.17.3.49", "172.17.0.0/16", "172.17.0.1")
		hvClient := hvfake.NewSimpleClientset(poolA, poolB)
		logger := log.FromContext(context.TODO())

		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns"},
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/net-a", "default/net-b"},
				NetworkConfigs: []infrav1.NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					{IPPoolRef: "pool-b", Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
				},
			},
		}
		scope := &Scope{
			HarvesterMachine: machine,
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		Expect(r.allocateVMIP(scope)).To(Succeed())
		first := machine.Status.AllocatedIPAddresses
		Expect(first).To(HaveLen(2))

		// second reconcile: same IPs, pools not allocated twice
		Expect(r.allocateVMIP(scope)).To(Succeed())
		Expect(machine.Status.AllocatedIPAddresses).To(Equal(first))

		updatedA, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "pool-a", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(updatedA.Status.Allocated).To(HaveLen(1))
	})
})

var _ = Describe("buildNetworkDataStatic with per-NIC static IPs", func() {
	It("should write a static subnet with each NIC's own mask, the default route (gateway) on the first NIC only, DHCP on the rest", func() {
		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns", UID: "uid-1"},
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/net-a", "default/net-b", "default/net-c"},
				NetworkConfigs: []infrav1.NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					{IPPoolRef: "pool-b", Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
				},
			},
			Status: infrav1.HarvesterMachineStatus{
				AllocatedIPAddresses: []infrav1.NICIPAllocation{
					{Network: "default/net-a", IPAddress: "172.16.3.40", PoolRef: "pool-a"},
					{Network: "default/net-b", IPAddress: "172.17.3.40", PoolRef: "pool-b"},
				},
			},
		}
		scope := &Scope{
			HarvesterMachine: machine,
			HarvesterCluster: &infrav1.HarvesterCluster{},
			EffectiveNetworkConfig: &infrav1.NetworkConfig{
				Address:    "172.16.3.40",
				Gateway:    "172.16.0.1",
				DNSServers: []string{"172.16.0.1"},
			},
		}

		result := buildNetworkDataStatic(scope)

		// NIC 0: static with its own gateway/mask
		Expect(result).To(ContainSubstring("name: eth0"))
		Expect(result).To(ContainSubstring("address: 172.16.3.40"))
		Expect(result).To(ContainSubstring("netmask: 255.255.255.0"))
		Expect(result).To(ContainSubstring("gateway: 172.16.0.1"))
		// NIC 1: static with its own (different) mask, but no default route —
		// emitting a gateway per NIC would install one default route per interface
		Expect(result).To(ContainSubstring("name: eth1"))
		Expect(result).To(ContainSubstring("address: 172.17.3.40"))
		Expect(result).To(ContainSubstring("netmask: 255.255.0.0"))
		Expect(result).ToNot(ContainSubstring("gateway: 172.17.0.1"))
		Expect(CountOccurrences(result, "gateway:")).To(Equal(1))
		// NIC 2: no allocation -> DHCP
		Expect(result).To(ContainSubstring("name: eth2"))
		// one dhcp entry only (for eth2)
		Expect(result).To(ContainSubstring("- type: dhcp"))
		Expect(CountOccurrences(result, "- type: dhcp")).To(Equal(1))
		Expect(CountOccurrences(result, "- type: static")).To(Equal(2))
		// DNS block still emitted once
		Expect(CountOccurrences(result, "type: nameserver")).To(Equal(1))
	})

	It("should keep the legacy behavior when only the first NIC is static", func() {
		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns", UID: "uid-2"},
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/net-a", "default/net-b"},
			},
			Status: infrav1.HarvesterMachineStatus{
				AllocatedIPAddress: "172.16.3.40",
				AllocatedPoolRef:   "pool-a",
			},
		}
		scope := &Scope{
			HarvesterMachine: machine,
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "pool-a",
						Gateway:    "172.16.0.1",
						SubnetMask: "255.255.0.0",
					},
				},
			},
			EffectiveNetworkConfig: &infrav1.NetworkConfig{Address: "172.16.3.40", Gateway: "172.16.0.1"},
		}

		result := buildNetworkDataStatic(scope)
		Expect(CountOccurrences(result, "- type: static")).To(Equal(1))
		Expect(CountOccurrences(result, "- type: dhcp")).To(Equal(1))
		Expect(result).To(ContainSubstring("netmask: 255.255.0.0"))
	})
})

var _ = Describe("releaseVMIP with multiple NIC allocations", func() {
	It("should release every allocated IP back to its pool", func() {
		poolA := multiNICPool("pool-a", "default/net-a", "172.16.3.40", "172.16.3.49", "172.16.0.0/16", "172.16.0.1")
		poolA.Status.Allocated = map[string]string{
			"172.16.3.40": "test-ns/test-vm-0",
			"172.16.3.41": "test-ns/other",
		}
		poolB := multiNICPool("pool-b", "default/net-b", "172.17.3.40", "172.17.3.49", "172.17.0.0/16", "172.17.0.1")
		poolB.Status.Allocated = map[string]string{
			"172.17.3.40": "test-ns/test-vm-0",
		}
		hvClient := hvfake.NewSimpleClientset(poolA, poolB)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-0", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/net-a", "default/net-b"},
				},
				Status: infrav1.HarvesterMachineStatus{
					AllocatedIPAddress: "172.16.3.40",
					AllocatedPoolRef:   "pool-a",
					AllocatedIPAddresses: []infrav1.NICIPAllocation{
						{Network: "default/net-a", IPAddress: "172.16.3.40", PoolRef: "pool-a"},
						{Network: "default/net-b", IPAddress: "172.17.3.40", PoolRef: "pool-b"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		r.releaseVMIP(scope)

		updatedA, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "pool-a", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(updatedA.Status.Allocated).To(HaveLen(1)) // only "other" remains
		Expect(updatedA.Status.Allocated).ToNot(HaveKey("172.16.3.40"))

		updatedB, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "pool-b", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(updatedB.Status.Allocated).To(BeEmpty())
	})
})

// CountOccurrences counts non-overlapping occurrences of substr in s.
func CountOccurrences(s, substr string) int {
	count := 0
	for i := 0; i+len(substr) <= len(s); {
		if s[i:i+len(substr)] == substr {
			count++
			i += len(substr)
		} else {
			i++
		}
	}

	return count
}
