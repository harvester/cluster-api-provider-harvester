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

package v1beta1

import (
	"strings"
	"testing"
)

func validMachine() *HarvesterMachine {
	return &HarvesterMachine{
		Spec: HarvesterMachineSpec{
			CPU:        2,
			Memory:     "4Gi",
			SSHUser:    "opensuse",
			SSHKeyPair: "default/ssh-key",
			Volumes:    []Volume{{VolumeType: "image", ImageName: "default/leap"}},
			Networks:   []string{"default/workers"},
		},
	}
}

func TestValidateMachineVMNetworkConfig(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m *HarvesterMachine)
		wantErr string
	}{
		{
			"valid machine-level config",
			func(m *HarvesterMachine) {
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					IPPoolRefs: []string{"pool-workers"},
					Gateway:    "10.20.0.1",
					SubnetMask: "255.255.255.0",
				}
			},
			"",
		},
		{
			"no pool source",
			func(m *HarvesterMachine) {
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					Gateway:    "10.20.0.1",
					SubnetMask: "255.255.255.0",
				}
			},
			"ipPoolRef",
		},
		{
			"inline ipPool is rejected at the machine level",
			func(m *HarvesterMachine) {
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					IPPool:     &IpPool{VMNetwork: "workers", Subnet: "10.20.0.0/24", Gateway: "10.20.0.1"},
					Gateway:    "10.20.0.1",
					SubnetMask: "255.255.255.0",
				}
			},
			"ipPool is not supported",
		},
		{
			"mutually exclusive with static networkConfig",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfig = &NetworkConfig{Address: "10.20.0.10", Gateway: "10.20.0.1"}
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					IPPoolRef:  "pool-workers",
					Gateway:    "10.20.0.1",
					SubnetMask: "255.255.255.0",
				}
			},
			"mutually exclusive",
		},
		{
			"invalid gateway",
			func(m *HarvesterMachine) {
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					IPPoolRef:  "pool-workers",
					Gateway:    "not-an-ip",
					SubnetMask: "255.255.255.0",
				}
			},
			"gateway",
		},
		{
			"invalid subnet mask",
			func(m *HarvesterMachine) {
				m.Spec.VMNetworkConfig = &VMNetworkConfig{
					IPPoolRef:  "pool-workers",
					Gateway:    "10.20.0.1",
					SubnetMask: "not-a-mask",
				}
			},
			"subnetMask",
		},
	}
	for _, tc := range cases {
		m := validMachine()
		tc.mutate(m)

		_, err := validateHarvesterMachine(m)

		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}

		if tc.wantErr != "" && err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}

		if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error should mention %q: %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestValidateMachineNetworkConfigs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(m *HarvesterMachine)
		wantErr string
	}{
		{
			"valid per-NIC configs parallel to networks",
			func(m *HarvesterMachine) {
				m.Spec.Networks = []string{"default/net-a", "default/net-b"}
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					{IPPoolRefs: []string{"pool-b"}, Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
				}
			},
			"",
		},
		{
			"fewer entries than networks is allowed (rest stays on DHCP)",
			func(m *HarvesterMachine) {
				m.Spec.Networks = []string{"default/net-a", "default/net-b"}
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
				}
			},
			"",
		},
		{
			"more entries than networks is rejected",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
					{IPPoolRef: "pool-b", Gateway: "172.17.0.1", SubnetMask: "255.255.255.0"},
				}
			},
			"only 1 networks",
		},
		{
			"entry without pool is rejected",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
				}
			},
			"requires one of ipPoolRef or ipPoolRefs",
		},
		{
			"invalid gateway",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "nope", SubnetMask: "255.255.255.0"},
				}
			},
			"gateway",
		},
		{
			"invalid subnet mask",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "nope"},
				}
			},
			"subnetMask",
		},
		{
			"mutually exclusive with static networkConfig",
			func(m *HarvesterMachine) {
				m.Spec.NetworkConfig = &NetworkConfig{Address: "10.20.0.10", Gateway: "10.20.0.1"}
				m.Spec.NetworkConfigs = []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "255.255.255.0"},
				}
			},
			"mutually exclusive",
		},
	}
	for _, tc := range cases {
		m := validMachine()
		tc.mutate(m)

		_, err := validateHarvesterMachine(m)

		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}

		if tc.wantErr != "" && err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}

		if tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error should mention %q: %v", tc.name, tc.wantErr, err)
		}
	}
}
