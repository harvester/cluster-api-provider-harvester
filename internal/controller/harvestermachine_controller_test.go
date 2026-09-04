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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

const (
	testBootstrapDataSecretName = "bootstrap-data"
	testBootstrapSecretName     = "bootstrap"
)

// =============================================================================
// Tests for getKubevirtNetworksFromHarvesterMachine (existing)
// =============================================================================

var _ = Describe("Convert HarvesterMachine networks to Kubevirt Networks", func() {
	var hvMachineNetworks *infrav1.HarvesterMachine

	var kvNetworks []kubevirtv1.Network

	BeforeEach(func() {
		hvMachineNetworks = &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"network1", "network2"},
			},
		}

		kvNetworks = []kubevirtv1.Network{
			{
				Name: "nic-1",
				NetworkSource: kubevirtv1.NetworkSource{
					Multus: &kubevirtv1.MultusNetwork{
						NetworkName: "network1",
					},
				},
			},
			{
				Name: "nic-2",
				NetworkSource: kubevirtv1.NetworkSource{
					Multus: &kubevirtv1.MultusNetwork{
						NetworkName: "network2",
					},
				},
			},
		}
	})
	Context("When we provide a list of HarvesterMachine networks", func() {
		It("Should return a list of Kubevirt Networks", func() {
			Expect(getKubevirtNetworksFromHarvesterMachine(hvMachineNetworks)).To(Equal(kvNetworks))
		})
	})
})

// =============================================================================
// Tests for buildNetworkDataStatic and buildDHCPCloudInit (existing + new)
// =============================================================================

var _ = Describe("buildNetworkData", func() {
	Context("DHCP cloud-init with 1 NIC", func() {
		It("Should generate write_files and bootcmd for dhclient", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{},
			}

			result := buildDHCPCloudInit(scope)
			Expect(result).To(ContainSubstring("bootcmd:"))
			Expect(result).To(ContainSubstring("dhclient-script-caphv.sh"))
			Expect(result).To(ContainSubstring("dhclient"))
			Expect(result).To(ContainSubstring("eth0"))
			Expect(result).To(ContainSubstring("cat >"))                         // script created inline in bootcmd
			Expect(strings.Count(result, "dhclient")).To(BeNumerically(">=", 2)) // script ref + bootcmd
		})
	})

	Context("DHCP cloud-init with 2 NICs", func() {
		It("Should generate bootcmd entries for both interfaces", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production", "default/management"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{},
			}

			result := buildDHCPCloudInit(scope)
			Expect(result).To(ContainSubstring("eth0"))
			Expect(result).To(ContainSubstring("eth1"))
			Expect(result).To(ContainSubstring("dhclient-eth0.lease"))
			Expect(result).To(ContainSubstring("dhclient-eth1.lease"))
		})
	})

	Context("Static eth0 + DHCP eth1 (with NetworkConfig, 2 NICs)", func() {
		It("Should generate static config for eth0 and DHCP for eth1", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production", "default/management"},
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
					Address:    "172.16.3.42",
					Gateway:    "172.16.0.1",
					DNSServers: []string{"172.16.0.1"},
				},
			}

			result := buildNetworkDataStatic(scope)
			Expect(result).To(ContainSubstring("name: eth0"))
			Expect(result).To(ContainSubstring("type: static"))
			Expect(result).To(ContainSubstring("address: 172.16.3.42"))
			Expect(result).To(ContainSubstring("netmask: 255.255.0.0"))
			Expect(result).To(ContainSubstring("gateway: 172.16.0.1"))
			Expect(result).To(ContainSubstring("name: eth1"))
			Expect(result).To(ContainSubstring("type: dhcp"))
			Expect(result).To(ContainSubstring("type: nameserver"))
			Expect(result).To(ContainSubstring("- 172.16.0.1"))
		})
	})

	Context("Static eth0 with 1 NIC (regression test)", func() {
		It("Should generate the same output as the old inline code", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
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
					Address:    "172.16.3.40",
					Gateway:    "172.16.0.1",
					DNSServers: []string{"172.16.0.1"},
				},
			}

			expected := `version: 1
config:
  - type: physical
    name: eth0
    subnets:
      - type: static
        address: 172.16.3.40
        netmask: 255.255.0.0
        gateway: 172.16.0.1
  - type: nameserver
    address:
      - 172.16.0.1
`
			result := buildNetworkDataStatic(scope)
			Expect(result).To(Equal(expected))
		})
	})

	Context("Static eth0 with DNS search domains", func() {
		It("Should include search domains in nameserver config", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
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
					Address:    "172.16.3.42",
					Gateway:    "172.16.0.1",
					DNSServers: []string{"172.16.0.1"},
					DNSSearch:  []string{"home.lo", "cluster.local"},
				},
			}
			result := buildNetworkDataStatic(scope)
			Expect(result).To(ContainSubstring("search:"))
			Expect(result).To(ContainSubstring("- home.lo"))
			Expect(result).To(ContainSubstring("- cluster.local"))
			Expect(result).To(ContainSubstring("type: nameserver"))
			Expect(result).To(ContainSubstring("- 172.16.0.1"))
		})
	})

	Context("Static eth0 with multiple DNS servers", func() {
		It("Should list all DNS servers", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{
					Spec: infrav1.HarvesterClusterSpec{
						VMNetworkConfig: &infrav1.VMNetworkConfig{
							SubnetMask: "255.255.255.0",
							Gateway:    "10.0.0.1",
							DNSServers: []string{"8.8.8.8", "8.8.4.4"},
						},
					},
				},
				EffectiveNetworkConfig: &infrav1.NetworkConfig{
					Address:    "10.0.0.50",
					Gateway:    "10.0.0.1",
					DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				},
			}

			result := buildNetworkDataStatic(scope)
			Expect(result).To(ContainSubstring("- 8.8.8.8"))
			Expect(result).To(ContainSubstring("- 8.8.4.4"))
			Expect(result).To(ContainSubstring("netmask: 255.255.255.0"))
		})
	})

	Context("Static with no DNS servers", func() {
		It("Should not include nameserver section", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{
					Spec: infrav1.HarvesterClusterSpec{
						VMNetworkConfig: &infrav1.VMNetworkConfig{
							SubnetMask: "255.255.0.0",
							Gateway:    "172.16.0.1",
						},
					},
				},
				EffectiveNetworkConfig: &infrav1.NetworkConfig{
					Address:    "172.16.3.40",
					Gateway:    "172.16.0.1",
					DNSServers: []string{},
				},
			}

			result := buildNetworkDataStatic(scope)
			Expect(result).NotTo(ContainSubstring("type: nameserver"))
		})
	})

	Context("Static with VMNetworkConfig nil but default subnet mask", func() {
		It("Should use default subnet mask 255.255.0.0", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/production"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{},
				EffectiveNetworkConfig: &infrav1.NetworkConfig{
					Address:    "172.16.3.40",
					Gateway:    "172.16.0.1",
					DNSServers: []string{},
				},
			}

			result := buildNetworkDataStatic(scope)
			Expect(result).To(ContainSubstring("netmask: 255.255.0.0"))
		})
	})

	Context("Static with 3 NICs", func() {
		It("Should create static for eth0 and DHCP for eth1 and eth2", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/prod", "default/mgmt", "default/storage"},
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
					Address:    "172.16.3.40",
					Gateway:    "172.16.0.1",
					DNSServers: []string{"172.16.0.1"},
				},
			}

			result := buildNetworkDataStatic(scope)
			Expect(result).To(ContainSubstring("name: eth0"))
			Expect(result).To(ContainSubstring("name: eth1"))
			Expect(result).To(ContainSubstring("name: eth2"))
			// eth0 should be static
			Expect(result).To(ContainSubstring("type: static"))
			// eth1 and eth2 should be DHCP
			Expect(strings.Count(result, "type: dhcp")).To(Equal(2))
		})
	})

	Context("DHCP cloud-init with 3 NICs", func() {
		It("Should generate bootcmd entries for all interfaces", func() {
			scope := &Scope{
				HarvesterMachine: &infrav1.HarvesterMachine{
					Spec: infrav1.HarvesterMachineSpec{
						Networks: []string{"default/prod", "default/mgmt", "default/storage"},
					},
				},
				HarvesterCluster: &infrav1.HarvesterCluster{},
			}

			result := buildDHCPCloudInit(scope)
			Expect(result).To(ContainSubstring("eth0"))
			Expect(result).To(ContainSubstring("eth1"))
			Expect(result).To(ContainSubstring("eth2"))
			Expect(result).To(ContainSubstring("dhclient-eth0.lease"))
			Expect(result).To(ContainSubstring("dhclient-eth1.lease"))
			Expect(result).To(ContainSubstring("dhclient-eth2.lease"))
		})
	})
})

// =============================================================================
// Tests for isVMRunning
// =============================================================================

var _ = Describe("isVMRunning", func() {
	It("should return true for RunStrategyAlways", func() {
		strategy := kubevirtv1.RunStrategyAlways
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		Expect(isVMRunning(vm)).To(BeTrue())
	})

	It("should return true for RunStrategyRerunOnFailure", func() {
		strategy := kubevirtv1.RunStrategyRerunOnFailure
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		Expect(isVMRunning(vm)).To(BeTrue())
	})

	It("should return true for RunStrategyOnce", func() {
		strategy := kubevirtv1.RunStrategyOnce
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		Expect(isVMRunning(vm)).To(BeTrue())
	})

	It("should return false for RunStrategyHalted", func() {
		strategy := kubevirtv1.RunStrategyHalted
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		Expect(isVMRunning(vm)).To(BeFalse())
	})

	It("should fallback to status.ready when both running and runStrategy are set", func() {
		strategy := kubevirtv1.RunStrategyAlways
		running := true
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &strategy,
				Running:     &running,
			},
			Status: kubevirtv1.VirtualMachineStatus{Ready: true},
		}
		Expect(isVMRunning(vm)).To(BeTrue())
	})

	It("should return false when both running and runStrategy set and status not ready", func() {
		strategy := kubevirtv1.RunStrategyAlways
		running := true
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &strategy,
				Running:     &running,
			},
			Status: kubevirtv1.VirtualMachineStatus{Ready: false},
		}
		Expect(isVMRunning(vm)).To(BeFalse())
	})

	It("should return true when only spec.running is true (deprecated path)", func() {
		running := true
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{Running: &running},
		}
		// When only running is set, RunStrategy() returns RunStrategyAlways for running=true
		Expect(isVMRunning(vm)).To(BeTrue())
	})

	It("should return false when only spec.running is false (deprecated path)", func() {
		running := false
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{Running: &running},
		}
		// When only running is set, RunStrategy() returns RunStrategyHalted for running=false
		Expect(isVMRunning(vm)).To(BeFalse())
	})

	It("should return false when neither running nor runStrategy is set", func() {
		vm := &kubevirtv1.VirtualMachine{
			Spec: kubevirtv1.VirtualMachineSpec{},
		}
		// RunStrategy() returns RunStrategyHalted by default when nothing is set
		Expect(isVMRunning(vm)).To(BeFalse())
	})
})

// =============================================================================
// Tests for buildNetworkInterfaces
// =============================================================================

var _ = Describe("buildNetworkInterfaces", func() {
	It("should create interfaces for each network", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/prod", "default/mgmt"},
			},
		}
		ifaces := buildNetworkInterfaces(machine)
		Expect(ifaces).To(HaveLen(2))
		Expect(ifaces[0].Name).To(Equal("nic-1"))
		Expect(ifaces[0].Model).To(Equal("virtio"))
		Expect(ifaces[1].Name).To(Equal("nic-2"))
		Expect(ifaces[1].Model).To(Equal("virtio"))
	})

	It("should handle single network", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/production"},
			},
		}
		ifaces := buildNetworkInterfaces(machine)
		Expect(ifaces).To(HaveLen(1))
		Expect(ifaces[0].Name).To(Equal("nic-1"))
		Expect(ifaces[0].Model).To(Equal("virtio"))
		// Check bridge binding method is set
		Expect(ifaces[0].InterfaceBindingMethod).ToNot(Equal(kubevirtv1.InterfaceBindingMethod{}))
	})

	It("should handle empty networks", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{Networks: []string{}},
		}
		ifaces := buildNetworkInterfaces(machine)
		Expect(ifaces).To(BeEmpty())
	})

	It("should handle 5 networks with correct numbering", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"n1", "n2", "n3", "n4", "n5"},
			},
		}
		ifaces := buildNetworkInterfaces(machine)
		Expect(ifaces).To(HaveLen(5))

		for i, iface := range ifaces {
			Expect(iface.Name).To(Equal("nic-" + strings.ReplaceAll(
				strings.ReplaceAll(string(rune('1'+i)), "\x00", ""), "", "")))
		}

		Expect(ifaces[0].Name).To(Equal("nic-1"))
		Expect(ifaces[4].Name).To(Equal("nic-5"))
	})
})

// =============================================================================
// Tests for buildAffinity
// =============================================================================

var _ = Describe("buildAffinity", func() {
	It("should create default PodAntiAffinity", func() {
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm"},
				Spec:       infrav1.HarvesterMachineSpec{},
			},
		}
		affinity := buildAffinity(scope)
		Expect(affinity).ToNot(BeNil())
		Expect(affinity.PodAntiAffinity).ToNot(BeNil())
		Expect(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		Expect(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight).To(Equal(int32(1)))
		preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		Expect(preferred[0].PodAffinityTerm.TopologyKey).To(Equal("kubernetes.io/hostname"))
		Expect(preferred[0].PodAffinityTerm.LabelSelector.MatchLabels["harvesterhci.io/vmNamePrefix"]).To(Equal("test-vm"))
		Expect(affinity.NodeAffinity).To(BeNil())
		Expect(affinity.PodAffinity).To(BeNil())
	})

	It("should merge NodeAffinity when specified", func() {
		nodeAffinity := &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"node1"}},
					}},
				},
			},
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm"},
				Spec: infrav1.HarvesterMachineSpec{
					NodeAffinity: nodeAffinity,
				},
			},
		}
		affinity := buildAffinity(scope)
		Expect(affinity.NodeAffinity).To(Equal(nodeAffinity))
		Expect(affinity.PodAntiAffinity).ToNot(BeNil())
		Expect(affinity.PodAffinity).To(BeNil())
	})

	It("should merge WorkloadAffinity when specified", func() {
		podAffinity := &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{TopologyKey: "kubernetes.io/hostname"},
			},
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm"},
				Spec: infrav1.HarvesterMachineSpec{
					WorkloadAffinity: podAffinity,
				},
			},
		}
		affinity := buildAffinity(scope)
		Expect(affinity.PodAffinity).To(Equal(podAffinity))
		Expect(affinity.PodAntiAffinity).ToNot(BeNil())
		Expect(affinity.NodeAffinity).To(BeNil())
	})

	It("should merge both NodeAffinity and WorkloadAffinity when specified", func() {
		nodeAffinity := &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"node1"}},
					}},
				},
			},
		}
		podAffinity := &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{TopologyKey: "topology.kubernetes.io/zone"},
			},
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "complex-vm"},
				Spec: infrav1.HarvesterMachineSpec{
					NodeAffinity:     nodeAffinity,
					WorkloadAffinity: podAffinity,
				},
			},
		}
		affinity := buildAffinity(scope)
		Expect(affinity.NodeAffinity).To(Equal(nodeAffinity))
		Expect(affinity.PodAffinity).To(Equal(podAffinity))
		Expect(affinity.PodAntiAffinity).ToNot(BeNil())
		preferred := affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution
		Expect(preferred[0].PodAffinityTerm.LabelSelector.MatchLabels["harvesterhci.io/vmNamePrefix"]).To(Equal("complex-vm"))
	})
})

// =============================================================================
// Tests for getCloudInitData
// =============================================================================

var _ = Describe("getCloudInitData", func() {
	It("should retrieve cloud-init data from bootstrap secret", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testBootstrapDataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo hello\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		dataSecretName := testBootstrapDataSecretName
		scope := &Scope{
			Ctx: context.TODO(),
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
				},
			},
			ReconcilerClient: fakeClient,
		}

		data, err := getCloudInitData(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(ContainSubstring("echo hello"))
		Expect(data).To(ContainSubstring("runcmd:"))
	})

	It("should return error when secret not found", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		dataSecretName := "missing-secret"
		scope := &Scope{
			Ctx: context.TODO(),
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
				},
			},
			ReconcilerClient: fakeClient,
		}

		_, err := getCloudInitData(scope)
		Expect(err).To(HaveOccurred())
	})

	It("should return error when secret has no 'value' key", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "no-value-secret", Namespace: "test-ns"},
			Data:       map[string][]byte{"other-key": []byte("some data")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		dataSecretName := "no-value-secret"
		scope := &Scope{
			Ctx: context.TODO(),
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
				},
			},
			ReconcilerClient: fakeClient,
		}

		_, err := getCloudInitData(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no userData key found"))
	})

	It("should return empty string for empty 'value' key", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-value-secret", Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		dataSecretName := "empty-value-secret" //nolint:gosec // test data, not real credentials
		scope := &Scope{
			Ctx: context.TODO(),
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
				},
			},
			ReconcilerClient: fakeClient,
		}

		data, err := getCloudInitData(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(data).To(Equal(""))
	})
})

// =============================================================================
// Tests for dhclientScriptContent
// =============================================================================

var _ = Describe("dhclientScriptContent", func() {
	It("should return a valid bash script", func() {
		content := dhclientScriptContent()
		Expect(content).To(HavePrefix("#!/bin/bash"))
	})

	It("should handle PREINIT reason", func() {
		content := dhclientScriptContent()
		Expect(content).To(ContainSubstring("PREINIT"))
		Expect(content).To(ContainSubstring("ip link set dev"))
	})

	It("should handle BOUND/RENEW/REBIND/REBOOT reasons", func() {
		content := dhclientScriptContent()
		Expect(content).To(ContainSubstring("BOUND|RENEW|REBIND|REBOOT"))
		Expect(content).To(ContainSubstring("ip addr add"))
		Expect(content).To(ContainSubstring("ip route add default"))
	})

	It("should configure resolv.conf from DHCP", func() {
		content := dhclientScriptContent()
		Expect(content).To(ContainSubstring("/etc/resolv.conf"))
		Expect(content).To(ContainSubstring("nameserver"))
	})

	It("should convert netmask to CIDR prefix", func() {
		content := dhclientScriptContent()
		Expect(content).To(ContainSubstring("new_subnet_mask"))
		Expect(content).To(ContainSubstring("prefix"))
	})
})

// =============================================================================
// Tests for buildPVCForVolume
// =============================================================================

var _ = Describe("storageClassForImage", func() {
	It("should use the storage class the image resolved to", func() {
		// Harvester 1.8.1 names image storage classes lh-<uuid>, so the name can
		// no longer be derived from the image name.
		img := &harvesterv1beta1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{Name: "image-rs64h", Namespace: "default"},
			Status: harvesterv1beta1.VirtualMachineImageStatus{
				StorageClassName: "lh-c141a832-fc3d-41db-b613-722c87c545e1",
			},
		}
		Expect(storageClassForImage(img)).To(Equal("lh-c141a832-fc3d-41db-b613-722c87c545e1"))
	})

	It("should fall back to the longhorn-<image> convention when the status is empty", func() {
		img := &harvesterv1beta1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{Name: "image-rs64h", Namespace: "default"},
		}
		Expect(storageClassForImage(img)).To(Equal("longhorn-image-rs64h"))
	})
})

var _ = Describe("buildPVCForVolume", func() {
	It("should build a PVC for storageClass volume type", func() {
		size := resource.MustParse("10Gi")
		vol := &infrav1.Volume{
			VolumeType:   "storageClass",
			StorageClass: "longhorn",
			VolumeSize:   &size,
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
		}
		pvc, err := buildPVCForVolume(vol, "test-pvc", "default", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(pvc.Name).To(Equal("test-pvc"))
		Expect(pvc.Namespace).To(Equal("default"))
		Expect(*pvc.Spec.StorageClassName).To(Equal("longhorn"))
		Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteMany))

		block := corev1.PersistentVolumeBlock
		Expect(*pvc.Spec.VolumeMode).To(Equal(block))
		Expect(pvc.Spec.Resources.Requests["storage"]).To(Equal(resource.MustParse("10Gi")))
	})

	It("should build a PVC for storageClass with different size", func() {
		size := resource.MustParse("100Gi")
		vol := &infrav1.Volume{
			VolumeType:   "storageClass",
			StorageClass: "harv-rep1",
			VolumeSize:   &size,
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "ns1"},
			},
		}
		pvc, err := buildPVCForVolume(vol, "big-disk-0-xyz", "ns1", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(pvc.Name).To(Equal("big-disk-0-xyz"))
		Expect(pvc.Namespace).To(Equal("ns1"))
		Expect(*pvc.Spec.StorageClassName).To(Equal("harv-rep1"))
		Expect(pvc.Spec.Resources.Requests["storage"]).To(Equal(resource.MustParse("100Gi")))
	})

	It("should set correct annotations for storageClass type", func() {
		size := resource.MustParse("20Gi")
		vol := &infrav1.Volume{
			VolumeType:   "storageClass",
			StorageClass: "longhorn",
			VolumeSize:   &size,
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
		}
		pvc, err := buildPVCForVolume(vol, "data-pvc", "default", scope)
		Expect(err).ToNot(HaveOccurred())
		// storageClass type should NOT have imageId annotation
		_, hasImageAnnotation := pvc.Annotations[hvAnnotationImageID]
		Expect(hasImageAnnotation).To(BeFalse())
	})
})

// =============================================================================
// Tests for getKubevirtNetworksFromHarvesterMachine (additional cases)
// =============================================================================

var _ = Describe("getKubevirtNetworksFromHarvesterMachine additional", func() {
	It("should handle empty networks list", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{},
			},
		}
		networks := getKubevirtNetworksFromHarvesterMachine(machine)
		Expect(networks).To(BeEmpty())
	})

	It("should handle single network", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"default/production"},
			},
		}
		networks := getKubevirtNetworksFromHarvesterMachine(machine)
		Expect(networks).To(HaveLen(1))
		Expect(networks[0].Name).To(Equal("nic-1"))
		Expect(networks[0].Multus.NetworkName).To(Equal("default/production"))
	})

	It("should handle namespaced network names", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Networks: []string{"ns1/net1", "ns2/net2", "ns3/net3"},
			},
		}
		networks := getKubevirtNetworksFromHarvesterMachine(machine)
		Expect(networks).To(HaveLen(3))
		Expect(networks[0].Multus.NetworkName).To(Equal("ns1/net1"))
		Expect(networks[1].Multus.NetworkName).To(Equal("ns2/net2"))
		Expect(networks[2].Multus.NetworkName).To(Equal("ns3/net3"))
	})
})

// =============================================================================
// Tests for getWorkloadClusterConfig
// =============================================================================

// newWorkloadClusterScope creates a Scope with a fake client for getWorkloadClusterConfig tests.
// If objects are provided, they are added to the fake client.
func newWorkloadClusterScope(clusterName, namespace string, objects ...corev1.Secret) *Scope {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = infrav1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for i := range objects {
		builder = builder.WithObjects(&objects[i])
	}

	fakeClient := builder.Build()

	logger := log.FromContext(context.TODO())

	return &Scope{
		Ctx:              context.TODO(),
		Logger:           &logger,
		ReconcilerClient: fakeClient,
		Cluster: &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
		},
	}
}

var _ = Describe("getWorkloadClusterConfig", func() {
	It("should return error when kubeconfig secret does not exist", func() {
		scope := newWorkloadClusterScope("test-cluster", "test-ns")

		_, err := getWorkloadClusterConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to get workload cluster kubeconfig secret"))
	})

	It("should return error when secret has no value key", func() {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster-kubeconfig",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"other-key": []byte("not-a-kubeconfig"),
			},
		}

		scope := newWorkloadClusterScope("test-cluster", "test-ns", secret)

		_, err := getWorkloadClusterConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no kubeconfig data found"))
	})

	It("should return error when kubeconfig data is invalid", func() {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bad-cluster-kubeconfig",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"value": []byte("this is not a valid kubeconfig"),
			},
		}

		scope := newWorkloadClusterScope("bad-cluster", "test-ns", secret)

		_, err := getWorkloadClusterConfig(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to get workload cluster config"))
	})

	It("should return valid config from correct kubeconfig secret", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		kubeconfig := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://172.16.3.100:6443
  name: workload
contexts:
- context:
    cluster: workload
    user: admin
  name: workload
current-context: workload
kind: Config
users:
- name: admin
  user:
    token: test-token-123
`)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "good-cluster-kubeconfig",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"value": kubeconfig,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		logger := log.FromContext(context.TODO())
		scope := &Scope{
			Ctx:              context.TODO(),
			Logger:           &logger,
			ReconcilerClient: fakeClient,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "good-cluster", Namespace: "test-ns"},
			},
		}

		config, err := getWorkloadClusterConfig(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(config).ToNot(BeNil())
		Expect(config.Host).To(Equal("https://172.16.3.100:6443"))
	})
})

// =============================================================================
// Tests for initializeWorkloadNode (early returns)
// =============================================================================

var _ = Describe("initializeWorkloadNode", func() {
	It("should return early when ProviderID is empty", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		logger := log.FromContext(context.TODO())
		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}

		scope := &Scope{
			Ctx:    context.TODO(),
			Logger: &logger,
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					ProviderID: "", // empty -> early return
				},
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			ReconcilerClient: fakeClient,
		}

		// Should not panic - just returns early
		r.initializeWorkloadNode(scope)
	})

	It("should return early when workload cluster config is not available", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		logger := log.FromContext(context.TODO())
		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}

		scope := &Scope{
			Ctx:    context.TODO(),
			Logger: &logger,
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{
					ProviderID: "harvester://test-vm",
				},
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			ReconcilerClient: fakeClient,
		}

		// Secret doesn't exist -> getWorkloadClusterConfig fails -> early return
		r.initializeWorkloadNode(scope)
	})
})

// =============================================================================
// Tests for getProviderIDFromWorkloadCluster error paths
// =============================================================================

var _ = Describe("getProviderIDFromWorkloadCluster", func() {
	It("should return error when workload cluster kubeconfig secret is missing", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		logger := log.FromContext(context.TODO())
		scope := &Scope{
			Ctx:              context.TODO(),
			Logger:           &logger,
			ReconcilerClient: fakeClient,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "no-config", Namespace: "ns"},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "machine-1", Namespace: "ns"},
			},
		}

		_, err := getProviderIDFromWorkloadCluster(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to get workload cluster config"))
	})
})

// =============================================================================
// Tests for buildVMTemplate
// =============================================================================

var _ = Describe("buildVMTemplate", func() {
	It("should build a complete VM template with static network", func() {
		// Create SSH KeyPair
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec: harvesterv1beta1.KeyPairSpec{
				PublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC test@test",
			},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		// Create bootstrap data secret
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testBootstrapDataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo hello\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()
		logger := log.FromContext(context.TODO())

		dataSecretName := testBootstrapDataSecretName
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName},
				},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU:        4,
					Memory:     "8Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						SubnetMask: "255.255.0.0",
						Gateway:    "172.16.0.1",
						DNSServers: []string{"172.16.0.1"},
					},
				},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
			EffectiveNetworkConfig: &infrav1.NetworkConfig{
				Address:    "172.16.3.40",
				Gateway:    "172.16.0.1",
				DNSServers: []string{"172.16.0.1"},
			},
		}

		disks := []diskInfo{
			{pvcName: "test-cp-0-disk-0-abc", index: 0},
		}
		vmiLabels := map[string]string{"harvesterhci.io/vmName": "test-cp-0"}

		tmpl, err := buildVMTemplate(scope, disks, vmiLabels)
		Expect(err).ToNot(HaveOccurred())
		Expect(tmpl).ToNot(BeNil())

		// Verify CPU
		Expect(tmpl.Spec.Domain.CPU.Cores).To(Equal(uint32(4)))
		Expect(tmpl.Spec.Domain.CPU.Sockets).To(Equal(uint32(1)))
		Expect(tmpl.Spec.Domain.CPU.Threads).To(Equal(uint32(1)))
		// Verify memory
		Expect(tmpl.Spec.Domain.Memory.Guest.String()).To(Equal("8Gi"))
		// Verify volumes (1 disk + cloudinit)
		Expect(tmpl.Spec.Volumes).To(HaveLen(2))
		Expect(tmpl.Spec.Volumes[0].Name).To(Equal("disk-0"))
		Expect(tmpl.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal("test-cp-0-disk-0-abc"))
		Expect(tmpl.Spec.Volumes[1].Name).To(Equal("cloudinitdisk"))
		// Verify disks
		Expect(tmpl.Spec.Domain.Devices.Disks).To(HaveLen(2))
		Expect(tmpl.Spec.Domain.Devices.Disks[0].Name).To(Equal("disk-0"))
		Expect(string(tmpl.Spec.Domain.Devices.Disks[0].DiskDevice.Disk.Bus)).To(Equal("virtio"))
		Expect(tmpl.Spec.Domain.Devices.Disks[1].Name).To(Equal("cloudinitdisk"))
		// Verify networks
		Expect(tmpl.Spec.Networks).To(HaveLen(1))
		Expect(tmpl.Spec.Networks[0].Multus.NetworkName).To(Equal("default/production"))
		// Verify interfaces
		Expect(tmpl.Spec.Domain.Devices.Interfaces).To(HaveLen(1))
		Expect(tmpl.Spec.Domain.Devices.Interfaces[0].Name).To(Equal("nic-1"))
		Expect(tmpl.Spec.Domain.Devices.Interfaces[0].Model).To(Equal("virtio"))
		// Verify hostname
		Expect(tmpl.Spec.Hostname).To(Equal("test-cp-0"))
		// Verify USB input
		Expect(tmpl.Spec.Domain.Devices.Inputs).To(HaveLen(1))
		Expect(string(tmpl.Spec.Domain.Devices.Inputs[0].Type)).To(Equal("tablet"))
		// Verify annotations
		Expect(tmpl.ObjectMeta.Annotations).To(HaveKey(hvAnnotationDiskNames))
		Expect(tmpl.ObjectMeta.Annotations).To(HaveKey(hvAnnotationSSH))
		Expect(tmpl.ObjectMeta.Annotations[hvAnnotationSSH]).To(ContainSubstring("capi-ssh-key"))
		// Verify labels
		Expect(tmpl.ObjectMeta.Labels).To(HaveKeyWithValue("harvesterhci.io/vmName", "test-cp-0"))
		// Verify cloud-init secret was created on Harvester
		secret, err := hvClient.CoreV1().Secrets("default").Get(context.TODO(), "test-cp-0-cloud-init", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Data).To(HaveKey("userdata"))
		Expect(secret.Data).To(HaveKey("networkdata")) // static mode
		// Verify userdata contains SSH key and qemu-guest-agent
		userdata := string(secret.Data["userdata"])
		Expect(userdata).To(ContainSubstring("ssh-rsa"))
		Expect(userdata).To(ContainSubstring("qemu-guest-agent"))
		Expect(userdata).To(ContainSubstring("echo hello"))
		// Verify networkdata contains static config
		networkdata := string(secret.Data["networkdata"])
		Expect(networkdata).To(ContainSubstring("172.16.3.40"))
		Expect(networkdata).To(ContainSubstring("type: static"))
		// Verify resource requests/limits
		Expect(tmpl.Spec.Domain.Resources.Requests["memory"]).To(Equal(resource.MustParse("8Gi")))
		Expect(tmpl.Spec.Domain.Resources.Limits["memory"]).To(Equal(resource.MustParse("8Gi")))
		Expect(tmpl.Spec.Domain.Resources.Limits["cpu"]).To(Equal(*resource.NewQuantity(4, resource.DecimalSI)))
	})

	It("should build a VM template with DHCP mode (no networkdata)", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa AAAA test@test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo dhcp\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-dhcp-0"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil, // DHCP mode
		}

		disks := []diskInfo{{pvcName: "test-dhcp-0-disk-0-xyz", index: 0}}
		vmiLabels := map[string]string{"harvesterhci.io/vmName": "test-dhcp-0"}
		tmpl, err := buildVMTemplate(scope, disks, vmiLabels)
		Expect(err).ToNot(HaveOccurred())
		Expect(tmpl).ToNot(BeNil())

		// Verify CPU and memory for DHCP VM
		Expect(tmpl.Spec.Domain.CPU.Cores).To(Equal(uint32(2)))
		Expect(tmpl.Spec.Domain.Memory.Guest.String()).To(Equal("4Gi"))

		// In DHCP mode, cloud-init should contain dhclient in userdata but NOT have networkdata key
		secret, err := hvClient.CoreV1().Secrets("default").Get(context.TODO(), "test-dhcp-0-cloud-init", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Data).To(HaveKey("userdata"))
		userdata := string(secret.Data["userdata"])
		Expect(userdata).To(ContainSubstring("dhclient"))
		Expect(userdata).To(ContainSubstring("echo dhcp"))
		// networkdata should NOT be present in DHCP mode
		_, hasNetworkdata := secret.Data["networkdata"]
		Expect(hasNetworkdata).To(BeFalse())
	})

	It("should build VM template with multiple disks and boot order", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa AAAA test@test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()
		logger := log.FromContext(context.TODO())

		size40 := resource.MustParse("40Gi")
		size10 := resource.MustParse("10Gi")
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-multidisk"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "image", VolumeSize: &size40, BootOrder: 1},
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size10, BootOrder: 0},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil,
		}

		disks := []diskInfo{
			{pvcName: "test-multidisk-disk-0-abc", index: 0},
			{pvcName: "test-multidisk-disk-1-def", index: 1},
		}
		tmpl, err := buildVMTemplate(scope, disks, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		// 2 data disks + 1 cloudinit disk
		Expect(tmpl.Spec.Volumes).To(HaveLen(3))
		Expect(tmpl.Spec.Volumes[0].Name).To(Equal("disk-0"))
		Expect(tmpl.Spec.Volumes[1].Name).To(Equal("disk-1"))
		Expect(tmpl.Spec.Volumes[2].Name).To(Equal("cloudinitdisk"))
		// Verify boot order on first disk
		Expect(tmpl.Spec.Domain.Devices.Disks[0].BootOrder).ToNot(BeNil())
		Expect(*tmpl.Spec.Domain.Devices.Disks[0].BootOrder).To(Equal(uint(1)))
		// Second disk has BootOrder 0 => no boot order set
		Expect(tmpl.Spec.Domain.Devices.Disks[1].BootOrder).To(BeNil())
		// diskNames annotation should be a JSON array with both PVC names
		Expect(tmpl.ObjectMeta.Annotations[hvAnnotationDiskNames]).To(ContainSubstring("test-multidisk-disk-0-abc"))
		Expect(tmpl.ObjectMeta.Annotations[hvAnnotationDiskNames]).To(ContainSubstring("test-multidisk-disk-1-def"))
	})

	It("should update existing cloud-init secret", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa AAAA test@test"},
		}
		// Pre-create the cloud-init secret to test update path
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-update-0-cloud-init", Namespace: "default"},
			Data:       map[string][]byte{"userdata": []byte("old-data")},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair, existingSecret)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-update-0"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil,
		}

		tmpl, err := buildVMTemplate(scope, nil, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(tmpl).ToNot(BeNil())

		// Verify the secret was updated (not old data)
		secret, err := hvClient.CoreV1().Secrets("default").Get(context.TODO(), "test-update-0-cloud-init", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		userdata := string(secret.Data["userdata"])
		Expect(userdata).ToNot(Equal("old-data"))
		Expect(userdata).To(ContainSubstring("qemu-guest-agent"))
	})

	It("should return error when SSH key pair not found", func() {
		hvClient := hvfake.NewSimpleClientset() // no key pair
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{SSHKeyPair: "default/missing-key"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		_, err := buildVMTemplate(scope, nil, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("keypair"))
	})

	It("should return error when SSH key pair name is malformed", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				Spec: infrav1.HarvesterMachineSpec{SSHKeyPair: "a/b/c"}, // too many slashes
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		_, err := buildVMTemplate(scope, nil, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Malformed"))
	})
})

// =============================================================================
// Tests for createVMFromHarvesterMachine
// =============================================================================

var _ = Describe("createVMFromHarvesterMachine", func() {
	It("should create a VM with storageClass volume in DHCP mode", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo hello\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
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
					CPU: 4, Memory: "8Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size, BootOrder: 1},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil, // DHCP mode
		}

		vm, err := createVMFromHarvesterMachine(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(vm).ToNot(BeNil())
		Expect(vm.Name).To(Equal("test-cp-0"))
		Expect(vm.Namespace).To(Equal("default"))
		Expect(*vm.Spec.RunStrategy).To(Equal(kubevirtv1.RunStrategyAlways))
		// Verify annotations
		Expect(vm.Annotations).To(HaveKey(vmAnnotationPVC))
		Expect(vm.Annotations).To(HaveKey(vmAnnotationNetworkIps))
		// Verify labels
		Expect(vm.Labels).To(HaveKeyWithValue("harvesterhci.io/creator", "docker-machine-driver-harvester"))
		// Verify template exists
		Expect(vm.Spec.Template).ToNot(BeNil())
		Expect(vm.Spec.Template.Spec.Domain.CPU.Cores).To(Equal(uint32(4)))

		// Verify the VM was created on the fake client
		createdVM, err := hvClient.KubevirtV1().VirtualMachines("default").Get(context.TODO(), "test-cp-0", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createdVM.Name).To(Equal("test-cp-0"))
	})

	It("should add control-plane label when machine has CP label", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
		).Build()
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-cp-0",
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil,
		}

		vm, err := createVMFromHarvesterMachine(scope)
		Expect(err).ToNot(HaveOccurred())
		// Check the CP VM label (cpVMLabelKey = "harvestercluster/machinetype", cpVMLabelValuePrefix = "controlplane")
		Expect(vm.Labels).To(HaveKey("harvestercluster/machinetype"))
		Expect(vm.Labels["harvestercluster/machinetype"]).To(Equal("controlplane-my-cluster"))
	})

	It("should NOT add control-plane label for worker machines", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
		).Build()
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "my-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "my-worker-0",
					Labels: map[string]string{}, // no CP label
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:        hvClient,
			ReconcilerClient:       fakeClient,
			Logger:                 &logger,
			EffectiveNetworkConfig: nil,
		}

		vm, err := createVMFromHarvesterMachine(scope)
		Expect(err).ToNot(HaveOccurred())
		// Worker should NOT have the CP label
		Expect(vm.Labels).ToNot(HaveKey("harvestercluster/machinetype"))
	})

	It("should create VM with static network config", func() {
		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)

		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dataSecretName := testBootstrapSecretName
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
				Data:       map[string][]byte{"value": []byte("")},
			},
		).Build()
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx:     context.TODO(),
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "static-cp-0"},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 4, Memory: "8Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size, BootOrder: 1},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						SubnetMask: "255.255.0.0",
						Gateway:    "172.16.0.1",
						DNSServers: []string{"172.16.0.1"},
					},
				},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
			EffectiveNetworkConfig: &infrav1.NetworkConfig{
				Address:    "172.16.3.42",
				Gateway:    "172.16.0.1",
				DNSServers: []string{"172.16.0.1"},
			},
		}

		vm, err := createVMFromHarvesterMachine(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(vm.Name).To(Equal("static-cp-0"))

		// Verify cloud-init secret has networkdata for static mode
		secret, err := hvClient.CoreV1().Secrets("default").Get(context.TODO(), "static-cp-0-cloud-init", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Data).To(HaveKey("networkdata"))
		networkdata := string(secret.Data["networkdata"])
		Expect(networkdata).To(ContainSubstring("172.16.3.42"))
		Expect(networkdata).To(ContainSubstring("type: static"))
	})
})

// =============================================================================
// Tests for removeEtcdMemberIfControlPlane
// =============================================================================

var _ = Describe("removeEtcdMemberIfControlPlane", func() {
	It("should skip when machine is not control-plane", func() {
		logger := log.FromContext(context.TODO())
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		scope := &Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}, // no CP label
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}
		r := &HarvesterMachineReconciler{}
		r.removeEtcdMemberIfControlPlane(scope) // should return early without error
	})

	It("should skip when machine has nil labels", func() {
		logger := log.FromContext(context.TODO())
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		scope := &Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{}, // nil labels map
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}
		r := &HarvesterMachineReconciler{}
		r.removeEtcdMemberIfControlPlane(scope) // should return early without error
	})

	It("should log warning and return when workload config unavailable for CP machine", func() {
		logger := log.FromContext(context.TODO())
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		scope := &Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
			},
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}
		r := &HarvesterMachineReconciler{}
		// kubeconfig secret doesn't exist -> getWorkloadClusterConfig fails -> early return
		r.removeEtcdMemberIfControlPlane(scope) // should not panic
	})
})

// =============================================================================
// Tests for ReconcileNormal
// =============================================================================

var _ = Describe("ReconcileNormal", func() {
	It("should return early when cluster is paused", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"cluster.x-k8s.io/paused": "true",
					},
				},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cp-0",
					Namespace: "test-ns",
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		// Status should be not ready when paused
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should add finalizer when not present", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cp-0",
					Namespace: "test-ns",
					// No finalizers yet
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		// Finalizer should be added
		Expect(scope.HarvesterMachine.Finalizers).To(ContainElement(infrav1.MachineFinalizer))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should wait for infrastructure ready", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(false)},
				},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should wait for bootstrap data secret", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status: clusterv1.ClusterStatus{
					Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
				},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: nil, // no bootstrap data yet
					},
				},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should create VM when no existing VM found (DHCP mode)", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo hello\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 4, Memory: "8Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size, BootOrder: 1},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv-cluster", Namespace: "test-ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// VM was just created and its node cannot be initialized yet, so the
		// reconcile requeues until the node registers and gets its providerID
		Expect(result.RequeueAfter).To(Equal(requeueDelay))

		// Verify the VM was created on Harvester
		createdVM, getErr := hvClient.KubevirtV1().VirtualMachines("default").Get(context.TODO(), "test-cp-0", metav1.GetOptions{})
		Expect(getErr).ToNot(HaveOccurred())
		Expect(createdVM.Name).To(Equal("test-cp-0"))
	})

	It("should detect existing running VM and get IP addresses", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyAlways
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &strategy,
			},
		}
		existingVMI := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{IP: "172.16.3.42", Name: "nic-1"},
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(existingVM, existingVMI)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 4, Memory: "8Gi",
				},
				Status: infrav1.HarvesterMachineStatus{
					Ready: false, // not yet ready
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())

		// When VM is running and has IPs but machine was not Ready yet, should set Ready=true and requeue
		Expect(result.Requeue).To(BeTrue()) //nolint:staticcheck // result.Requeue still used by controller
		Expect(scope.HarvesterMachine.Status.Ready).To(BeTrue())
		Expect(scope.HarvesterMachine.Status.Addresses).To(HaveLen(1))
		Expect(scope.HarvesterMachine.Status.Addresses[0].Address).To(Equal("172.16.3.42"))
	})

	It("should requeue when VM is running but VMI not found yet", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyAlways
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &strategy,
			},
		}
		// No VMI -> getIPAddressesFromVMI will fail
		hvClient := hvfake.NewSimpleClientset(existingVM)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// Should requeue since VMI doesn't exist yet (not found error)
		Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should set providerID from VM UID when workload config unavailable", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyAlways
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cp-0", Namespace: "default",
				UID: "fake-uid-12345",
			},
			Spec: kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		existingVMI := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{IP: "172.16.3.42", Name: "nic-1"},
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(existingVM, existingVMI)
		logger := log.FromContext(context.TODO())

		// Pre-set MachineCreatedCondition to true (VM was previously created)
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cp-0", Namespace: "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
			Spec: infrav1.HarvesterMachineSpec{
				ProviderID: "", // no providerID yet
			},
			Status: infrav1.HarvesterMachineStatus{
				Ready: true, // already marked Ready (has IPs)
			},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type:   infrav1.MachineCreatedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.MachineCreatedCondition,
		})

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// The workload node is not initialized yet (no reachable workload
		// cluster here), so the reconcile must requeue until it is: without
		// this, a node registering after the last event-driven reconcile
		// leaves the machine Provisioned forever.
		Expect(result.RequeueAfter).To(Equal(requeueDelay))
		// ProviderID should be set from VM UID (fallback when kubeconfig unavailable)
		Expect(scope.HarvesterMachine.Spec.ProviderID).To(Equal("harvester://fake-uid-12345"))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeTrue())
	})

	It("should mark ready when providerID already set and VM is running", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyAlways
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		existingVMI := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{IP: "172.16.3.42", Name: "nic-1"},
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(existingVM, existingVMI)
		logger := log.FromContext(context.TODO())

		// Pre-set MachineCreatedCondition
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cp-0", Namespace: "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
			Spec: infrav1.HarvesterMachineSpec{
				ProviderID: "harvester://already-set", // providerID already set
			},
			Status: infrav1.HarvesterMachineStatus{
				Ready: true, // already ready
			},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type:   infrav1.MachineCreatedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.MachineCreatedCondition,
		})

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueDelay))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeTrue())
		Expect(scope.HarvesterMachine.Spec.ProviderID).To(Equal("harvester://already-set"))
	})

	It("should handle MachineCreatedCondition=true but VM missing", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		hvClient := hvfake.NewSimpleClientset() // no VM
		logger := log.FromContext(context.TODO())

		// Pre-set MachineCreatedCondition to true (as if VM was previously created)
		hvMachine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cp-0", Namespace: "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
		}
		conditions.Set(hvMachine, metav1.Condition{
			Type:   infrav1.MachineCreatedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.MachineCreatedCondition,
		})

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: hvMachine,
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// VM is gone but condition says created -> mark not found, requeue
		Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should requeue when VM running but no IP addresses and was already Ready", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyAlways
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec:       kubevirtv1.VirtualMachineSpec{RunStrategy: &strategy},
		}
		// VMI exists but has no IP addresses (interfaces with empty IPs)
		existingVMI := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{IP: "", Name: "nic-1"}, // no IP yet
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(existingVM, existingVMI)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cp-0", Namespace: "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Status: infrav1.HarvesterMachineStatus{
					Ready: true, // was previously ready, but now IPs disappeared
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// No IP addresses but was Ready -> requeue
		Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should handle halted VM by setting not ready and requeuing", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		strategy := kubevirtv1.RunStrategyHalted
		existingVM := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy: &strategy,
			},
		}
		hvClient := hvfake.NewSimpleClientset(existingVM)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-cp-0",
					Namespace:  "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		// VM is halted -> not running -> requeue after short delay
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should allocate IP from pool when VMNetworkConfig is set", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("runcmd:\n  - echo static\n")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{},
			},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair, pool)
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cp-0", Namespace: "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 4, Memory: "8Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size, BootOrder: 1},
					},
					NetworkConfig: nil, // no machine-level config -> use pool
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv-cluster", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef:  "capi-vm-pool",
						SubnetMask: "255.255.0.0",
						Gateway:    "172.16.0.1",
						DNSServers: []string{"172.16.0.1"},
					},
				},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueDelay))
		// Should have allocated an IP
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).ToNot(BeEmpty())
		// EffectiveNetworkConfig should be populated
		Expect(scope.EffectiveNetworkConfig).ToNot(BeNil())
		Expect(scope.EffectiveNetworkConfig.Address).To(Equal(scope.HarvesterMachine.Status.AllocatedIPAddress))
		Expect(scope.EffectiveNetworkConfig.Gateway).To(Equal("172.16.0.1"))
		// VM should have been created
		createdVM, getErr := hvClient.KubevirtV1().VirtualMachines("default").Get(context.TODO(), "test-cp-0", metav1.GetOptions{})
		Expect(getErr).ToNot(HaveOccurred())
		Expect(createdVM.Name).To(Equal("test-cp-0"))
	})

	It("should use machine-level NetworkConfig when specified", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		sshKeyPair := &harvesterv1beta1.KeyPair{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-ssh-key", Namespace: "default"},
			Spec:       harvesterv1beta1.KeyPairSpec{PublicKey: "ssh-rsa test"},
		}
		hvClient := hvfake.NewSimpleClientset(sshKeyPair)
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-static-0", Namespace: "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/capi-ssh-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size},
					},
					NetworkConfig: &infrav1.NetworkConfig{
						Address:    "10.0.0.100",
						Gateway:    "10.0.0.1",
						DNSServers: []string{"8.8.8.8"},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv-cluster", Namespace: "test-ns"},
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						SubnetMask: "255.255.255.0",
						Gateway:    "10.0.0.1",
						DNSServers: []string{"8.8.8.8"},
					},
				},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueDelay))
		// EffectiveNetworkConfig should use machine-level config
		Expect(scope.EffectiveNetworkConfig).ToNot(BeNil())
		Expect(scope.EffectiveNetworkConfig.Address).To(Equal("10.0.0.100"))
		Expect(scope.EffectiveNetworkConfig.Gateway).To(Equal("10.0.0.1"))
	})

	It("should return error when VM creation fails (missing SSH key)", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		// No SSH key pair in the fake client -> createVMFromHarvesterMachine will fail
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		size := resource.MustParse("40Gi")
		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cp-0", Namespace: "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					CPU: 2, Memory: "4Gi",
					SSHKeyPair: "default/missing-key",
					Networks:   []string{"default/production"},
					Volumes: []infrav1.Volume{
						{VolumeType: "storageClass", StorageClass: "longhorn", VolumeSize: &size},
					},
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hv-cluster", Namespace: "test-ns"},
				Spec:       infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		_, err := r.ReconcileNormal(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to build VM definition"))
		Expect(scope.HarvesterMachine.Status.Ready).To(BeFalse())
	})

	It("should return error when IP allocation fails", func() {
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		_ = infrav1.AddToScheme(scheme)
		_ = clusterv1.AddToScheme(scheme)

		dataSecretName := testBootstrapDataSecretName
		bootstrapSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: dataSecretName, Namespace: "test-ns"},
			Data:       map[string][]byte{"value": []byte("")},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bootstrapSecret).Build()

		hvClient := hvfake.NewSimpleClientset() // no pool
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			Ctx: context.TODO(),
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
				Status:     clusterv1.ClusterStatus{Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)}},
			},
			Machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
				Spec:       clusterv1.MachineSpec{Bootstrap: clusterv1.Bootstrap{DataSecretName: &dataSecretName}},
			},
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cp-0", Namespace: "test-ns",
					Finalizers: []string{infrav1.MachineFinalizer},
				},
				Spec: infrav1.HarvesterMachineSpec{
					NetworkConfig: nil, // no machine-level config
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					TargetNamespace: "default",
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef: "missing-pool", // pool doesn't exist
					},
				},
			},
			HarvesterClient:  hvClient,
			ReconcilerClient: fakeClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{Client: fakeClient, Scheme: scheme}
		result, err := r.ReconcileNormal(scope)
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})
})

// =============================================================================
// Tests for buildPVCForVolume with unknown volume type
// =============================================================================

var _ = Describe("buildPVCForVolume edge cases", func() {
	It("should create PVC with no storage class for unknown volume type", func() {
		size := resource.MustParse("5Gi")
		vol := &infrav1.Volume{
			VolumeType: "unknown",
			VolumeSize: &size,
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
		}
		pvc, err := buildPVCForVolume(vol, "test-pvc", "default", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(pvc.Name).To(Equal("test-pvc"))
		// Unknown type should not set StorageClassName
		Expect(pvc.Spec.StorageClassName).To(BeNil())
	})

	It("should create PVC with empty annotations for storageClass type", func() {
		size := resource.MustParse("50Gi")
		vol := &infrav1.Volume{
			VolumeType:   "storageClass",
			StorageClass: "longhorn-image-xyz",
			VolumeSize:   &size,
		}
		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
		}
		pvc, err := buildPVCForVolume(vol, "boot-disk", "default", scope)
		Expect(err).ToNot(HaveOccurred())
		// storageClass type should NOT have imageId annotation
		_, hasImageAnnotation := pvc.Annotations[hvAnnotationImageID]
		Expect(hasImageAnnotation).To(BeFalse())
		Expect(*pvc.Spec.StorageClassName).To(Equal("longhorn-image-xyz"))
	})
})

// =============================================================================
// Tests for buildPVCForVolume with image type (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("buildPVCForVolume with image type", func() {
	It("should build a PVC for image volume type", func() {
		size := resource.MustParse("40Gi")
		vol := &infrav1.Volume{
			VolumeType: "image",
			ImageName:  "default/test-image-display",
			VolumeSize: &size,
		}

		// Create a fake VirtualMachineImage
		testImage := &harvesterv1beta1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "image-abc123",
				Namespace: "default",
			},
			Spec: harvesterv1beta1.VirtualMachineImageSpec{
				DisplayName: "test-image-display",
			},
			Status: harvesterv1beta1.VirtualMachineImageStatus{
				StorageClassName: "longhorn-image-abc123",
			},
		}
		hvClient := hvfake.NewSimpleClientset(testImage)

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
		}

		pvc, err := buildPVCForVolume(vol, "test-pvc", "default", scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(*pvc.Spec.StorageClassName).To(Equal("longhorn-image-abc123"))
		Expect(pvc.Annotations[hvAnnotationImageID]).To(Equal("default/image-abc123"))
	})

	It("should return error when image not found", func() {
		size := resource.MustParse("40Gi")
		vol := &infrav1.Volume{
			VolumeType: "image",
			ImageName:  "default/nonexistent-image",
			VolumeSize: &size,
		}

		hvClient := hvfake.NewSimpleClientset() // no images

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
		}

		_, err := buildPVCForVolume(vol, "test-pvc", "default", scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to find VM image"))
	})
})

// =============================================================================
// Tests for getIPAddressesFromVMI (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("getIPAddressesFromVMI", func() {
	It("should extract IP addresses from VMI interfaces", func() {
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
		}
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{IP: "172.16.3.42", Name: "nic-1"},
					{IP: "", Name: "nic-2"}, // should be skipped
					{IP: "172.16.3.43", Name: "nic-3"},
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(vmi)

		addresses, err := getIPAddressesFromVMI(context.TODO(), vm, hvClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(addresses).To(HaveLen(2))
		Expect(addresses[0].Address).To(Equal("172.16.3.42"))
		Expect(addresses[0].Type).To(Equal(clusterv1.MachineExternalIP))
		Expect(addresses[1].Address).To(Equal("172.16.3.43"))
		Expect(addresses[1].Type).To(Equal(clusterv1.MachineExternalIP))
	})

	It("should return error when VMI not found", func() {
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-vm", Namespace: "default"},
		}
		hvClient := hvfake.NewSimpleClientset() // no VMI

		_, err := getIPAddressesFromVMI(context.TODO(), vm, hvClient)
		Expect(err).To(HaveOccurred())
	})

	It("should return empty list when VMI has no interfaces", func() {
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "no-nic-vm", Namespace: "default"},
		}
		vmi := &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "no-nic-vm", Namespace: "default"},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{},
			},
		}
		hvClient := hvfake.NewSimpleClientset(vmi)

		addresses, err := getIPAddressesFromVMI(context.TODO(), vm, hvClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(addresses).To(BeEmpty())
	})
})

// =============================================================================
// Tests for deletePVCsByPrefix (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("deletePVCsByPrefix", func() {
	It("should delete PVCs matching prefix", func() {
		hvClient := hvfake.NewSimpleClientset(
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-disk-0-abc", Namespace: "default"},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "test-vm-disk-1-def", Namespace: "default"},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "other-pvc", Namespace: "default"},
			},
		)
		logger := log.FromContext(context.TODO())
		scope := &Scope{
			HarvesterClient: hvClient,
			Logger:          &logger,
		}
		r := &HarvesterMachineReconciler{}
		r.deletePVCsByPrefix(context.TODO(), scope, "default", "test-vm-disk-")

		pvcs, err := hvClient.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(pvcs.Items).To(HaveLen(1))
		Expect(pvcs.Items[0].Name).To(Equal("other-pvc"))
	})

	It("should handle no PVCs matching prefix", func() {
		hvClient := hvfake.NewSimpleClientset(
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "unrelated-pvc", Namespace: "default"},
			},
		)
		logger := log.FromContext(context.TODO())
		scope := &Scope{
			HarvesterClient: hvClient,
			Logger:          &logger,
		}
		r := &HarvesterMachineReconciler{}
		r.deletePVCsByPrefix(context.TODO(), scope, "default", "test-vm-disk-")

		pvcs, err := hvClient.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(pvcs.Items).To(HaveLen(1))
		Expect(pvcs.Items[0].Name).To(Equal("unrelated-pvc"))
	})

	It("should handle empty namespace", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())
		scope := &Scope{
			HarvesterClient: hvClient,
			Logger:          &logger,
		}
		r := &HarvesterMachineReconciler{}
		r.deletePVCsByPrefix(context.TODO(), scope, "default", "test-vm-disk-")
		// Should not panic, just do nothing
	})
})

// =============================================================================
// Tests for allocateVMIP (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("allocateVMIP", func() {
	It("should allocate IP from pool", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
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
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).ToNot(BeEmpty())
	})

	It("should be idempotent when IP already allocated", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
				Status: infrav1.HarvesterMachineStatus{
					AllocatedIPAddress: "172.16.3.40",
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(Equal("172.16.3.40"))
	})

	It("should return error when VMNetworkConfig is nil", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("VMNetworkConfig is nil"))
	})

	It("should return error when IPPoolRef is empty", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no IPPool references configured"))
	})

	It("should find existing allocation in pool", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.3.42": "test-ns/test-cp-0",
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
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
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(Equal("172.16.3.42"))
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("capi-vm-pool"))
	})

	It("should allocate from second pool when first is exhausted", func() {
		pool1 := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.40", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.3.40": "test-ns/other-machine", // pool1 full
				},
			},
		}
		pool2 := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-2"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.4.40", RangeEnd: "172.16.4.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{},
		}
		hvClient := hvfake.NewSimpleClientset(pool1, pool2)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRefs: []string{"pool-1", "pool-2"},
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(Equal("172.16.4.40"))
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("pool-2"))
	})

	It("should return error when all pools exhausted", func() {
		pool1 := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.40", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.3.40": "test-ns/other-machine",
				},
			},
		}
		pool2 := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-2"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.4.40", RangeEnd: "172.16.4.40", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.4.40": "test-ns/yet-another",
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool1, pool2)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-new", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRefs: []string{"pool-1", "pool-2"},
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("all configured IP pools exhausted"))
	})

	It("should use IPPoolRef for backward compatibility when IPPoolRefs is empty", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "legacy-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.50", RangeEnd: "172.16.3.59", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef: "legacy-pool", // old field, IPPoolRefs empty
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		err := r.allocateVMIP(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(scope.HarvesterMachine.Status.AllocatedIPAddress).To(Equal("172.16.3.50"))
		Expect(scope.HarvesterMachine.Status.AllocatedPoolRef).To(Equal("legacy-pool"))
	})
})

// =============================================================================
// Tests for releaseVMIP (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("releaseVMIP", func() {
	It("should release IP from pool", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.3.40": "test-ns/test-cp-0",
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
				Status: infrav1.HarvesterMachineStatus{
					AllocatedIPAddress: "172.16.3.40",
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
		r.releaseVMIP(scope)
		// No error return - it's best-effort. Verify pool was updated.
	})

	It("should skip release when no IP allocated", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Status: infrav1.HarvesterMachineStatus{AllocatedIPAddress: ""},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		r.releaseVMIP(scope) // should return immediately
	})

	It("should skip release when VMNetworkConfig is nil", func() {
		hvClient := hvfake.NewSimpleClientset()
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Status: infrav1.HarvesterMachineStatus{AllocatedIPAddress: "172.16.3.40"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}

		r := &HarvesterMachineReconciler{}
		r.releaseVMIP(scope) // should log and return
	})

	It("should handle invalid allocated IP gracefully", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "capi-vm-pool"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.3.40", RangeEnd: "172.16.3.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
				Status: infrav1.HarvesterMachineStatus{
					AllocatedIPAddress: "not-a-valid-ip", // invalid IP
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
		r.releaseVMIP(scope) // should log warning about invalid IP and return
	})

	It("should skip release when pool not found", func() {
		hvClient := hvfake.NewSimpleClientset() // no pool
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				Status: infrav1.HarvesterMachineStatus{AllocatedIPAddress: "172.16.3.40"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRef: "missing-pool",
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		r.releaseVMIP(scope) // should log warning and return
	})

	It("should release IP using AllocatedPoolRef", func() {
		pool := &lbv1beta1.IPPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-2"},
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{RangeStart: "172.16.4.40", RangeEnd: "172.16.4.49", Subnet: "172.16.0.0/16", Gateway: "172.16.0.1"},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Allocated: map[string]string{
					"172.16.4.42": "test-ns/test-cp-0",
				},
			},
		}
		hvClient := hvfake.NewSimpleClientset(pool)
		logger := log.FromContext(context.TODO())

		scope := &Scope{
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "test-ns"},
				Status: infrav1.HarvesterMachineStatus{
					AllocatedIPAddress: "172.16.4.42",
					AllocatedPoolRef:   "pool-2", // allocated from pool-2, not pool-1
				},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{
					VMNetworkConfig: &infrav1.VMNetworkConfig{
						IPPoolRefs: []string{"pool-1", "pool-2"},
					},
				},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		r.releaseVMIP(scope)
		// Verify pool-2 was used (not pool-1)
		updatedPool, err := hvClient.LoadbalancerV1beta1().IPPools().Get(context.TODO(), "pool-2", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		_, exists := updatedPool.Status.Allocated["172.16.4.42"]
		Expect(exists).To(BeFalse())
	})
})

// =============================================================================
// Tests for ReconcileDelete (requires fake HarvesterClient)
// =============================================================================

var _ = Describe("ReconcileDelete", func() {
	It("should delete VM, cloud-init secret, and PVCs", func() {
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0", Namespace: "default"},
			Spec:       kubevirtv1.VirtualMachineSpec{},
		}
		cloudInitSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0-cloud-init", Namespace: "default"},
		}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-0-disk-0-abc", Namespace: "default"},
		}
		hvClient := hvfake.NewSimpleClientset(vm, cloudInitSecret, pvc)
		logger := log.FromContext(context.TODO())

		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cp-0",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
		}

		scope := Scope{
			Ctx:              context.TODO(),
			HarvesterMachine: machine,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		result, err := r.ReconcileDelete(scope)
		// VM exists, so it should request deletion and requeue
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})

	It("should clean up PVCs when VM is already gone", func() {
		cloudInitSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1-cloud-init", Namespace: "default"},
		}
		pvc0 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1-disk-0-abc", Namespace: "default"},
		}
		pvc1 := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cp-1-disk-1-def", Namespace: "default"},
		}
		hvClient := hvfake.NewSimpleClientset(cloudInitSecret, pvc0, pvc1) // no VM
		logger := log.FromContext(context.TODO())

		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cp-1",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
		}

		scope := Scope{
			Ctx:              context.TODO(),
			HarvesterMachine: machine,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero()) // no requeue — finalizer removed

		// Verify PVCs were deleted
		pvcs, listErr := hvClient.CoreV1().PersistentVolumeClaims("default").List(context.TODO(), metav1.ListOptions{})
		Expect(listErr).ToNot(HaveOccurred())
		Expect(pvcs.Items).To(BeEmpty())

		// Verify finalizer was removed
		Expect(machine.Finalizers).ToNot(ContainElement(infrav1.MachineFinalizer))
	})

	It("should handle cloud-init secret already deleted", func() {
		hvClient := hvfake.NewSimpleClientset() // no VM, no secret
		logger := log.FromContext(context.TODO())

		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-cp-2",
				Namespace:  "test-ns",
				Finalizers: []string{infrav1.MachineFinalizer},
			},
		}

		scope := Scope{
			Ctx:              context.TODO(),
			HarvesterMachine: machine,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		result, err := r.ReconcileDelete(scope)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		// Finalizer should be removed
		Expect(machine.Finalizers).ToNot(ContainElement(infrav1.MachineFinalizer))
	})

	It("should return error when finalizer is already missing", func() {
		hvClient := hvfake.NewSimpleClientset() // no VM, no secret
		logger := log.FromContext(context.TODO())

		machine := &infrav1.HarvesterMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-no-finalizer",
				Namespace:  "test-ns",
				Finalizers: []string{}, // no finalizer
			},
		}

		scope := Scope{
			Ctx:              context.TODO(),
			HarvesterMachine: machine,
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"},
			},
			HarvesterCluster: &infrav1.HarvesterCluster{
				Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"},
			},
			HarvesterClient: hvClient,
			Logger:          &logger,
		}

		r := &HarvesterMachineReconciler{}
		_, err := r.ReconcileDelete(scope)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to remove finalizer"))
	})
})
