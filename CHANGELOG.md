# Changelog

All notable changes to this project are documented in this file.

## [Unreleased]

### Fixed

- **Harvester v1.9 endpoint lookup**: the cloud provider kubeconfig was built
  from the VIP published on the `kube-system/ingress-expose` Service, which
  Harvester v1.9 no longer ships, so every cluster failed with "unable to
  compute the Harvester Endpoint". When `ingress-expose` does not exist, the
  VIP is now read from the LoadBalancer address of `kube-system/rke2-traefik`,
  extracted exactly as it was from `ingress-expose`. Older Harvester releases
  keep the previous behavior.
- **Machines stuck in Provisioning with the v1beta2 contract**: the CRDs
  declare the `cluster.x-k8s.io/v1beta2` contract, so the CAPI core (v1.13)
  reads `status.initialization.provisioned` of a HarvesterMachine. The end
  of a successful reconcile set `status.ready` without it, leaving machines
  with `ready: true` and an empty `initialization` forever. The field is now
  mirrored from `status.ready` where the status is patched, as the
  HarvesterCluster already did; existing stuck machines recover on their next
  reconcile.
- **`caphv-generate` emits the `v1beta1` API**: the generator still wrote the
  HarvesterClusterTemplate, the HarvesterMachineTemplates and every ClusterClass
  reference and patch selector at the deprecated `v1alpha1`. Templates created
  through `v1alpha1` were stored with an empty `spec.template.metadata`, and the
  CAPI topology controller then failed to clone them ("spec.template.metadata in
  body should have at least 1 properties"). All Harvester objects and references
  now use `infrastructure.cluster.x-k8s.io/v1beta1`, the stored version.
- **Template conversion no longer writes an empty `spec.template.metadata`**:
  the conversion webhook serialized an unset template metadata as `{}`, which
  the CRD schema rejects (`minProperties: 1`). Any template created or read
  through `v1alpha1` (by users, GitOps sources or CAPI itself) therefore broke
  template cloning. The field is now omitted when empty, as in the CAPI types.
  Templates already stored with the empty object keep it until they are
  recreated.

## [v0.10.1] - 2026-07-28

### Fixed

- **Cluster template flavors shipped as release assets**: the release workflow
  only uploaded `infrastructure-components.yaml` and `metadata.yaml`, so the
  cluster template flavors (including the new `stig` one) and the ClusterClass
  were never downloadable and `clusterctl generate cluster --flavor ...` could
  not work against a release. All `out/` artifacts are now attached to the
  release draft. Releases are immutable on this repository, so the missing
  assets could not be added to v0.10.0 retroactively; use this release instead.

## [v0.10.0] - 2026-07-28

### Fixed

- **Workload node initialization requeue**: a machine whose node registered in
  the workload cluster after the machine's last event-driven reconcile stayed
  in the `Provisioned` phase forever: the node initialization (providerID,
  uninitialized taint removal) silently failed and nothing triggered another
  reconcile. The reconciler now requeues until the node is initialized.
  Clusters with a MachineHealthCheck were masking the gap (its probes kept
  generating events); the shipped ClusterClass has none and exposed it.

### Added

- **FIPS enforcement switch in the ClusterClass**: the new `fipsRequired`
  topology variable (default `false`) injects a bootstrap guard that refuses
  to start a node whose image does not run in FIPS mode
  (`/proc/sys/crypto/fips_enabled`), so FIPS clusters cannot silently run on
  non-FIPS images. FIPS mode itself comes from the node image; the compliance
  guide documents the image build recipe. The certification suite gains the
  matching `FIPS_REQUIRED` knob, and the CIS and FIPS node preparation
  commands now compose in a single patch.
- **STIG-hardened cluster template flavor** (`cluster-template-stig.yaml`):
  a ready-to-use flavor aligning the RKE2 side of the cluster with the DISA
  RKE2 STIG guidance: CIS profile with injected node prerequisites, audit
  logging (metadata-level policy, retention flags), TLS 1.2 minimum and idle
  streaming timeouts on the API server and kubelets, restricted pod security
  admission with the Rancher ecosystem exemptions, and node VMs booting with
  UEFI Secure Boot and a persistent vTPM. OS-level STIG items stay with the
  node image.
- **CIS hardening switch in the ClusterClass**: the new `cisProfile` topology
  variable enables the RKE2 CIS profile on every node of the cluster and
  injects the node prerequisites the hardened kubelet requires (the `etcd`
  system user and the protect-kernel-defaults sysctls) through
  `preRKE2Commands`. Empty by default: existing clusters are unaffected.
  The certification suite gains the matching `CIS_PROFILE` knob.

## [v0.9.0] - 2026-07-28

### Added

- **Failure domains** ([#237](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/237)):
  the provider now discovers the failure domains of the target Harvester cluster
  (one per host `topology.kubernetes.io/zone` label when every host carries one,
  one per host otherwise), publishes them in
  `HarvesterCluster.status.failureDomains` for the CAPI contract, and pins each
  machine to its assigned domain with a required node affinity, so control plane
  machines spread across Harvester hosts or zones. No configuration is needed;
  single host clusters publish one domain and behave as before.

## [v0.8.0] - 2026-07-28

### Added

- **UEFI Secure Boot and vTPM options** ([#238](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/238)):
  `spec.firmware` (`efi`, `secureBoot`) and `spec.tpm` (`enabled`, `persistent`) on
  HarvesterMachine let each machine type boot with UEFI firmware, Secure Boot
  (with the required SMM feature) and an emulated TPM device, the building
  blocks for measured or attested cluster nodes. VMs without these blocks keep
  booting with the current defaults; `secureBoot` requires `efi` (webhook
  enforced).

## [v0.7.0] - 2026-07-27

### Added

- **Per machine type network configuration** ([#234](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/234)):
  `spec.vmNetworkConfig` can now be set on a HarvesterMachine (typically through a
  HarvesterMachineTemplate) to fully replace the cluster-level configuration for that machine
  type: IP allocation uses the pools referenced there and the cloud-init static network
  configuration uses its gateway, subnet mask and DNS settings. Combined with network-aware
  pool selection, this lets control-plane and worker nodes live in separate networks with
  different subnets. Machine types without the override keep the cluster-level behavior.
  The machine-level variant accepts pool references only (no inline `ipPool`) and is mutually
  exclusive with the static per-machine `networkConfig`.

## [v0.6.1] - 2026-07-27

### Added

- **Network-aware IP pool selection**: when `ipPoolRefs` lists several pools, a pool
  whose Harvester `spec.selector.network` designates a VM network is only used for
  machines attached to that network (a pool with no selector network keeps matching
  any machine). Separate address ranges for control-plane and worker networks now
  work with a single pool list, and a machine whose networks match no listed pool
  gets an explicit error instead of an address from another network.

### Fixed

- The HarvesterCluster webhook validation accepts `ipPoolRefs` as a pool source
  (it still required `ipPoolRef` or `ipPool` and rejected list-only specs).

## [v0.6.0] - 2026-07-20

### Changed

- **Build ecosystem moved to CAPI v1.13.4 / controller-runtime v0.23.3 / k8s.io
  v0.35.4**, aligning with what the next Turtles/Rancher will ship (Turtles main is
  already on CAPI v1.13). The published CRDs and CAPI contracts are unchanged: the
  provider keeps running against the CAPI v1.12 core deployed by Rancher 2.14.x,
  validated end to end by the Turtles integration suite on a real Harvester.
- The webhook setup adopts controller-runtime v0.23's generic builder.
- `metadata.yaml` declares the 0.6 release series (clusterctl refuses versions
  missing from the series list).

## [v0.5.2] - 2026-07-20

### Fixed

- **Image StorageClass resolution** ([#211](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/211)):
  the StorageClass of an "image" volume is now read from
  `VirtualMachineImage.status.storageClassName` instead of being composed as
  `longhorn-<image>`. Harvester names image StorageClasses `lh-<uuid>` since a recent
  release, so the composed name did not exist and the PVC stayed `Pending`
  (`failed to get storage class longhorn-image-xxxxx`). This affects any freshly
  created image, independently of the Harvester version; images created before the
  naming change keep working. The old convention remains as a fallback for images
  whose status is not populated yet.

### Changed

- The release workflow now publishes the GitHub release as the last step, from a
  draft carrying all assets, so releases and tags can be made immutable
  ([#218](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/218)).
  Re-trigger a missed release with `gh workflow run release.yml --ref vX.Y.Z`:
  deleting and re-pushing a tag is no longer supported.
- Dependency bumps: `kubevirt.io/api` 1.8.4, ginkgo 2.32.0 / gomega 1.42.1,
  `golang.org/x/crypto` and `golang.org/x/net` (security advisories), bci/golang 1.26
  and several GitHub Actions.

## [v0.5.1] - 2026-07-17

### Fixed

- **cert-manager CA injection on the `harvesterclustertemplates` CRD**: the release
  manifests shipped an unsubstituted `CERTIFICATE_NAMESPACE/CERTIFICATE_NAME`
  annotation. Harmless with plain `kubectl apply`, but Turtles parses the components
  and fails on the placeholder, so installing v0.5.0 through a `CAPIProvider` could
  not complete.
- **RBAC for `clusters.provisioning.cattle.io`**: the fleet-integration path lists
  Rancher provisioning clusters and was denied under a real Rancher.
- The CAPI contract labels are no longer part of the controller Deployment and
  Service selectors (`includeSelectors: false`): selectors are immutable, so every
  contract-label change used to break `kubectl apply` upgrades (and could leave the
  webhook Service without endpoints). Selectors are now the minimal
  `control-plane: controller-manager` and will stay stable across releases.
  **One-time upgrade note**: applying these manifests over an existing install with
  `kubectl apply` requires deleting the controller Deployment once
  (`kubectl -n caphv-system delete deploy caphv-controller-manager`) before applying;
  `clusterctl`/CAPI-operator-managed installs are unaffected.

## [v0.5.0] - 2026-07-06

### Added - API graduation to v1beta1

The CAPHV API graduates from `v1alpha1` to `v1beta1` (see
`docs/migration-v0.4-to-v0.5.md`):

- New `api/v1beta1` package; all four CRDs serve `v1alpha1` + `v1beta1` with
  `v1beta1` as the stored version and hub. Conversion is served by the existing
  webhook server and proven lossless by fuzz round-trip tests.
- `v1alpha1` is deprecated but stays served for at least two more minor releases.
- The deprecated `status.failureReason`/`status.failureMessage` fields do not exist
  in `v1beta1` and are not carried across conversion (unset since v0.4.0; failures
  surface through the conditions).
- Validation webhooks run on both versions; the `HarvesterClusterTemplate` CRD gains
  the conversion/cainjection patches the other CRDs already had.
- In-repo templates, the Helm-shipped ClusterClass and the samples now reference
  `v1beta1`.

## [v0.4.0] - 2026-07-06

### Added - v1beta2 contract on the publish side

Until now CAPHV *consumed* the CAPI v1.12 / v1beta2 APIs but still *published*
the v1beta1 contract: `HarvesterCluster` signalled readiness only through
`status.ready`, and the CRDs carried only the `cluster.x-k8s.io/v1beta1`
contract label. CAPI reads the provisioned field from whichever path the CRD's
contract label selects (`status.ready` for v1beta1,
`status.initialization.provisioned` for v1beta2). Once CAPI drops the
deprecated v1beta1 contract, a `HarvesterCluster` that never sets
`status.initialization.provisioned` would stop being seen as provisioned and
clusters would hang in provisioning.

- `HarvesterCluster.status.initialization.provisioned` is now published and
  kept in sync with `status.ready` at the single status-patch point, so one
  `v1alpha1` object satisfies both the v1beta1 and v1beta2 contracts.
  (`HarvesterMachine` already published this field.)
- Added the `cluster.x-k8s.io/v1beta2: v1alpha1` contract label to the CRDs.

  > Validated end-to-end on a v0.3.x management cluster against Harvester
  > 1.8.0: with the v1beta2 label active, a freshly created cluster's
  > `Cluster.status.initialization.infrastructureProvisioned` transitions to
  > `true` via the v1beta2 read path, and existing clusters are unaffected.

- `HarvesterCluster` and `HarvesterMachine` now publish the `Paused` condition
  (v1beta2 contract) whenever the owner `Cluster` or the object itself is
  paused, and pausing freezes the object state instead of flipping
  `HarvesterMachine.status.ready` to `false`. Pause/unpause transitions on the
  `Cluster` trigger an immediate reconciliation (new watch); the
  pause-filtering event predicates were removed accordingly.

### Deprecated

- `HarvesterCluster.status.failureReason`/`.failureMessage` and
  `HarvesterMachine.status.failureReason`/`.failureMessage`: the CAPI v1beta2
  contract removed terminal failures - failures surface through the
  conditions instead. The controller no longer sets these fields; they will
  be dropped at the next API version.

### Security

- Bumped `go.opentelemetry.io/otel/sdk` to v1.43.0 (and `otel`/`otel/trace`),
  clearing the transitive advisory affecting the in-tree version.

## [v0.3.1] - 2026-06-15

### Fixed

- **`metadata.yaml` missing the `0.3` release series**: clusterctl maps a
  provider version to a CAPI contract via `metadata.yaml`. The `0.3` series
  was never added, so `clusterctl init --infrastructure harvester:v0.3.0`
  failed with *"version v0.3.0 ... does not match any release series.
  Available series: [0.2, 0.1]"* - blocking every clusterctl-based (i.e.
  no-Rancher) install. Added `0.3 -> v1beta2`. Verified end-to-end:
  `clusterctl init` brings up CAPI v1.12.8 + RKE2 v0.25.0 + CAPHV on a
  fresh cluster (all controllers Running, webhook Ready).

  > v0.3.1 is functionally identical to v0.3.0 (same controller image and
  > manifests); only the clusterctl metadata mapping is corrected. Existing
  > v0.3.0 deployments do not need to upgrade unless they install via
  > clusterctl.

### Documentation

- Added an **"Installation without Rancher"** section to
  `docs/operations.md` covering two validated paths - clusterctl init and
  raw manifests (with `${VAR:=default}` rendering) - for running CAPHV on a
  management cluster without Rancher/Turtles.

## [v0.3.0] - 2026-05-30

### ⚠️ Breaking - ecosystem upgrade required

CAPHV v0.3.0 targets the Cluster API v1.12 / v1beta2 ecosystem. The
management cluster must run **all** of the following before installing
v0.3.0:

- Cluster API core v1.12.x (serves `cluster.x-k8s.io/v1beta2`)
- cluster-api-provider-rke2 v0.24.x+ (bootstrap and control plane,
  v1beta2 RKE2 CRDs - validated against v0.25.0)
- Kubernetes 1.34.x on the management cluster
- Go 1.24+ when building from source

Compatible dependent providers (validated):

- rancher/turtles v0.26.2
- Rancher Manager 2.14.1 (Turtles is shipped by default and must have
  rke2-bootstrap + rke2-control-plane CAPIProviders explicitly enabled)

Older managers (CAPI v1.10/v1beta1) will reject v0.3.0 because the
controller looks up Cluster objects via `cluster.x-k8s.io/v1beta2`.
Plan the upgrade in this order:

1. Bump CAPI core to v1.12.x
2. Bump RKE2 bootstrap/control-plane to v0.24.x+
3. Bump Turtles to v0.26.x
4. Upgrade CAPHV to v0.3.0 (this release)

### Changed

- **Dependencies**:
  - `sigs.k8s.io/cluster-api` v1.10.3 -> v1.12.8
  - `sigs.k8s.io/controller-runtime` v0.21.0 -> v0.22.5
  - `k8s.io/{api,apimachinery,client-go,...}` v0.33.2 -> v0.34.8 (pinned
    via `replace` directives to keep transitive deps off v0.36 / Go 1.26)
  - `k8s.io/kube-openapi` pin bumped to track apimachinery v0.34.8
  - Go directive in `go.mod`: 1.24.2 -> 1.25.0; Dockerfile builder image
    `registry.suse.com/bci/golang:1.24.5` -> `1.25`

- **CAPI API package layout**: all imports of
  `sigs.k8s.io/cluster-api/api/v1beta1` moved to
  `sigs.k8s.io/cluster-api/api/core/v1beta2` (upstream restructured `api/`
  into subpackages per group).

- **Conditions surface aligned with `k8s.io/apimachinery/pkg/apis/meta/v1`**:
  - `HarvesterCluster.Status.Conditions` and `HarvesterMachine.Status.Conditions`
    typed as `[]metav1.Condition`.
  - `GetConditions() []metav1.Condition` / `SetConditions(...)` satisfy
    `sigs.k8s.io/cluster-api/util/conditions.Setter`.
  - All condition constants typed as plain `string`.
  - Helpers `conditions.MarkTrue/MarkFalse/MarkUnknown` (legacy
    clusterv1) replaced with explicit `conditions.Set(obj,
    metav1.Condition{...})`. The implicit `Severity` field is dropped
    (not part of the k8s contract).

- **CAPI ObjectReference contract**:
  - `Cluster.Spec.InfrastructureRef` no longer carries `Namespace` (the
    infra reference is always in the same namespace as the Cluster).
  - `Cluster.Status.InfrastructureReady` removed; replaced by
    `Cluster.Status.Initialization.InfrastructureProvisioned *bool`.

- **CRD schemas**: regenerated by controller-gen - `status.conditions[]`
  now uses the standard k8s `metav1.Condition` schema (required
  `lastTransitionTime`, `message`, `reason`, `status`, `type`; optional
  `observedGeneration`).

### Fixed

- **Condition `Reason` values now conform to the v1beta2 regex**: nine
  reason constants on `HarvesterCluster` (Load Balancer, Custom IP Pool,
  Cloud Provider Config, and `InitMachineNotYetCreatedReason`) previously
  held free-form sentences. `metav1.Condition.Reason` is validated
  server-side against `^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`, so
  every status patch was rejected after the v1beta2 migration, causing
  reconcile failures and controller-runtime leader-election loss.
  Constants are now PascalCase identifiers; the human-readable text was
  already carried in each `conditions.Set` call's `Message` field.

### Templates and CLI

- `templates/clusterclass/rke2/clusterclass-harvester-rke2.yaml`,
  `templates/cluster-template-rke2*.yaml`, and the `bin/caphv-generate`
  script now emit `controlplane.cluster.x-k8s.io/v1beta2` and
  `bootstrap.cluster.x-k8s.io/v1beta2` for RKE2 templates. Core CAPI
  types (`Cluster`, `ClusterClass`, `ClusterResourceSet`,
  `MachineHealthCheck`) stay on `cluster.x-k8s.io/v1beta1` in the
  rendered YAML - CAPI v1.12 conversion webhooks transparently bridge
  to v1beta2 internals, keeping user-facing manifests stable.

### Validated end-to-end on a clean management cluster

- VM `mgmt-v030` on Harvester 1.8.0 (openSUSE Leap 15.6, RKE2
  v1.34.7+rke2r1) running:
  - Rancher Manager 2.14.1
  - Rancher Turtles 0.26.2
  - cert-manager v1.18.2
  - CAPI core v1.12.7, rke2-bootstrap v0.25.0, rke2-control-plane v0.25.0
  - CAPHV v0.3.0
- Webhooks: 5/5 e2e tests pass.
- Unit tests: 195/195 (`internal/controller`, `util`).
- ClusterClass topology reconciles successfully end-to-end:
  HarvesterCluster + HarvesterMachine + RKE2ControlPlane created from
  a `caphv-generate` manifest, VM provisioned on Harvester (KubeVirt
  1.7.0), IP allocated from `capi-vm-pool`, providerID assigned.

### Verifying release artifacts

`.github/workflows/release.yml` signs the multi-arch image keylessly with
cosign v3.0.6 using the GitHub Actions OIDC identity, attests SLSA build
provenance via `actions/attest-build-provenance`, and generates SBOM +
provenance through `docker/build-push-action`.

> **cosign v3.0.0+ is required to verify.** v0.2.9 and v0.3.0 are signed in
> the OCI 1.1 bundle format used by cosign v3; older cosign v2 clients
> report `no signatures found` against these images.

```bash
cosign verify ghcr.io/rancher-sandbox/cluster-api-provider-harvester:v0.3.0 \
  --certificate-identity-regexp "^https://github.com/rancher-sandbox/cluster-api-provider-harvester" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

gh attestation verify \
  oci://ghcr.io/rancher-sandbox/cluster-api-provider-harvester:v0.3.0 \
  --owner rancher-sandbox
```

## [v0.2.9] - 2026-04-15

### Security

- **Supply chain hardening (rancher/rancher-security#1667)**:
  - All GitHub Actions in `lint.yml`, `test.yml`, and `release.yml` pinned to full 40-char commit SHAs (no more tag references)
  - Replaced `curl HEAD/install.sh | bash` golangci-lint installer with the official `golangci/golangci-lint-action` (SHA-pinned to v9.2.0)
  - Added cosign keyless signing of multi-arch container images (OIDC via GitHub Actions identity, no secrets)
  - Added SLSA build provenance attestation (`actions/attest-build-provenance`) pushed to the registry
  - Enabled SBOM and provenance generation in `docker/build-push-action` (`sbom: true`, `provenance: mode=max`)
  - Added workflow-level and job-level least-privilege permissions across all CI workflows
  - Added `persist-credentials: false` to all `actions/checkout` invocations (defense-in-depth)
- **Build hygiene**: Restored missing `scripts/ci-lint-dockerfiles.sh` hadolint wrapper that the Makefile referenced but did not exist (pattern shared with cluster-api-provider-rke2 and rancher-turtles)

### Fixed

- **CI Lint workflow**: Upgraded `golangci-lint` from v2.2.1 to v2.11.1 to fix runtime panic on transitive dependencies declaring `go 1.26`
- **Workflow runtime**: Upgraded `azure/setup-helm` from v4.3.1 to v5.0.0 (Node.js 24, addresses GitHub deprecation warnings for Node 20 EOL 2026-09-16)

### Changed

- **Dependencies**: 8 Dependabot bumps merged (docker actions v3->v4/v5/v6/v7, ginkgo v2.28.1, gomega v1.39.1, prometheus/client_golang v1.23.2)
- **`.golangci.yml`**: Aligned `run.go` from `1.23` to `1.24` to match `go.mod`

### Verifying release artifacts

The container image and helm chart are signed with cosign using GitHub OIDC.

> **cosign v3.0.0+ is required.** This release was signed with cosign v3,
> which stores signatures in the OCI 1.1 bundle format; older cosign v2
> clients report `no signatures found`.

```bash
cosign verify ghcr.io/rancher-sandbox/cluster-api-provider-harvester:v0.2.9 \
  --certificate-identity-regexp "^https://github.com/rancher-sandbox/cluster-api-provider-harvester" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

A SLSA provenance attestation is attached to the registry alongside the
image and can be verified with:

```bash
gh attestation verify \
  oci://ghcr.io/rancher-sandbox/cluster-api-provider-harvester:v0.2.9 \
  --owner rancher-sandbox
```

## [v0.2.8] - 2026-03-16

### Fixed

- **CAPI contract compliance**: Added `ObjectMeta` to `HarvesterMachineTemplateResource` per InfraMachineTemplate contract; removed unused `HarvesterClusterTemplate.Status` field
- **Webhook clarity**: Removed standalone `examples/webhook-deployment.yaml` - webhooks are included in `infrastructure-components.yaml` and deployed automatically
- **Manifest verification**: Added `verify-manifests` target to `make verify` to catch out-of-sync generated manifests
- **Release artifacts**: Cluster templates (`cluster-template-*.yaml`) and ClusterClass (`clusterclass-harvester-rke2.yaml`) now included in release assets per clusterctl conventions
- **v1beta2 readiness**: Documented `status.initialization.provisioned` as forward-looking for CAPI v1beta2; `status.ready` remains the authoritative field for v1beta1
- **Helm chart deprecated**: Marked as legacy in favor of CAPIProvider/clusterctl installation
- **CAAPF note**: Added RFD 0051 advisory in Fleet addons documentation
- **Cleanup**: Removed unused `listImagesSelector` constant

## [v0.2.7] - 2026-03-10

### Fixed

- **Kustomize modernization**: Migrated config from deprecated `bases`, `patchesStrategicMerge`, `vars`, and `commonLabels` to `resources`, `patches`, `replacements`, and `labels` - eliminates 5 deprecation warnings from kustomize builds
- **Finalizer naming conventions**: Renamed finalizers to include `/finalizer` path segment per Kubernetes conventions (`harvestermachine.infrastructure.cluster.x-k8s.io/finalizer`, `harvestercluster.infrastructure.cluster.x-k8s.io/finalizer`); includes dual-finalizer migration for existing clusters
- **Context propagation**: Replaced all `context.TODO()` / `context.Background()` in production code with proper context propagation from reconciler scope
- **IP pool error handling**: Added error handling for `netip.AddrFromSlice()` calls in ippool.go
- **Typo fix**: Corrected "invalide" to "invalid" in ippool.go error message
- **Privileged container documentation**: Added comment explaining privileged container requirement in manager deployment
- **Regenerated infrastructure-components.yaml** without deprecation warnings

## [v0.2.6] - 2026-03-09

### Added

- **CSI decoupling**: Separated cloud-config Secret into its own ConfigMap (`cloud-config-addon-<name>`) and ClusterResourceSet, decoupling CSI driver from the controller - CSI can now be deployed via Fleet independently
- **Fleet label automation**: New `reconcileFleetIntegration()` automatically sets `fleetWorkspaceName: fleet-default` on Rancher management cluster and propagates CAPI labels (`cni`, `csi`, `cloud-config`, `cluster-name`) to Fleet cluster after Turtles import
- **Fleet CSI bundle**: CSI driver manifests in the Fleet addons repo (`/fleet/harvester-csi/`), deployed via Fleet GitRepo in Fleet mode
- **FleetIntegrationReady condition**: Tracks the status of Fleet label propagation (skipped for non-auto-import clusters)

### Changed

- `updateCloudProviderConfig` now points to `cloud-config-addon-<name>` (key: `cloud-config.yaml`) instead of `harvester-csi-driver-addon-<name>` (key: `harvester-cloud-provider-deploy.yaml`)
- CSI ConfigMap + CRS only generated in CRS mode (not Fleet mode); in Fleet mode, CSI is deployed via Fleet GitRepo
- Fleet mode GitRepo now includes `/fleet/harvester-csi` path in addition to CNI config
- Cluster labels: added `cloud-config: harvester` for CRS matching; Fleet mode uses `csi: harvester` (matches Fleet bundle selector)

## [v0.2.5] - 2026-03-08

### Added

- **Fleet/CAAPF addon management**: CNI configuration can now be deployed via Fleet GitOps instead of ClusterResourceSets - HelmChartConfig for Calico/Canal/Cilium tuning
- **CAAPF integration**: CAPIProvider manifest for Cluster API Addon Provider Fleet (v0.12.0, CAPI v1beta1 compatible)
- **CNI configuration flags**: `--pod-cidr`, `--cni-mtu`, `--cni-encapsulation`, `--cni-bgp` in caphv-generate
- **Fleet addon repository**: Separate Git repository (`caphv-fleet-addons`) with Fleet bundles for Calico, Canal, and Cilium HelmChartConfig
- **Fleet mode in caphv-generate**: `--fleet-addon-repo` generates a Fleet `GitRepo` with cluster-scoped targeting instead of CNI CRS
- **E2E fleet test suite**: 3 tests (CAAPF controller, Fleet mode generation, CRS retrocompatibility)

### Changed

- Cluster manifests now include `spec.clusterNetwork.pods.cidrBlocks` for pod CIDR configuration
- CCM and CSI remain in CRS mode regardless of addon mode (controller-coupled bootstrap dependency)

## [v0.2.3] - 2026-03-07

### Added

- **DHCP VM support**: VMs can now use DHCP instead of requiring static IP allocation from an IPPool - simply omit `vmNetworkConfig` and `networkConfig` to get DHCP on all NICs
- **Multi-NIC cloud-init**: Static mode network-config v1 generates entries for all NICs (eth0 static, eth1+ DHCP)

### Fixed

- **Wicked DHCP failure on KubeVirt**: Wicked's BPF filter uses link-layer offsets on `AF_PACKET SOCK_DGRAM`, but the kernel presents network-layer data to BPF, causing all DHCP responses to be silently dropped. Workaround: use ISC dhclient (`SOCK_RAW`/LPF) via cloud-init bootcmd
- **Cloud-init ordering**: dhclient-script created inline in `bootcmd` (not `write_files` which runs after `bootcmd`)
- **dhclient blocking**: Use `-1` flag (try once, fork to background) instead of `-d` (foreground) which blocked cloud-init

### Changed

- Split `buildNetworkData()` into `buildNetworkDataStatic()` (for static IP mode) and `buildDHCPCloudInit()` (for DHCP mode)
- Skip `networkdata` generation entirely in DHCP mode to prevent wicked nanny from overriding dhclient-assigned IPs

## [v0.2.1] - 2026-03-06

### Added

- **ClusterClass support**: Production-ready `harvester-rke2` ClusterClass with 12 variables and 12 patches, reducing cluster definitions from ~200 lines to ~30 lines of YAML
- **CLI generator** (`caphv-generate`): Interactive and flag-based tool to generate Cluster manifests from ClusterClass - outputs Namespace, Secret, Cluster topology, ConfigMaps, ClusterResourceSets, and MachineHealthCheck
- **Rancher auto-import via Turtles**: Label `cluster-api.cattle.io/rancher-auto-import=true` on generated Cluster manifests triggers automatic import into Rancher Manager through the Turtles operator
- **Helm ClusterClass integration**: `clusterClass.enabled=true` deploys the ClusterClass alongside the controller

### Fixed

- ClusterClass namespace scoping: ClusterClass must be in the same namespace as the Cluster (CAPI requirement)
- `controlPlaneEndpoint.host` must not be empty: use `"0.0.0.0"` as placeholder for dynamic IP allocation
- ClusterRole `configmaps` permission at cluster scope (controller cache requirement)

## [v0.2.0] - 2026-03-06

### Added

- **Harvester v1.7.1 compatibility**: Updated API clients, tested against Harvester v1.7.1 + Rancher v2.13.1
- **Multi-disk VM support**: Multiple volumes per VM (image-backed boot disk + storageClass data disks) with configurable boot order - fixes upstream limitation of single-disk only
- **IPPool-based IP allocation**: Automatic static IP assignment from Harvester IPPool resources, replacing manual or DHCP-only approaches
- **Network-config v1**: Cloud-init network configuration compatible with SLES/openSUSE (v2 not supported)
- **Cloud provider bootstrap**: Automatic `hostNetwork`, `dnsPolicy`, RBAC, and `not-ready` tolerations for the Harvester cloud controller manager - solves the Calico/cloud-provider chicken-and-egg problem
- **Node initialization from management cluster**: Automatic `providerID` and taint removal via management cluster, eliminating manual intervention
- **Automatic etcd member cleanup**: Removes stale etcd members when control-plane machines are deleted
- **Validating webhooks**: Admission webhooks for `HarvesterMachine` and `HarvesterCluster` resources - validates CPU, memory, sshUser, volumes, networks, targetNamespace, identitySecret, ipamType, vmNetworkConfig
- **E2E integration tests**: 18 tests across 4 suites (webhook, scale, multi-disk, remediation) running against a live cluster - addresses upstream issue [#91](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/91)
- **Helm chart**: Full Helm chart with support for webhooks, cert-manager integration, ClusterClass, and configurable resources
- **MachineHealthCheck**: Tested full auto-remediation cycle (VM deletion -> detection -> etcd cleanup -> replacement -> Ready in ~9 min)
- **Rolling K8s upgrade**: Tested control plane + worker rolling upgrade (v1.31.6 -> v1.31.14, ~35 min)

### Fixed

- **`memory.guest` on VM domain spec**: Set `memory.guest` for KubeVirt compatibility - fixes upstream issue [#139](https://github.com/rancher-sandbox/cluster-api-provider-harvester/issues/139)
- **`Store.Reserve()` not updating `Status.Allocated`**: Every machine got the same first IP from the pool
- **`CheckNamespacedName` regex missing underscore**: Image names containing `x86_64` failed parsing
- **CPU topology**: Set `sockets=1 threads=1` to prevent vCPU count multiplication
- **`spec.running` nil pointer dereference**: Harvester uses `runStrategy: Always` (not deprecated `running: true`); switched to `vm.RunStrategy()` method
- **Orphaned PVCs**: Delete VM first, then clean up PVCs by prefix
- **Cloud-init secret keys**: Must be lowercase (`userdata`/`networkdata`, not camelCase)

### Changed

- VM creation uses `spec.runStrategy: Always` instead of deprecated `spec.running: true`
- Webhooks use `admission.CustomValidator` interface (not `webhook.Validator`, removed in controller-runtime v0.21.0)

## [v0.1.6] - Upstream

See [upstream releases](https://github.com/rancher-sandbox/cluster-api-provider-harvester/releases) for v0.1.x changelog.
