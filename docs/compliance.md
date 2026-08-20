# Compliance guide

This guide maps common hardening and compliance frameworks to the layers of a
CAPHV-managed platform, and shows which requirements the provider addresses
directly. A workload cluster provisioned by CAPHV involves four layers, and
every framework below spreads its requirements across them:

| Layer | Component | Typical responsibilities |
|---|---|---|
| Virtualization | Harvester / KubeVirt / Longhorn | host hardening, VM isolation, storage encryption, host HA |
| Infrastructure provisioning | **CAPHV (this provider)** | VM firmware and devices, networks and IP allocation, placement, per machine type configuration |
| Kubernetes distribution | RKE2 (CAPI bootstrap and control plane provider) | API server, kubelet and etcd hardening, audit, RBAC, CIS profile |
| Cluster operations | GitOps, Rancher, monitoring | policies, secrets handling, backup, audit automation |

CAPHV cannot (and should not) implement requirements that belong to another
layer; this guide tells you where each control lives and what the provider
gives you to satisfy the ones in its scope.

## BSI IT-Grundschutz, module APP.4.4 (Kubernetes)

The German BSI IT-Grundschutz Kompendium covers Kubernetes in building block
APP.4.4 (edition 2023; refer to the official publication for the authoritative
text). The table lists every requirement with the layer that carries it and,
when CAPHV is involved, the feature to use.

| Req | Topic | Layer | How |
|---|---|---|---|
| A1 | Planning application separation | operations | one workload cluster per application or protection level; CAPHV makes clusters cheap to create (see A14) |
| A2 | CI/CD automation planning | operations | GitOps-driven cluster definitions (validated with Fleet in the certification suite) |
| A3 | Identity and permission management | RKE2 / Rancher | Kubernetes RBAC, OIDC via Rancher |
| A4 | Pod separation | RKE2 / operations | Pod Security Standards, enforced cluster-wide by the RKE2 CIS profile |
| A5 | Backup in the cluster | RKE2 / Harvester / operations | RKE2 etcd snapshots, Longhorn backups of VM volumes |
| A6 | Pod initialization | operations | application-level concern |
| A7 | **Network separation** | **CAPHV** + CNI | network-aware IP pool selection (v0.6.1) and per machine type `vmNetworkConfig` (v0.7.0) put control plane and workers in separate networks with their own pools, gateway, subnet mask and DNS; in-cluster segmentation stays with the CNI |
| A8 | Securing configuration files | operations | GitOps repository controls |
| A9 | Dedicated nodes | **CAPHV** + RKE2 | `nodeAffinity`/`workloadAffinity` per machine template pin VMs to dedicated Harvester hosts; node taints and labels via the RKE2 templates |
| A10 | Securing sensitive configuration data | RKE2 / operations | RKE2 secrets encryption at rest, external secret stores |
| A11 | Automatic configuration audit | operations | kube-bench, policy engines, NeuVector |
| A12 | Securing infrastructure applications | operations | registry, CI/CD and backup infrastructure hardening |
| A13 | Automated network configuration | CNI / operations | NetworkPolicies managed as code |
| A14 | Dedicated clusters | **CAPHV** | one cluster per application: clusters are isolated by `targetNamespace` on the Harvester side and cheap to provision through ClusterClass |
| A15 | Application separation on node and cluster level | **CAPHV** + RKE2 | separate MachineDeployments per application class, each with its own template (affinity, network, firmware); scheduling constraints via RKE2 |
| A16 | Use of operators | architecture | the whole platform is operator-driven (Cluster API) |
| A17 | **Node attestation** | **CAPHV** | UEFI Secure Boot and vTPM per machine type (v0.8.0): `spec.firmware {efi, secureBoot}` and `spec.tpm {enabled, persistent}` |
| A18 | Micro-segmentation | CNI / operations | NetworkPolicies, service mesh |
| A19 | **High availability** | **CAPHV** + Harvester | failure domain discovery, publication and placement (v0.9.0): control plane machines spread across Harvester zones or hosts; host-level HA is Harvester's |
| A20 | **Encrypted data storage for pods** | Harvester + **CAPHV** | Longhorn-encrypted StorageClasses and encrypted VM images; CAPHV consumes them transparently (see "Encrypted storage" in `operations.md`) |
| A21 | Regular pod restarts | operations | workload lifecycle policies |

The bold rows are the ones the provider had to implement; they were requested
by users subject to BSI compliance in
[#234](https://github.com/harvester/cluster-api-provider-harvester/issues/234),
[#237](https://github.com/harvester/cluster-api-provider-harvester/issues/237)
and
[#238](https://github.com/harvester/cluster-api-provider-harvester/issues/238),
and functionally validated against a real Harvester cluster.

## CIS Benchmarks

The CIS Kubernetes Benchmark is best applied through RKE2, which ships a
dedicated hardening mode: setting `profile: cis` in the RKE2 server
configuration enforces the benchmark's kernel parameters, applies restrictive
Pod Security Standards and refuses to start on non-compliant nodes.

With the shipped ClusterClass this is a single switch: set the `cisProfile`
topology variable (`cis`, or a versioned profile such as `cis-1.23`):

```yaml
spec:
  topology:
    variables:
      - name: cisProfile
        value: "cis"
```

The profile is then enabled on the control plane and the workers
(`agentConfig.cisProfile` on the RKE2 templates), and the node prerequisites
the hardened kubelet requires are injected automatically through
`preRKE2Commands`: the `etcd` system user and the protect-kernel-defaults
sysctls (`vm.panic_on_oom=0`, `vm.overcommit_memory=1`, `kernel.panic=10`,
`kernel.panic_on_oops=1`). Clusters built without ClusterClass can replicate
this by setting the same two fields in their RKE2 templates.

## DISA STIG

DISA publishes a Security Technical Implementation Guide for RKE2, and SLES
has its own OS STIG. Layer split:

- RKE2 STIG items (audit logging, TLS ciphers, kubelet flags, PSS) are
  configured through the RKE2 templates of the cluster;
- OS-level STIG items belong to the node image (build STIG-hardened SLES
  images and reference them in `volumes[].imageName`);
- CAPHV contributes the infrastructure controls: Secure Boot and vTPM for
  boot integrity, separate networks per machine type, failure domain spread.

The release ships a pre-hardened flavor, `cluster-template-stig.yaml`
(`clusterctl generate cluster --flavor stig`), which configures the RKE2 side
accordingly: CIS profile on every node with the prerequisites injected, audit
logging enabled through a metadata-level audit policy Secret with retention
flags on the API server, TLS 1.2 minimum and idle streaming timeouts on the
kubelets and the API server, the restricted pod security admission
configuration with the Rancher ecosystem exemptions, and VMs booting with
UEFI Secure Boot and a persistent vTPM. Pair it with a STIG-hardened SLES
image (`VM_IMAGE_NAME`) to cover the OS layer.

## FIPS 140

FIPS mode is a boot-time property of the node image (kernel parameter plus the
FIPS pattern); it cannot be turned on by cloud-init without a reboot, so CAPHV
does not pretend to enable it. The split is:

- **Build a FIPS node image** once: boot the standard SLES or openSUSE cloud
  image, run `zypper -n in -t pattern fips`, add `fips=1` to
  `GRUB_CMDLINE_LINUX_DEFAULT` (`grub2-mkconfig -o /boot/grub2/grub.cfg`),
  regenerate the initrd (`dracut -f`), run `cloud-init clean --logs`, stop the
  VM and export its volume as a Harvester image
  (`VirtualMachineImage` with `sourceType: export-from-volume`). Reference the
  image in `volumes[].imageName`.
- **Enforce it from the cluster definition**: the `fipsRequired` topology
  variable of the shipped ClusterClass (default `false`) injects a guard into
  the node bootstrap that fails unless `/proc/sys/crypto/fips_enabled` is `1`,
  so a cluster that must be FIPS can never silently run on a non-FIPS image.
- RKE2 is available in FIPS-validated builds for government use; select the
  matching RKE2 version in the cluster definition.

## ANSSI (France)

The French ANSSI hardening recommendations (BP-028 for Linux, and the ANSSI
Kubernetes security guide) follow the same split: OS items belong to the node
image, Kubernetes items to RKE2 configuration, and the infrastructure themes
(network separation, dedicated nodes, availability) are covered by the same
CAPHV features as the BSI rows above.
