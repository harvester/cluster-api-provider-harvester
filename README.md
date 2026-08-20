# Cluster API Provider Harvester (CAPHV)

> Kubernetes Cluster API infrastructure provider for [Harvester HCI](https://harvesterhci.io), with Harvester v1.7+/v1.8+ compatibility and production-ready features.

## Overview

CAPHV is a [Cluster API](https://cluster-api.sigs.k8s.io/) Infrastructure Provider for provisioning Kubernetes clusters on [Harvester HCI](https://harvesterhci.io/).

Since the original v0.1.x implementation, the project has grown significant
production capabilities:

| Feature | v0.1.x (original) | Current |
|---------|-------------------|---------|
| Harvester compatibility | v1.2.0 | v1.7.x + v1.8.x |
| Multi-disk VMs | Single disk only | Multiple disks (image + storageClass) |
| IP allocation | Manual / DHCP | Automatic from Harvester IPPool or DHCP |
| Cloud-init | Basic | Network-config v1 (SLES), multi-NIC, static IP + DHCP |
| Cloud provider bootstrap | Manual fixes needed | Automatic (hostNetwork, RBAC, tolerations) |
| Node initialization | Manual providerID | Automatic from management cluster |
| etcd cleanup | Manual | Automatic on CP machine deletion |
| Validating webhooks | None | HarvesterMachine + HarvesterCluster |
| Boot order | Not supported | Configurable per-disk |
| VM runStrategy | Deprecated `spec.running` | `spec.runStrategy: Always` |
| MachineHealthCheck | Untested | Tested, full auto-remediation |
| Rolling K8s upgrade | Untested | Tested (CP + workers) |
| E2E tests | Kubebuilder scaffold only | 18 integration tests (live cluster) |
| ClusterClass | Generic example only | Production-ready with vmNetworkConfig, IPPool, sshUser |
| CLI generator | None | `caphv-generate` script (~30-line clusters) |
| Fleet/CAAPF addons | Not supported | CSI/CNI via Fleet GitOps with per-cluster CNI tuning |
| Helm chart | None | Full chart with webhook + ClusterClass support |
| Multi-network clusters | Single network | Network-aware pool selection + per machine type `vmNetworkConfig` (separate control-plane/worker networks) |
| Boot security | BIOS only | UEFI Secure Boot + vTPM per machine type |
| Failure domains | Not published | Discovered from Harvester hosts/zones, control plane spread |
| Hardening | None | `cisProfile` and `fipsRequired` ClusterClass switches, STIG flavor, compliance guide |

## Prerequisites

- Harvester HCI v1.7.x or v1.8.x cluster
- Management cluster (RKE2 recommended) with:
  - Cluster API Core **v1.12.x** (serves `cluster.x-k8s.io/v1beta2`)
  - RKE2 Bootstrap + ControlPlane providers **v0.24.x+** (validated against v0.25.0)
  - Rancher Turtles **v0.26.x** (Rancher Manager 2.14.x ships Turtles by default,
    but the RKE2 CAPIProviders must be enabled explicitly - see below)
  - cert-manager (required for webhooks, default-on)
- Harvester identity Secret (kubeconfig for the target Harvester cluster)
- SSH KeyPair created on Harvester
- VM image uploaded to Harvester (SLES 15 SP7 or openSUSE Leap 15.6 recommended)
- IPPool configured on Harvester (for automatic IP allocation)

> **Upgrading to v0.5.x**: the API graduated from `v1alpha1` to `v1beta1` - see
> [docs/migration-v0.4-to-v0.5.md](docs/migration-v0.4-to-v0.5.md). Existing `v1alpha1`
> objects keep working (served + converted), new manifests should use `v1beta1`.
>
> **Upgrading from v0.2.x**: see [docs/migration-v0.2-to-v0.3.md](docs/migration-v0.2-to-v0.3.md)
> - v0.3.0+ requires the CAPI v1.12 / v1beta2 ecosystem and is **not backward-compatible**
> with managers running CAPI v1.10.

### Enabling RKE2 providers under Rancher Turtles

Rancher Turtles only deploys `cluster-api` core by default. To use CAPHV, enable
the RKE2 providers explicitly:

```yaml
apiVersion: turtles-capi.cattle.io/v1alpha1
kind: CAPIProvider
metadata:
  name: rke2-bootstrap
  namespace: rke2-bootstrap-system
spec:
  name: rke2
  type: bootstrap
---
apiVersion: turtles-capi.cattle.io/v1alpha1
kind: CAPIProvider
metadata:
  name: rke2-control-plane
  namespace: rke2-control-plane-system
spec:
  name: rke2
  type: controlPlane
```

## Installation

### Option 1: CAPIProvider via Rancher Turtles (recommended for production)

```yaml
apiVersion: turtles-capi.cattle.io/v1alpha1
kind: CAPIProvider
metadata:
  name: harvester
  namespace: caphv-system
spec:
  name: harvester
  type: infrastructure
  version: v0.10.1
  fetchConfig:
    url: https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.10.1/infrastructure-components.yaml
  configSecret:
    name: caphv-variables
```

See [docs/operations.md](docs/operations.md) for full CAPIProvider deployment, upgrade, and migration instructions.

### Option 2: Helm Chart (legacy)

> **Note**: The Helm chart is maintained for compatibility but is not the recommended
> installation method. Prefer CAPIProvider via Rancher Turtles (Option 1) or `clusterctl init`
> for production deployments. The chart may be removed in a future release.

```bash
# Without webhooks
helm install caphv chart/caphv/ \
  -n caphv-system --create-namespace \
  --set image.repository=ghcr.io/harvester/cluster-api-provider-harvester \
  --set image.tag=v0.10.1

# With webhooks (requires cert-manager)
helm install caphv chart/caphv/ \
  -n caphv-system --create-namespace \
  --set image.repository=ghcr.io/harvester/cluster-api-provider-harvester \
  --set image.tag=v0.10.1 \
  --set webhooks.enabled=true \
  --set webhooks.certManager.enabled=true
```

### Option 3: Kustomize

```bash
# Build and push the image
make docker-build docker-push IMG=ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1

# Deploy
make deploy IMG=ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1
```

### Option 4: Manual (standalone manifests)

```bash
kubectl apply -f out/infrastructure-components.yaml
```

## Quick Start (ClusterClass - recommended)

Using ClusterClass reduces cluster creation from ~200 lines to ~30 lines of YAML.

### 1. Install the ClusterClass (once per management cluster)

```bash
# Via Helm (with controller)
helm install caphv chart/caphv/ \
  -n caphv-system --create-namespace \
  --set clusterClass.enabled=true

# Or standalone
kubectl apply -f templates/clusterclass/rke2/clusterclass-harvester-rke2.yaml
```

### 2. Generate cluster manifests with the CLI

```bash
# Generate all manifests
bin/caphv-generate \
  --name my-cluster \
  --image "default/my-vm-image.qcow2" \
  --ssh-keypair "default/my-ssh-key" \
  --network "default/my-vm-network" \
  --gateway 10.0.0.1 \
  --subnet-mask 255.255.255.0 \
  --ip-pool my-ip-pool \
  --dns 10.0.0.53 \
  --harvester-kubeconfig ~/.kube/harvester.yaml \
  > cluster.yaml

# Or interactive mode
bin/caphv-generate --interactive

# Apply
kubectl apply -f cluster.yaml
# Or directly: bin/caphv-generate [...] --apply
```

The CLI generates: Namespace, Secret, Cluster (topology), ConfigMaps (CCM/CSI/Calico), ClusterResourceSets, and MachineHealthCheck.

### Fleet Mode (optional - GitOps addon management)

With CAAPF installed, addons can be managed via Fleet instead of CRS:

```bash
bin/caphv-generate \
  --name my-cluster \
  --cni calico --cni-mtu 1450 --cni-encapsulation VXLAN \
  --pod-cidr 10.244.0.0/16 \
  --fleet-addon-repo https://my-gitea/org/caphv-fleet-addons.git \
  --image "default/my-vm-image.qcow2" \
  --ssh-keypair "default/my-ssh-key" \
  --network "default/my-vm-network" \
  --gateway 10.0.0.1 --subnet-mask 255.255.255.0 \
  --ip-pool my-ip-pool \
  --harvester-kubeconfig ~/.kube/harvester.yaml \
  --apply
```

See [docs/fleet-addons.md](docs/fleet-addons.md) for full documentation.

### 3. Monitor cluster creation

```bash
kubectl get cluster,machine,harvestermachine -n my-cluster
```

## User Experience Summary

### Prerequisites (one-time setup)

On the management cluster:
- Rancher + Turtles installed
- CAPHV deployed via Helm (`clusterClass.enabled=true`)
- Rancher `cacerts` setting configured (required for Turtles strict TLS mode with external TLS termination)
- Harvester kubeconfig available locally

### Create a cluster

**Interactive mode (guided):**
```bash
caphv-generate --interactive
```
The script asks ~15 questions with sensible defaults, then generates and applies everything.

**Flags mode (scriptable):**
```bash
caphv-generate \
  --name my-cluster \
  --cp-replicas 3 --worker-replicas 2 \
  --image "default/my-vm-image.qcow2" \
  --ssh-keypair "default/my-ssh-key" \
  --network "default/my-vm-network" \
  --gateway 10.0.0.1 --subnet-mask 255.255.255.0 \
  --ip-pool my-ip-pool --dns 10.0.0.53 \
  --harvester-kubeconfig ~/.kube/harvester.yaml \
  --apply
```

### What happens automatically (~16 min)

1. **Namespace** created
2. **Secret** with Harvester kubeconfig injected
3. **ClusterClass** + templates deployed in the namespace
4. **Cluster topology** created - CAPI orchestrates everything:
   - VMs created on Harvester (IPs allocated from IPPool)
   - RKE2 bootstrap (control plane then workers)
   - Cloud-init with static IP, iptables, SSH
   - Cloud provider + CSI Harvester installed via ClusterResourceSets
   - MachineHealthCheck active (auto-remediation)
5. **Rancher** detects the cluster (auto-import label) - deploys agent - cluster visible in the UI

### Result

- Fully functional Kubernetes cluster (RKE2)
- Visible and manageable in Rancher UI
- Auto-remediation: if a VM dies, it is automatically recreated (~9 min)
- Rolling upgrade: change the K8s version in the Cluster spec - rolling update CP then workers

### Day 2 Operations

- **Scale**: modify `replicas` in the Cluster spec
- **Upgrade K8s**: modify `version` in the Cluster spec
- **Delete**: `kubectl delete cluster my-cluster -n my-namespace` - everything is cleaned up (VMs, PVCs, secrets)

## Quick Start (manual - full control)

### 1. Create the identity Secret

```bash
kubectl create secret generic hv-identity-secret \
  -n <namespace> \
  --from-file=kubeconfig=<path-to-harvester-kubeconfig>
```

### 2. Create a Cluster

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: my-cluster
  namespace: my-namespace
spec:
  clusterNetwork:
    pods:
      cidrBlocks: [10.52.0.0/16]
    services:
      cidrBlocks: [10.53.0.0/16]
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta2
    kind: RKE2ControlPlane
    name: my-cluster-cp
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: HarvesterCluster
    name: my-cluster-hv
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: my-cluster-hv
  namespace: my-namespace
spec:
  targetNamespace: default
  identitySecret:
    name: hv-identity-secret
    namespace: my-namespace
  loadBalancerConfig:
    ipamType: pool
  vmNetworkConfig:
    gateway: "10.0.0.1"
    subnetMask: "255.255.255.0"
    ipPoolRef: default/my-ip-pool
```

### 3. Define Machine Templates

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterMachineTemplate
metadata:
  name: my-cluster-machine
  namespace: my-namespace
spec:
  template:
    spec:
      cpu: 2
      memory: "4Gi"
      sshUser: sles
      sshKeyPair: default/my-ssh-key
      volumes:
        - volumeType: image
          imageName: default/my-vm-image.qcow2
          volumeSize: "40Gi"
          bootOrder: 1
        - volumeType: storageClass     # optional: additional data disk
          storageClass: longhorn
          volumeSize: "10Gi"
      networks:
        - default/production
```

### 4. Create Control Plane + Workers

See [templates/](templates/) for complete RKE2 cluster template examples.

## Architecture

```
Management Cluster (RKE2)
├── CAPI Core Controller
├── RKE2 Bootstrap Controller
├── RKE2 ControlPlane Controller
├── CAPHV Controller  ◄── this project
│   ├── HarvesterCluster reconciler
│   ├── HarvesterMachine reconciler
│   │   ├── IP allocation from IPPool
│   │   ├── VM creation (multi-disk, cloud-init, static IP)
│   │   ├── Cloud provider bootstrap (hostNetwork fix)
│   │   ├── Node init (providerID + taint removal)
│   │   └── etcd cleanup on CP deletion
│   └── Validating webhooks (optional)
└── Rancher Turtles (optional, auto-import)

Harvester HCI (target)
├── VMs (created by CAPHV)
├── IPPool (IP allocation)
├── VM Images (boot disks)
└── Longhorn (storage)
```

## Configuration Reference

### HarvesterCluster

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.targetNamespace` | string | Yes | Namespace on Harvester for VMs |
| `spec.identitySecret.name` | string | Yes | Secret containing Harvester kubeconfig |
| `spec.identitySecret.namespace` | string | Yes | Namespace of identity secret |
| `spec.loadBalancerConfig.ipamType` | string | Yes | `pool` or `dhcp` |
| `spec.vmNetworkConfig.gateway` | string | Yes* | Gateway IP (*required for pool IPAM) |
| `spec.vmNetworkConfig.subnetMask` | string | Yes* | Subnet mask (e.g. "255.255.0.0") |
| `spec.vmNetworkConfig.ipPoolRef` | string | No | Reference to a Harvester IPPool |
| `spec.vmNetworkConfig.ipPoolRefs` | []string | No | Multiple pools, ordered fallback + network-aware selection (a pool whose `selector.network` matches one of the machine's networks) |
| `spec.vmNetworkConfig.ipPool` | object | No | Inline IPPool to create in Harvester (cluster level only) |

> **DHCP mode**: If `vmNetworkConfig` is omitted and no machine-level `networkConfig` is set, all VM NICs will use DHCP automatically. No IPPool or static IP configuration is needed.

### HarvesterMachine

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `spec.cpu` | int | Yes | Number of CPU cores (must be > 0) |
| `spec.memory` | string | Yes | Memory (e.g. "4Gi") |
| `spec.sshUser` | string | Yes | SSH user for cloud-init |
| `spec.sshKeyPair` | string | Yes | Harvester SSH KeyPair reference |
| `spec.volumes` | []Volume | Yes | At least one volume required |
| `spec.networks` | []string | Yes | At least one network required |
| `spec.volumes[].volumeType` | string | Yes | `image` or `storageClass` |
| `spec.volumes[].imageName` | string | For image | Harvester VM image (namespace/name) |
| `spec.volumes[].storageClass` | string | For SC | Storage class for blank disk |
| `spec.volumes[].volumeSize` | string | Yes | Disk size (e.g. "40Gi") |
| `spec.volumes[].bootOrder` | int | No | Boot priority (1 = first) |
| `spec.networkConfig` | object | No | Static IP configuration for this machine (address, gateway, DNS) |
| `spec.vmNetworkConfig` | object | No | Machine-level pool-based network config overriding the cluster-level one (pools, gateway, subnet mask, DNS); mutually exclusive with `networkConfig` |
| `spec.firmware` | object | No | `efi` and `secureBoot` (Secure Boot requires EFI) |
| `spec.tpm` | object | No | `enabled` and `persistent` emulated TPM device |
| `spec.nodeAffinity` / `spec.workloadAffinity` | object | No | VM scheduling constraints on Harvester hosts / other workloads |
| `spec.failureDomain` | string | No | Failure domain to pin the VM to (normally set by CAPI from the published domains) |

## Monitoring

CAPHV exposes custom Prometheus metrics (`caphv_*` namespace) via the controller-runtime metrics endpoint (port 8080, protected by kube-rbac-proxy).

A ServiceMonitor is included in the kustomize build. A ready-to-import Grafana dashboard is at `config/grafana/caphv-dashboard.json`.

Key metrics: `caphv_machine_create_total`, `caphv_machine_creation_duration_seconds`, `caphv_machine_status`, `caphv_ippool_allocations_total`, `caphv_cluster_ready`, `caphv_etcd_member_remove_total`, `caphv_node_init_duration_seconds`.

See [docs/operations.md](docs/operations.md) for the full metrics list and alerting recommendations.

## Documentation

- [Operations Guide](docs/operations.md) - installation via CAPIProvider, cluster lifecycle, multi-pool and per machine type networking, encrypted storage, failure domains, Secure Boot/vTPM, monitoring, backup/DR
- [Compliance Guide](docs/compliance.md) - BSI APP.4.4, CIS, DISA STIG, FIPS and ANSSI mapping per layer, with the provider switches to use
- [Compatibility Matrix](docs/compatibility.md) - certified CAPHV / Rancher / Turtles / CAPI pairings
- [Fleet Addons Guide](docs/fleet-addons.md) - Fleet/CAAPF addon management for CSI and CNI
- [Troubleshooting](docs/troubleshooting.md) - IPPool, cloud-init, DHCP, Turtles/Rancher, VM creation, etcd
- [Release Process](docs/release-process.md) - immutable release flow, assets, certification bump checklist
- [Maintainer Notes](docs/maintainer-notes.md) - invariants, ecosystem behaviors, debugging techniques and known limitations for maintainers
- Migration guides: [v0.2 to v0.3](docs/migration-v0.2-to-v0.3.md), [v0.4 to v0.5](docs/migration-v0.4-to-v0.5.md)

## E2E Tests

Integration tests run against a live Harvester + CAPI cluster:

```bash
./test/e2e/run-e2e.sh              # Run all (18 tests, ~30min)
./test/e2e/run-e2e.sh webhook      # Validation tests (~10s)
./test/e2e/run-e2e.sh scale        # Scale up/down (~7min)
./test/e2e/run-e2e.sh multidisk    # Multi-disk VM (~7min)
./test/e2e/run-e2e.sh remediation  # MHC auto-remediation (~14min)
```

## Building

```bash
# Build binary
make build

# Build container image
make docker-build IMG=ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1

# Run unit tests
make test
```

## Verifying release artifacts

Container images and SLSA build provenance are produced by
`.github/workflows/release.yml` and signed keylessly with
[cosign](https://docs.sigstore.dev/) using the GitHub Actions OIDC
identity.

> **Requires cosign v3.0.0 or newer.** Releases v0.2.9+ are signed in
> the OCI 1.1 bundle format used by cosign v3. Older cosign v2 clients
> report `no signatures found` against these images - install cosign
> v3 to verify.

Verify the container image:

```bash
cosign verify ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1 \
  --certificate-identity-regexp "^https://github.com/harvester/cluster-api-provider-harvester" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Verify the SLSA build provenance with the GitHub CLI:

```bash
gh attestation verify \
  oci://ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1 \
  --owner harvester
```

Both checks confirm the image was built from this repository's
`release.yml` workflow at the matching tag.

## Testing & compatibility

The provider is continuously validated by the suites in
[`test/certification/`](test/certification/): a nightly version-pairing tier, a full
Rancher-stack tier, and the Turtles integration suite (`CreateUsingGitOpsSpec`)
running twice a week against a real Harvester on a self-hosted runner. The
CAPHV ↔ Rancher/Turtles/CAPI pairing matrix lives in
[docs/compatibility.md](docs/compatibility.md).

## Release History

| Version | Date | Key changes |
|---------|------|-------------|
| v0.10.1 | 2026-07-28 | Release assets now include the cluster template flavors and the ClusterClass (`clusterctl generate cluster --flavor ...` works against releases) |
| v0.10.0 | 2026-07-28 | `cisProfile` and `fipsRequired` ClusterClass hardening switches, STIG-hardened template flavor, compliance guide; fix: requeue until the workload node is initialized |
| v0.9.0 | 2026-07-28 | Failure domains: discovery from Harvester hosts/zones, publication for the CAPI contract, placement via node affinity (#237) |
| v0.8.0 | 2026-07-28 | UEFI Secure Boot and vTPM options per machine type (#238) |
| v0.7.0 | 2026-07-27 | Per machine type network configuration: machine-level `vmNetworkConfig` (#234) |
| v0.6.1 | 2026-07-27 | Network-aware IP pool selection for `ipPoolRefs`; webhook accepts list-only pool sources |
| v0.6.0 | 2026-07-20 | Build ecosystem on CAPI v1.13.4 / controller-runtime v0.23.3; typed admission validators |
| v0.5.2 | 2026-07-20 | Image StorageClass resolved from the image status (fixes PVC Pending on freshly created images, #211); immutable-releases-compatible release flow (#218); kubevirt.io/api 1.8.4, harvester-load-balancer 1.8.1, Go 1.26, security bumps |
| v0.5.1 | 2026-07-17 | cert-manager CA injection restored on the clustertemplates CRD (required for installs through a Turtles CAPIProvider); RBAC for provisioning.cattle.io; Turtles integration suite (CreateUsingGitOpsSpec) on a self-hosted runner |
| v0.5.0 | 2026-07-06 | API graduation v1alpha1 -> v1beta1 (hub with conversion webhooks, fuzz-tested round-trip); deprecated failure fields dropped |
| v0.4.0 | 2026-07-06 | Complete v1beta2 CAPI contract: initialization.provisioned, Paused condition, FailureReason/Message deprecation |
| v0.3.0 | 2026-05-30 | CAPI v1.12 / v1beta2 ecosystem migration: cluster-api v1.12.x, controller-runtime v0.22.5, k8s.io v0.34, metav1.Condition, RKE2 templates v1beta2. Validated against Harvester v1.8 + Rancher 2.14 + Turtles 0.26 |
| v0.2.9 | 2026-04-15 | Supply chain hardening: cosign keyless signing, SLSA provenance, SBOM, GitHub Actions SHA-pinning, hadolint, least-privilege workflow permissions |
| v0.2.8 | 2026-03-16 | CAPI contract compliance fixes, kustomize verify, v1beta2 readiness docs, Helm chart deprecation note |
| v0.2.7 | 2026-03-10 | Code quality fixes for SURE-11421 review: kustomize modernization, finalizer naming conventions, context propagation |
| v0.2.6 | 2026-03-09 | CSI decoupling, Fleet label automation, Fleet CSI bundle |
| v0.2.5 | 2026-03-08 | Fleet/CAAPF addon management, CNI configuration flags |
| v0.2.4 | 2026-03-08 | CAPIProvider in Turtles, P0 milestone complete |
| v0.2.3 | 2026-03-07 | DHCP VM support, multi-NIC cloud-init |
| v0.2.1 | 2026-03-06 | ClusterClass (harvester-rke2), CLI generator (caphv-generate), Helm ClusterClass option |
| v0.2.0 | 2026-03-06 | Harvester v1.7.1, multi-disk, IPPool, webhooks, auto-remediation, e2e tests |
| v0.1.6 | 2024-xx-xx | Upstream: initial CAPI contract, single disk, DHCP only |

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
