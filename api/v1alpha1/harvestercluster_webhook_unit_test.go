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

package v1alpha1

import (
	"strings"
	"testing"
)

func validCluster() *HarvesterCluster {
	return &HarvesterCluster{
		Spec: HarvesterClusterSpec{
			TargetNamespace:    "default",
			IdentitySecret:     SecretKey{Namespace: "default", Name: "id"},
			LoadBalancerConfig: LoadBalancerConfig{IPAMType: IPAMType(DHCP)},
		},
	}
}

func TestValidateClusterVMNetworkConfigInterfaces(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *VMNetworkConfig
		wantErr string
	}{
		{
			"interfaces without top-level gateway, subnetMask or pool are valid",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{IPPoolRef: "oob2", Gateway: "11.0.0.1", SubnetMask: "255.255.254.0"},
					{IPPoolRefs: []string{"production"}, Gateway: "10.0.0.1", SubnetMask: "255.255.254.0"},
				},
				DNSServers: []string{"1.1.1.1", "2.2.2.2"},
				DNSSearch:  []string{"cdc.fr"},
			},
			"",
		},
		{
			"interface without pool is rejected",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{Gateway: "172.17.0.1", SubnetMask: "255.255.0.0"},
				},
			},
			"interfaces[0] requires one of ipPoolRef or ipPoolRefs",
		},
		{
			"interface without gateway is rejected",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", SubnetMask: "255.255.255.0"},
				},
			},
			"interfaces[0].gateway is required",
		},
		{
			"interface with invalid gateway is rejected",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "nope", SubnetMask: "255.255.255.0"},
				},
			},
			"interfaces[0].gateway",
		},
		{
			"interface without subnetMask is rejected",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1"},
				},
			},
			"interfaces[0].subnetMask is required",
		},
		{
			"interface with invalid subnet mask is rejected",
			&VMNetworkConfig{
				Interfaces: []NetworkInterfaceConfig{
					{IPPoolRef: "pool-a", Gateway: "172.16.0.1", SubnetMask: "nope"},
				},
			},
			"interfaces[0].subnetMask",
		},
		{
			"missing top-level gateway without interfaces is rejected",
			&VMNetworkConfig{IPPoolRef: "pool-a", SubnetMask: "255.255.255.0"},
			"gateway is required",
		},
		{
			"valid single-NIC",
			&VMNetworkConfig{IPPoolRef: "pool-a", Gateway: "10.0.0.1", SubnetMask: "255.255.255.0"},
			"",
		},
	}
	for _, tc := range cases {
		c := validCluster()
		c.Spec.VMNetworkConfig = tc.cfg

		_, err := validateHarvesterCluster(c)

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
