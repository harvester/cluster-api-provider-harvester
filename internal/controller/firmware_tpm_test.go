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

	kubevirtv1 "kubevirt.io/api/core/v1"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// =============================================================================
// Tests for the firmware and TPM options (UEFI Secure Boot, vTPM, issue #238)
// =============================================================================

var _ = Describe("applyFirmwareAndTPM", func() {
	It("should leave the domain untouched when neither firmware nor tpm is set", func() {
		machine := &infrav1.HarvesterMachine{}
		domain := &kubevirtv1.DomainSpec{}

		applyFirmwareAndTPM(machine, domain)

		Expect(domain.Firmware).To(BeNil())
		Expect(domain.Features).To(BeNil())
		Expect(domain.Devices.TPM).To(BeNil())
	})

	It("should configure EFI without Secure Boot and set secureBoot explicitly to false", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Firmware: &infrav1.Firmware{EFI: true},
			},
		}
		domain := &kubevirtv1.DomainSpec{}

		applyFirmwareAndTPM(machine, domain)

		Expect(domain.Firmware).ToNot(BeNil())
		Expect(domain.Firmware.Bootloader).ToNot(BeNil())
		Expect(domain.Firmware.Bootloader.EFI).ToNot(BeNil())
		// kubevirt defaults secureBoot to true when EFI is set, so the value
		// must always be pinned explicitly.
		Expect(domain.Firmware.Bootloader.EFI.SecureBoot).To(HaveValue(BeFalse()))
		Expect(domain.Features).To(BeNil())
	})

	It("should configure Secure Boot with the SMM feature", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				Firmware: &infrav1.Firmware{EFI: true, SecureBoot: true},
			},
		}
		domain := &kubevirtv1.DomainSpec{}

		applyFirmwareAndTPM(machine, domain)

		Expect(domain.Firmware.Bootloader.EFI.SecureBoot).To(HaveValue(BeTrue()))
		Expect(domain.Features).ToNot(BeNil())
		Expect(domain.Features.SMM).ToNot(BeNil())
		Expect(domain.Features.SMM.Enabled).To(HaveValue(BeTrue()))
	})

	It("should attach a TPM device with the requested persistence", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				TPM: &infrav1.TPM{Enabled: true, Persistent: true},
			},
		}
		domain := &kubevirtv1.DomainSpec{}

		applyFirmwareAndTPM(machine, domain)

		Expect(domain.Devices.TPM).ToNot(BeNil())
		Expect(domain.Devices.TPM.Persistent).To(HaveValue(BeTrue()))
		Expect(domain.Firmware).To(BeNil())
	})

	It("should not attach a TPM device when the block is present but disabled", func() {
		machine := &infrav1.HarvesterMachine{
			Spec: infrav1.HarvesterMachineSpec{
				TPM: &infrav1.TPM{Enabled: false},
			},
		}
		domain := &kubevirtv1.DomainSpec{}

		applyFirmwareAndTPM(machine, domain)

		Expect(domain.Devices.TPM).To(BeNil())
	})
})
