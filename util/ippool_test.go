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
	"math/big"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
)

var _ = Describe("IP Pool Store", func() {
	var (
		pool  *lbv1beta1.IPPool
		store *Store
	)

	BeforeEach(func() {
		pool = &lbv1beta1.IPPool{
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
			},
		}
		store = NewStore(pool)
	})

	Describe("NewStore", func() {
		It("should create a store wrapping the pool", func() {
			Expect(store).ToNot(BeNil())
			Expect(store.IPPool).To(Equal(pool))
		})
	})

	Describe("Lock/Unlock/Close", func() {
		It("should be no-ops that return nil", func() {
			Expect(store.Lock()).To(Succeed())
			Expect(store.Unlock()).To(Succeed())
			Expect(store.Close()).To(Succeed())
		})
	})

	Describe("Reserve", func() {
		It("should reserve an IP and update allocated map", func() {
			ok, err := store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())
			Expect(pool.Status.Allocated).To(HaveKeyWithValue("172.16.3.40", "machine-1"))
			Expect(pool.Status.LastAllocated).To(Equal("172.16.3.40"))
			Expect(pool.Status.Available).To(Equal(int64(9)))
		})

		It("should return false for an already reserved IP", func() {
			ok, err := store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeTrue())

			ok, err = store.Reserve("machine-2", "", net.ParseIP("172.16.3.40"), "")
			Expect(err).ToNot(HaveOccurred())
			Expect(ok).To(BeFalse())
		})

		It("should allow reserving multiple different IPs", func() {
			ok, _ := store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			Expect(ok).To(BeTrue())

			ok, _ = store.Reserve("machine-2", "", net.ParseIP("172.16.3.41"), "")
			Expect(ok).To(BeTrue())

			Expect(pool.Status.Allocated).To(HaveLen(2))
			Expect(pool.Status.Available).To(Equal(int64(8)))
		})
	})

	Describe("LastReservedIP", func() {
		It("should return the last reserved IP", func() {
			_, _ = store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			_, _ = store.Reserve("machine-2", "", net.ParseIP("172.16.3.41"), "")

			ip, err := store.LastReservedIP("")
			Expect(err).ToNot(HaveOccurred())
			Expect(ip.String()).To(Equal("172.16.3.41"))
		})
	})

	Describe("ReleaseByID", func() {
		It("should release all IPs for a given ID", func() {
			_, _ = store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			_, _ = store.Reserve("machine-1", "", net.ParseIP("172.16.3.41"), "")
			_, _ = store.Reserve("machine-2", "", net.ParseIP("172.16.3.42"), "")

			err := store.ReleaseByID("machine-1", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(pool.Status.Allocated).To(HaveLen(1))
			Expect(pool.Status.Allocated).To(HaveKey("172.16.3.42"))
			Expect(pool.Status.AllocatedHistory).To(HaveLen(2))
		})

		It("should be safe to call on nil Allocated map", func() {
			pool.Status.Allocated = nil
			err := store.ReleaseByID("machine-1", "")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be a no-op when the ID is not found", func() {
			_, _ = store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")

			err := store.ReleaseByID("machine-unknown", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(pool.Status.Allocated).To(HaveLen(1))
		})
	})

	Describe("GetByID", func() {
		It("should return IPs for a given ID", func() {
			_, _ = store.Reserve("machine-1", "", net.ParseIP("172.16.3.40"), "")
			_, _ = store.Reserve("machine-2", "", net.ParseIP("172.16.3.41"), "")

			ips := store.GetByID("machine-1", "")
			Expect(ips).To(HaveLen(1))
			Expect(ips[0].String()).To(Equal("172.16.3.40"))
		})

		It("should return empty slice when ID not found", func() {
			ips := store.GetByID("machine-unknown", "")
			Expect(ips).To(BeEmpty())
		})
	})
})

var _ = Describe("MakeRange", func() {
	It("should parse a valid range with start and end", func() {
		r := &lbv1beta1.Range{
			Subnet:     "172.16.0.0/16",
			RangeStart: "172.16.3.40",
			RangeEnd:   "172.16.3.49",
			Gateway:    "172.16.0.1",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RangeStart.String()).To(Equal("172.16.3.40"))
		Expect(result.RangeEnd.String()).To(Equal("172.16.3.49"))
		Expect(result.Gateway.String()).To(Equal("172.16.0.1"))
	})

	It("should use defaults when start/end/gateway are empty", func() {
		r := &lbv1beta1.Range{
			Subnet: "192.168.1.0/24",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RangeStart.String()).To(Equal("192.168.1.1"))
		Expect(result.RangeEnd.String()).To(Equal("192.168.1.254"))
		Expect(result.Gateway.String()).To(Equal("192.168.1.1"))
	})

	It("should swap start and end if start > end", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "192.168.1.200",
			RangeEnd:   "192.168.1.100",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RangeStart.String()).To(Equal("192.168.1.100"))
		Expect(result.RangeEnd.String()).To(Equal("192.168.1.200"))
	})

	It("should reject an invalid subnet", func() {
		r := &lbv1beta1.Range{
			Subnet: "not-a-cidr",
		}

		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
	})

	It("should reject an IP outside the subnet", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "10.0.0.1",
		}

		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
	})

	It("should reject the network address as start", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "192.168.1.0",
		}

		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
	})

	It("should reject the broadcast address as end", func() {
		r := &lbv1beta1.Range{
			Subnet:   "192.168.1.0/24",
			RangeEnd: "192.168.1.255",
		}

		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("CountIP", func() {
	It("should count IPs in a range excluding gateway", func() {
		r := &lbv1beta1.Range{
			Subnet:     "172.16.0.0/16",
			RangeStart: "172.16.3.40",
			RangeEnd:   "172.16.3.49",
			Gateway:    "172.16.0.1",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())

		count := CountIP(result)
		// 40 to 49 = 10 IPs, gateway 172.16.0.1 is outside range
		Expect(count).To(Equal(int64(10)))
	})

	It("should subtract gateway when it falls within the range", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "192.168.1.1",
			RangeEnd:   "192.168.1.10",
			Gateway:    "192.168.1.1",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())

		count := CountIP(result)
		// 1 to 10 = 10 IPs, minus gateway = 9
		Expect(count).To(Equal(int64(9)))
	})

	It("should return 1 for a single-IP range", func() {
		r := &lbv1beta1.Range{
			Subnet:     "172.16.0.0/16",
			RangeStart: "172.16.3.40",
			RangeEnd:   "172.16.3.40",
		}

		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())

		count := CountIP(result)
		Expect(count).To(Equal(int64(1)))
	})
})

var _ = Describe("AllocateVMIPFromPool", func() {
	It("should allocate an IP from a pool", func() {
		pool := &lbv1beta1.IPPool{
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "172.16.0.0/16",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
						Gateway:    "172.16.0.1",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
			},
		}

		ip, err := AllocateVMIPFromPool(pool, "machine-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).ToNot(BeEmpty())
		// IP should be in the range 172.16.3.40-49
		parsed := net.ParseIP(ip)
		Expect(parsed).ToNot(BeNil())
		Expect(pool.Status.Allocated).To(HaveKey(ip))
	})

	It("should allocate different IPs for different machines", func() {
		pool := &lbv1beta1.IPPool{
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "172.16.0.0/16",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
						Gateway:    "172.16.0.1",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
			},
		}

		ip1, err := AllocateVMIPFromPool(pool, "machine-1")
		Expect(err).ToNot(HaveOccurred())

		ip2, err := AllocateVMIPFromPool(pool, "machine-2")
		Expect(err).ToNot(HaveOccurred())

		Expect(ip1).ToNot(Equal(ip2))
		Expect(pool.Status.Allocated).To(HaveLen(2))
	})

	It("should reuse a historical IP for the same machine ID", func() {
		pool := &lbv1beta1.IPPool{
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "172.16.0.0/16",
						RangeStart: "172.16.3.40",
						RangeEnd:   "172.16.3.49",
						Gateway:    "172.16.0.1",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				Available: 10,
				AllocatedHistory: map[string]string{
					"172.16.3.45": "machine-1",
				},
			},
		}

		ip, err := AllocateVMIPFromPool(pool, "machine-1")
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).To(Equal("172.16.3.45"))
	})

	It("should fail with an invalid range", func() {
		pool := &lbv1beta1.IPPool{
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet: "invalid",
					},
				},
			},
		}

		_, err := AllocateVMIPFromPool(pool, "machine-1")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ipToInt", func() {
	It("should convert IPv4 address to integer", func() {
		ip := net.ParseIP("192.168.1.1")
		result := ipToInt(ip)
		expected := big.NewInt(0).SetBytes(ip.To4())
		Expect(result.Cmp(expected)).To(Equal(0))
	})

	It("should convert IPv6 address to integer", func() {
		ip := net.ParseIP("::1")
		// For IPv6, To4() returns nil, so it should use To16()
		result := ipToInt(ip)
		Expect(result).ToNot(BeNil())

		expected := big.NewInt(1)
		Expect(result.Cmp(expected)).To(Equal(0))
	})

	It("should handle full IPv6 address", func() {
		ip := net.ParseIP("2001:db8::1")
		result := ipToInt(ip)
		Expect(result).ToNot(BeNil())
		Expect(result.Sign()).To(Equal(1)) // positive
	})
})

var _ = Describe("MakeRange edge cases", func() {
	It("should handle invalid range start IP", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "999.999.999.999",
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid range start"))
	})

	It("should handle invalid range end IP", func() {
		r := &lbv1beta1.Range{
			Subnet:   "192.168.1.0/24",
			RangeEnd: "999.999.999.999",
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid range end"))
	})

	It("should handle invalid gateway IP", func() {
		r := &lbv1beta1.Range{
			Subnet:  "192.168.1.0/24",
			Gateway: "999.999.999.999",
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid gateway"))
	})

	It("should swap start and end when start > end", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "192.168.1.200",
			RangeEnd:   "192.168.1.100",
		}
		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RangeStart.String()).To(Equal("192.168.1.100"))
		Expect(result.RangeEnd.String()).To(Equal("192.168.1.200"))
	})

	It("should handle point-to-point subnet /32", func() {
		r := &lbv1beta1.Range{
			Subnet: "192.168.1.1/32",
		}
		result, err := MakeRange(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
	})

	It("should handle IP outside subnet for range start", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "10.0.0.1",
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("out of subnet"))
	})
})

var _ = Describe("parseIP edge cases", func() {
	It("should reject network address", func() {
		r := &lbv1beta1.Range{
			Subnet:     "192.168.1.0/24",
			RangeStart: "192.168.1.0", // network address
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("network address"))
	})

	It("should reject broadcast address", func() {
		r := &lbv1beta1.Range{
			Subnet:   "192.168.1.0/24",
			RangeEnd: "192.168.1.255", // broadcast address
		}
		_, err := MakeRange(r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("broadcast address"))
	})
})

var _ = Describe("AllocateVMIPFromPool with history", func() {
	It("should reuse historical IP for the same machine", func() {
		pool := &lbv1beta1.IPPool{
			Spec: lbv1beta1.IPPoolSpec{
				Ranges: []lbv1beta1.Range{
					{
						Subnet:     "192.168.1.0/24",
						RangeStart: "192.168.1.10",
						RangeEnd:   "192.168.1.20",
					},
				},
			},
			Status: lbv1beta1.IPPoolStatus{
				AllocatedHistory: map[string]string{
					"192.168.1.15": "machine-reuse",
				},
			},
		}

		ip, err := AllocateVMIPFromPool(pool, "machine-reuse")
		Expect(err).ToNot(HaveOccurred())
		Expect(ip).To(Equal("192.168.1.15"))
	})
})
