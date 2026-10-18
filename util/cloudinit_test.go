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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MergeCloudInitStrings", func() {
	var cloudinit1 string

	var cloudinit2 string

	var cloudinit3 string

	BeforeEach(func() {
		cloudinit1 = `#cloud-config
package_update: true
packages:
  - nginx
runcmd:
  - echo "hello world"
`
		cloudinit2 = `ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAACAA... user@host`

		cloudinit3 = `#cloud-config
package_update: false
packages:
  - curl
runcmd:
  - echo "hello world 3"
`
	})
	It("Should show the right resulting cloud-init", func() {
		mergedCloudInit, err := MergeCloudInitData(cloudinit1, cloudinit2, cloudinit3)
		Expect(err).ToNot(HaveOccurred())

		mergedCloudInitString := string(mergedCloudInit)
		_, err = GinkgoWriter.Write(mergedCloudInit)
		Expect(err).NotTo(HaveOccurred())
		Expect(mergedCloudInitString).To(Equal(`#cloud-config
package_update: false
packages:
- nginx
- curl
runcmd:
- echo "hello world"
- echo "hello world 3"
ssh_authorized_keys:
- ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAACAA... user@host
`))
	})
})

var _ = Describe("MergeCloudInitData edge cases", func() {
	It("should handle empty strings gracefully", func() {
		result, err := MergeCloudInitData("", "", "")
		Expect(err).ToNot(HaveOccurred())
		Expect(string(result)).To(HavePrefix("#cloud-config"))
	})

	It("should handle a single cloud-init input", func() {
		input := `packages:
  - curl
runcmd:
  - echo "hello"
`
		result, err := MergeCloudInitData(input)
		Expect(err).ToNot(HaveOccurred())

		resultStr := string(result)
		Expect(resultStr).To(ContainSubstring("curl"))
		Expect(resultStr).To(ContainSubstring("echo"))
	})

	It("should merge bootcmd sections as lists", func() {
		ci1 := `bootcmd:
  - echo "first"
`
		ci2 := `bootcmd:
  - echo "second"
`
		result, err := MergeCloudInitData(ci1, ci2)
		Expect(err).ToNot(HaveOccurred())

		resultStr := string(result)
		Expect(resultStr).To(ContainSubstring("echo \"first\""))
		Expect(resultStr).To(ContainSubstring("echo \"second\""))
	})

	It("should merge write_files sections as lists", func() {
		ci1 := `write_files:
  - path: /tmp/file1
    content: "hello"
`
		ci2 := `write_files:
  - path: /tmp/file2
    content: "world"
`
		result, err := MergeCloudInitData(ci1, ci2)
		Expect(err).ToNot(HaveOccurred())

		resultStr := string(result)
		Expect(resultStr).To(ContainSubstring("/tmp/file1"))
		Expect(resultStr).To(ContainSubstring("/tmp/file2"))
	})

	It("should overwrite non-list keys with the last value", func() {
		ci1 := `hostname: node1
`
		ci2 := `hostname: node2
`
		result, err := MergeCloudInitData(ci1, ci2)
		Expect(err).ToNot(HaveOccurred())

		resultStr := string(result)
		Expect(resultStr).To(ContainSubstring("node2"))
		Expect(resultStr).ToNot(ContainSubstring("node1"))
	})

	It("should return error for malformed YAML", func() {
		_, err := MergeCloudInitData("invalid: [yaml: broken")
		Expect(err).To(HaveOccurred())
	})

	It("should skip empty strings between valid inputs", func() {
		ci1 := `packages:
  - vim
`
		ci2 := ""
		ci3 := `packages:
  - git
`
		result, err := MergeCloudInitData(ci1, ci2, ci3)
		Expect(err).ToNot(HaveOccurred())

		resultStr := string(result)
		Expect(resultStr).To(ContainSubstring("vim"))
		Expect(resultStr).To(ContainSubstring("git"))
	})

	It("should return error when list section value is not a list", func() {
		// First input sets packages as a proper list
		ci1 := `packages:
  - vim
`
		// Second input has packages as a scalar (not a list)
		// This should trigger the "unable to cast value to []any" error
		ci2 := `packages: curl
`
		_, err := MergeCloudInitData(ci1, ci2)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to cast"))
	})
})

var _ = Describe("IsTalosMachineConfig", func() {
	It("detects a Talos machine config", func() {
		data := "version: v1alpha1\nmachine:\n  type: controlplane\ncluster:\n  clusterName: test\n"
		Expect(IsTalosMachineConfig(data)).To(BeTrue())
	})

	It("detects a multi-document Talos config", func() {
		data := "version: v1alpha1\nmachine:\n  type: worker\n---\napiVersion: v1alpha1\nkind: HostnameConfig\n"
		Expect(IsTalosMachineConfig(data)).To(BeTrue())
	})

	It("does not match cloud-config", func() {
		Expect(IsTalosMachineConfig("#cloud-config\nruncmd:\n  - echo hi\n")).To(BeFalse())
	})

	It("does not match cloud-config without a header", func() {
		Expect(IsTalosMachineConfig("runcmd:\n  - echo hello\n")).To(BeFalse())
	})

	It("does not match kubeadm jinja cloud-config", func() {
		data := "## template: jinja\n#cloud-config\nruncmd:\n  - kubeadm init\n"
		Expect(IsTalosMachineConfig(data)).To(BeFalse())
	})

	It("does not match empty data or a shell script", func() {
		Expect(IsTalosMachineConfig("")).To(BeFalse())
		Expect(IsTalosMachineConfig("#!/bin/bash\necho hi\n")).To(BeFalse())
	})
})
