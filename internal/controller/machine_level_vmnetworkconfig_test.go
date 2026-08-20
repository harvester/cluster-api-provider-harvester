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
	"sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

// =============================================================================
// Tests for the machine-level VMNetworkConfig override (per machine type
// network configuration, issue #234)
// =============================================================================

var _ = Describe("effectiveVMNetworkConfig", func() {
	It("should return the machine-level config when set", func() {
		machineCfg := &infrav1.VMNetworkConfig{IPPoolRef: "pool-workers", Gateway: "10.20.0.1", SubnetMask: "255.255.255.0"}
		clusterCfg := &infrav1.VMNetworkConfig{IPPoolRef: "capi-vm-pool", Gateway: "172.16.0.1", SubnetMask: "255.255.0.0"}

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{VMNetworkConfig: machineCfg},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{VMNetworkConfig: clusterCfg},
			},
		}

		Expect(effectiveVMNetworkConfig(scope)).To(BeIdenticalTo(machineCfg))
	})

	It("should fall back to the cluster-level config when the machine has none", func() {
		clusterCfg := &infrav1.VMNetworkConfig{IPPoolRef: "capi-vm-pool", Gateway: "172.16.0.1", SubnetMask: "255.255.0.0"}

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{VMNetworkConfig: clusterCfg},
			},
		}

		Expect(effectiveVMNetworkConfig(scope)).To(BeIdenticalTo(clusterCfg))
	})

	It("should return nil when neither level has a config", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{},
		}

		Expect(effectiveVMNetworkConfig(scope)).To(BeNil())
	})
})

var _ = Describe("allocateVMIP with machine-level VMNetworkConfig", func() {
	var clusterPool *lbv1beta1.IPPool

	var workerPool *lbv1beta1.IPPool

	BeforeEach(func() {
		clusterPool = &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{},
		}
		workerPool = &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-workers"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "10.20.0.10", RangeEnd: "10.20.0.19", Subnet: "10.20.0.0/24", Gateway: "10.20.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{},
		}
	})

	It("should allocate from the machine-level pools instead of the cluster-level ones", func() {
		hvClient := hvfake.NewSimpleClientset(clusterPool, workerPool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-worker-0", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRefs: []string{"pool-workers"},
						Gateway:    "10.20.0.1",
						SubnetMask: "255.255.255.0",
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef: "capi-vm-pool",
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("pool-workers"))
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(HavePrefix("10.20.0."))
	})

	It("should allocate even when the HarvesterCluster has no VMNetworkConfig", func() {
		hvClient := hvfake.NewSimpleClientset(workerPool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-worker-1", Namespace: "test-ns"},
				Spec: infrav1.HarvesterMachineSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "pool-workers",
						Gateway:    "10.20.0.1",
						SubnetMask: "255.255.255.0",
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("pool-workers"))
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(HavePrefix("10.20.0."))
	})
})

var _ = Describe("buildNetworkDataStatic with machine-level VMNetworkConfig", func() {
	It("should use the machine-level subnet mask instead of the cluster-level one", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					Networks: []string{"default/workers"},
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "pool-workers",
						Gateway:    "10.20.0.1",
						SubnetMask: "255.255.255.0",
						DNSServers: []string{"10.20.0.53"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						SubnetMask: "255.255.0.0",
						Gateway:    "172.16.0.1",
						DNSServers: []string{"172.16.0.1"},
					},
				},
			},
			EffectiveNetworkConfig: &infrav1.NetworkConfig{
				Address:    "10.20.0.10",
				Gateway:    "10.20.0.1",
				DNSServers: []string{"10.20.0.53"},
			},
		}

		result := buildNetworkDataStatic(scope)
		Expect(result).To(ContainSubstring("netmask: 255.255.255.0"))
		Expect(result).ToNot(ContainSubstring("netmask: 255.255.0.0"))
		Expect(result).To(ContainSubstring("address: 10.20.0.10"))
		Expect(result).To(ContainSubstring("gateway: 10.20.0.1"))
		Expect(result).To(ContainSubstring("- 10.20.0.53"))
	})
})
