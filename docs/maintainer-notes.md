# Maintainer notes

Hard-earned knowledge for whoever maintains this provider. Everything here was
paid for with a real debugging session; none of it is obvious from the code.
Read this before touching the release process, the ClusterClass, the
certification suites or the conversion layer.

## Invariants: do not break these

- **CAPI talks to this provider in `v1alpha1`.** The CRD contract labels map
  every CAPI contract (including `v1beta2`) to the `v1alpha1` provider API, so
  the core controllers and the RKE2 providers read and apply provider objects
  through the conversion webhook. Consequences: the `v1alpha1` package must
  keep converting losslessly (the fuzz round-trip tests in `api/v1alpha1/` are
  the guard; every new field needs its conversion mapping and a regenerated
  deepcopy), and a bug in conversion breaks *everything* while all unit tests
  stay green. Validate any API change against a live cluster, not just envtest.
- **Contract labels must never enter the Deployment/Service selectors.**
  Selectors are immutable: with the labels inside (as before v0.5.1), every
  contract change broke `kubectl apply` upgrades and could leave the webhook
  Service without endpoints. `includeSelectors: false` in the kustomization is
  load-bearing.
- **`metadata.yaml` needs a `releaseSeries` entry for every new minor, before
  tagging.** clusterctl and the CAPI operator refuse a version whose series is
  missing ("does not match any release series"), and releases are immutable so
  the asset cannot be fixed afterwards.
- **`release.yml` must attach every asset to the draft release.** Releases are
  immutable once published (rancher-security hardening): assets cannot be
  added, replaced or deleted, and tags cannot be moved. Publishing is the last
  step of the workflow, after the image and chart jobs. Re-trigger a missed
  release with `gh workflow run release.yml --ref vX.Y.Z`; never delete and
  re-push a tag. Verify signatures with cosign v3+ (v2 wrongly reports "no
  signatures" on OCI 1.1 bundles).
- **Always pin `secureBoot` explicitly when EFI is set.** KubeVirt defaults
  `secureBoot` to true as soon as an EFI bootloader is present; a user asking
  for plain UEFI would silently get Secure Boot (and an unbootable VM with an
  unsigned image). `applyFirmwareAndTPM` sets the value in both directions on
  purpose.
- **`metav1.Condition.Reason` must be PascalCase.** The API server enforces a
  regex; free-text reasons are rejected at patch time. Human text goes in
  `Message`.
- **Two ClusterClass patches must not add the same path.** Patches adding
  `preRKE2Commands` overwrite each other, which is why the CIS and FIPS node
  preparation live in a single combined `nodePreparation` patch with a
  conditional Go template. Extend that patch instead of adding a sibling.
- **Do not put `controlPlaneEndpoint` in topology cluster templates.** The
  topology controller's server-side apply reverts it and fights the provider.
- **Do not remove the node-init requeue** (`initializeWorkloadNode` returning
  `false` triggers `RequeueAfter`). Without it, a node that registers after the
  machine's last event-driven reconcile stays `Provisioned` forever. The gap is
  masked on clusters with a MachineHealthCheck (its probes keep generating
  events), which is exactly why it survived so long: the test suites all had an
  MHC. Symptom: `Ready` node with empty `spec.providerID`, silent logs.
- **Image StorageClasses come from `VirtualMachineImage.status.storageClassName`.**
  Recent Harvester names them `lh-<uuid>`; the legacy `longhorn-<image>`
  convention is only a fallback. The failure mode of getting this wrong is
  PVCs stuck `Pending` on freshly created images only, which no CI catches if
  the suites reuse old images.

## Behavior of the surrounding systems

- **CAPI never writes the failure domain onto the InfraMachine.** The control
  plane provider assigns it to `Machine.spec.failureDomain`; the InfraMachine's
  own `spec.failureDomain` is a deprecated read path *from* the provider, and
  the provider reports placement through `status.failureDomain`. Read the owner
  Machine, honor it, report status.
- **The HarvesterCluster reconcile reads the cloud-config ConfigMap before
  creating the load balancer.** A missing
  `updateCloudProviderConfig.manifestsConfigMapName` ConfigMap blocks the LB
  (and therefore the whole cluster) with only a reconciler error to show for
  it. The shipped templates create the ConfigMaps; hand-rolled setups must too.
- **Harvester's IPPool CRD** (`ippools.loadbalancer.harvesterhci.io`) is
  cluster-scoped and has **no status subresource**: repairs to
  `status.allocated` (for example after an interrupted e2e run leaks a load
  balancer allocation) are done by patching the main object. Always use the
  full resource name: on Harvester the short name `ippool` resolves to
  Calico's IPPool.
- **The Harvester LB webhook validates `selector.network`** against existing
  NetworkAttachmentDefinitions: a pool cannot reference a network that does
  not exist.
- **RKE2's "Failed to process image event ... not found" errors are noise.**
  They appear during every node bring-up and do not prevent the node from
  joining; the real failure mode is registry/network degradation making the
  image pulls exceed the MachineHealthCheck's `nodeStartupTimeout`. Do not
  chase these errors unless the node actually fails to join.
- **The RKE2 CIS profile blocks the Rancher import by default.** `profile: cis`
  enforces restricted PodSecurity cluster-wide, which rejects the
  `cattle-cluster-agent` pod. The `cisProfile` ClusterClass switch therefore
  also installs a pod security admission configuration file with the
  documented Rancher/CNI/storage namespace exemptions. Removing that file
  breaks the import silently (agent pending forever).
- **CABPR (the RKE2 bootstrap provider) generates its own CIS prerequisite
  script** post-install; the `preRKE2Commands` injected by the ClusterClass are
  deliberate defense in depth (both are idempotent). CABPR v1beta2 also
  requires `machineTemplate.spec.infrastructureRef` and an explicit
  `rolloutStrategy` on RKE2ControlPlane, and consumes audit policies from a
  Secret keyed `audit-policy.yaml`.
- **`VerifyClusterAvailable` in the Turtles spec has a fixed 5 minute window.**
  Hardened bring-ups (CIS image pulls, later MachineDeployment start) pass with
  only a couple of minutes of margin on modest hardware; if it flakes, the spec
  exposes `SkipClusterAvailableWait`.

## Debugging techniques that pay off

- **Guest autopsy without SSH**: any KubeVirt guest with the qemu-guest-agent
  can be inspected through the virt-launcher pod:
  `kubectl exec <virt-launcher-pod> -c compute -- virsh -c qemu:///session \
  qemu-agent-command <domain> '{"execute":"guest-exec", ...}'` (then
  `guest-exec-status`, output is base64). Invaluable when cloud-init or the
  SSH keys are the thing being debugged.
- **Verify what actually runs, not what you built.** When testing release
  candidates, compare the *deployed pod's* `imageID` digest with the digest you
  pushed, and check for a symbol of the new code in the binary
  (`strings`/`go tool nm`). Silent image overwrites and cache effects produce
  green runs that validate nothing.
- **Interrupted e2e runs leak.** Killing a run mid-flight leaves the kind
  cluster alive (its controllers re-create VMs you delete and hold ports
  80/443) and can leak Harvester load balancers plus their pool allocations.
  Clean the containers first, then the leftover LBs, then resync the pool.

## Certification and CI

The suites live in `test/certification/` (see its README): a nightly
version-pairing tier and an on-demand Rancher-stack tier (both Harvester-free,
standard runners), and the Turtles `CreateUsingGitOpsSpec` integration tier
against a **real Harvester** on a **self-hosted runner**, scheduled twice a
week. Two operational notes:

- The integration runner and its Harvester currently live in the outgoing
  maintainer's environment. A team-owned replacement (any Linux VM with
  docker/kind/helm/go, LAN reachability to a Harvester, and the
  `HARVESTER_KUBECONFIG_B64` secret) is required for continuity; the workflow
  itself (`certification-import-gitops.yml`) is environment-agnostic.
- After every release, bump `test/certification/config/config.yaml`
  (`CAPHV_VERSION` + `CAPHV_COMPONENTS_URL`) so the scheduled runs certify the
  new version; this is part of the release checklist in
  `docs/release-process.md`.

## Known limitations and deferred work

- **Failure domain spread has never been exercised on a multi-host Harvester**
  (development happened against a single-node cluster): discovery, publication,
  assignment and affinity are validated end to end, but an actual multi-host
  spread deserves a validation run on real hardware
  ([#237](https://github.com/harvester/cluster-api-provider-harvester/issues/237)
  asked for community feedback).
- The shipped ClusterClass's load balancer defaults to `ipamType: dhcp`;
  environments without DHCP on the VM network must switch it to `pool`.
- The machine-level `vmNetworkConfig` accepts pool references only (no inline
  `ipPool`), by design.
- FIPS is enforced (bootstrap guard), not enabled: FIPS mode is a property of
  the node image; the build recipe is in `docs/compliance.md`.
- `v1alpha1` is deprecated since v0.5.0 and must stay served through at least
  two more minors from there; removing it requires a contract-label change and
  a migration note.
- Ideas parked upstream: decoupling IP allocation into a standard CAPI IPAM
  provider; automatic per machine type documentation examples for multi-VLAN
  topologies (a real deployment is described in
  [#234](https://github.com/harvester/cluster-api-provider-harvester/issues/234)).
