# CAPHV Certification Suites

This directory contains the certification suites for CAPHV (Cluster API Provider
Harvester). They validate that a CAPHV release installs cleanly and stays compatible
with its targeted ecosystem, in three tiers. The version-pairing and Rancher-stack
tiers need no Harvester endpoint and run on standard CI runners; the Turtles
integration tier provisions real clusters and requires a Harvester plus a self-hosted
runner:

| Tier | Suite | Stack | Trigger |
|------|-------|-------|---------|
| **Turtles integration** (the suite the Turtles team expects) | `suites/import-gitops/` | Full Rancher + Turtles + real Harvester: `CreateUsingGitOpsSpec` provisions a cluster, verifies Available, the Rancher auto-import (cattle-cluster-agent) and a clean deletion | self-hosted runner (needs a Harvester) |
| **Version-pairing** (nightly) | `suites/version-pairing/` | CAPI core + RKE2 via [cluster-api-operator] - no Rancher | `certification.yml` (nightly + dispatch) |
| **Rancher + Turtles stack** (on-demand) | `suites/rancher-turtles/` | Full released Rancher; Turtles/CAPI/RKE2 via its system chart controller - no workload cluster | `certification-tier-a.yml` (dispatch) |

> Terminology: Turtles has no provider *certification* process - "certified" means
> "actively tested". The integration tier is the flow the Turtles team maintains in
> [rancher/turtles-integration-suite-example]; the other two tiers are lighter
> complements. Version pairings are recorded in [docs/compatibility.md](../../docs/compatibility.md).

[rancher/turtles-integration-suite-example]: https://github.com/rancher/turtles-integration-suite-example

> Why is the nightly tier Rancher-free? Turtles 0.26+ cannot run standalone: its
> controller watches Rancher's `management.cattle.io` CRDs and crashloops without a full
> Rancher install. The nightly tier therefore certifies the deeper CAPHV↔CAPI contract
> through the Rancher-independent cluster-api-operator, and the full-stack tier covers
> the Rancher integration on demand. The deeper e2e (real Harvester provisioning +
> auto-import) remains separate; the
> `suites/data/cluster-templates/harvester-rke2-topology.yaml` template is retained
> for it.

[cluster-api-operator]: https://github.com/kubernetes-sigs/cluster-api-operator

## Tier: version-pairing (nightly)

The suite self-deploys a kind management cluster with cert-manager and
cluster-api-operator, then installs the CAPI core, the RKE2 bootstrap/control-plane
providers and the CAPHV release under test, and asserts the pairing is healthy.

### What the suite validates

1. The four providers (CAPI core, RKE2 bootstrap, RKE2 control-plane, CAPHV
   infrastructure) all reach `Ready=True`, with the targeted versions installed.
2. The `caphv-controller-manager` deployment becomes Available under the targeted CAPI
   core.
3. The CAPHV CRDs are registered and advertise a `cluster.x-k8s.io/<contract>` label the
   core accepts.

The bootstrap replicates `hack/tier-c-smoke.sh` - the manually validated recipe - step by
step. If you change one, keep the other in sync.

## Tier: Rancher + Turtles (on-demand)

`suites/rancher-turtles/` certifies CAPHV under the FULL targeted Rancher: kind +
cert-manager + nginx ingress (isolated mode), then the **released Rancher** from its
official chart repository. Rancher's system chart controller automatically installs the
released Turtles, the CAPI core and the RKE2 providers - the exact out-of-the-box stack
a Rancher user gets (verified against a real Rancher 2.14.1 install). Only the CAPHV
`CAPIProvider` (`turtles-capi.cattle.io/v1alpha1`) is applied on top.

It validates: the CAPHV CAPIProvider reaches `Ready` with the targeted version, the
controller runs healthy under the Rancher-managed core, the CRDs carry a CAPI contract
label, and the certified ecosystem versions (Turtles system chart, CAPI core) are
recorded in the logs.

> The upstream Turtles e2e flow additionally builds a local rancher/charts tree and
> serves it from an in-cluster Gitea - that is only needed to test **unreleased**
> Turtles charts. Certifying released pairings uses Rancher's default chart repository,
> which is the representative source, so no Gitea is involved.

Run with `make test-tier-a` (or `make certification-test-tier-a` from the project root),
overriding `RANCHER_VERSION` / `CAPHV_VERSION` as needed. Budget ~30-45 minutes.

## Tier: Turtles integration suite (primary, scheduled)

`suites/import-gitops/` runs the Turtles `CreateUsingGitOpsSpec` - the suite the
Turtles team runs for every certified provider (modeled on
`rancher/turtles-integration-suite-example`) - against a **real Harvester**:

1. kind + cert-manager + nginx (hostPort) + released Rancher + Turtles + CAPI core,
   plus the RKE2 providers and the CAPHV `CAPIProvider`;
2. a ClusterClass-based RKE2 cluster (1 CP + 1 worker) is provisioned on Harvester
   through GitOps (Fleet), with a pool-backed control-plane LoadBalancer;
3. the cluster becomes `Available`, Rancher auto-imports it (cattle-cluster-agent
   deployed downstream and connected), the v3 cluster record turns ready;
4. the CAPI cluster is deleted and the suite verifies the deletion completes and the
   Rancher record disappears (no stalling finalizers, VMs/LB/IPs released).

**Environment requirements** (see also the workflow
`.github/workflows/certification-import-gitops.yml`): a Linux host with **docker**
(kind's mainstream provider), `kind`, `helm`, `kubectl`, `go` and the `crust-gather`
kubectl plugin (`scripts/ensure-crust-gather.sh`), plus two network properties:

- `RANCHER_HOSTNAME` (e.g. `<host-LAN-IP>.sslip.io`) must be reachable on 443 both
  from inside the kind cluster and **from the workload VMs** Harvester provisions;
- the host must reach the Harvester API and the workload cluster's control-plane
  endpoint (an IP from the Harvester LB pool).

A plain VM on the LAN satisfies both. Rootless/NAT'd container engines on multi-bridge
hosts often do not - nested kind networking is the first thing to check when the
downstream agent cannot reach the server-url.

In CI the suite runs on a **self-hosted runner** living inside the test environment
(same pattern as the Turtles vSphere runner), on a schedule and manual dispatch only -
never on pull requests. `HARVESTER_KUBECONFIG_B64` is provided as an Actions secret.

Run manually with `MANAGEMENT_CLUSTER_ENVIRONMENT=internal-kind`,
`RANCHER_HOSTNAME=<host-ip>.sslip.io` and `HARVESTER_KUBECONFIG_B64=$(base64 -w0 <
kubeconfig)`; budget ~40 minutes (see the workflow for the full invocation).

## Version matrix

All pins live in `config/config.yaml` (`variables:`) and are overridable from the
environment:

| Variable | Default | Meaning |
|----------|---------|---------|
| `CAPHV_VERSION` | current release (see `config.yaml`) | CAPHV release under certification |
| `CAPHV_COMPONENTS_URL` | release asset URL | `infrastructure-components.yaml` of that release |
| `CAPI_VERSION` | `v1.12.7` | Targeted Cluster API core (version-pairing tier) |
| `CAPI_OPERATOR_VERSION` | `0.27.0` | cluster-api-operator helm chart (version-pairing tier) |
| `RANCHER_VERSION` | `2.14.1` | Targeted Rancher chart (Rancher+Turtles tier) |
| `CIS_PROFILE` | empty | When set (e.g. `cis`), the import-gitops cluster is provisioned with the RKE2 CIS profile through the ClusterClass switch |
| `FIPS_REQUIRED` | `false` | When `true`, node bootstrap requires a FIPS-enabled node image |
| `VM_NETWORK`, `VOLUME_IMAGE`, `SSH_USER`, `SSH_KEYPAIR`, `TARGET_NAMESPACE`, `GATEWAY`, `SUBNET_MASK`, `IP_POOL_REF`, `DNS_SERVERS` | see `config.yaml` | Harvester-side inputs of the import-gitops cluster template |

The RKE2 providers deliberately carry no pin: the operator installs its latest known
release, matching the validated recipe. cert-manager's version is pinned by the turtles
`DeployCertManager` helper.

## Prerequisites

- A container runtime for kind: docker, or **rootful** podman (the kind library
  auto-detects docker -> nerdctl -> podman; the CLI-only `KIND_EXPERIMENTAL_PROVIDER`
  variable has no effect here). No kind binary is needed - kind is vendored as a Go
  library.
- `helm` (v3+) and `kubectl` on `PATH`.
- Go (version per `go.mod`).
- Internet access (chart repos, GitHub release assets, ghcr.io).

## Running

```bash
# Wrapper script (resolves helm path, creates artifacts folder)
./run.sh

# Keep the kind cluster for debugging
./run.sh --skip-cleanup

# Or via make (same defaults)
make test

# Certify a different pairing
CAPHV_VERSION=v0.3.2 \
CAPHV_COMPONENTS_URL=https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.3.2/infrastructure-components.yaml \
./run.sh
```

From the project root: `make certification-test` / `make certification-build`.

On a podman host, build first and run the compiled binary as root (kind needs rootful
podman, and this keeps Go caches owned by your user):

```bash
make build
sudo -E env "PATH=$PATH" HOME=/root \
  E2E_CONFIG=$PWD/config/config.yaml ARTIFACTS_FOLDER=$PWD/_artifacts \
  HELM_BINARY_PATH=$(command -v helm) \
  ./bin/certification.test -test.timeout 60m -ginkgo.v -ginkgo.label-filter=short
```

## Structure

```
test/certification/
├── config/config.yaml            # E2E config: version matrix, intervals, kind settings
├── hack/tier-c-smoke.sh          # Manually validated recipe the suite replicates
├── suites/
│   ├── const.go                  # Embedded manifests (build tag: e2e)
│   ├── data/
│   │   ├── providers/            # provider CRs (envsubst templates)
│   │   │   ├── core.yaml         #   CoreProvider (CAPI ${CAPI_VERSION})
│   │   │   ├── rke2.yaml         #   RKE2 Bootstrap/ControlPlane providers
│   │   │   ├── harvester.yaml    #   CAPHV InfrastructureProvider (${CAPHV_VERSION})
│   │   │   └── harvester-capiprovider.yaml  # CAPHV as Turtles CAPIProvider (tier A)
│   │   └── cluster-templates/    # ClusterClass template (on-demand e2e tier)
│   ├── version-pairing/
│   │   ├── suite_test.go         # Bootstrap: kind -> cert-manager -> operator -> providers
│   │   └── version_pairing_test.go  # Certification assertions
│   ├── rancher-turtles/
│   │   ├── suite_test.go         # Bootstrap: kind -> ingress -> Rancher (system charts)
│   │   └── rancher_turtles_test.go  # Certification assertions (tier A)
│   └── import-gitops/            # Turtles CreateUsingGitOpsSpec vs real Harvester
├── scripts/ensure-crust-gather.sh # Installs the crust-gather kubectl plugin
├── Makefile
├── run.sh
├── go.mod                        # Standalone module (decoupled from the main CAPHV module)
└── README.md
```

## Certification submission

After the suite passes for a new pairing, a certification issue can be submitted on
[rancher/turtles](https://github.com/rancher/turtles/issues) with the test logs, the
CAPHV version and a link to this suite.

## Running the Turtles integration tier manually

See the tier description at the top of this document for the flow and the
environment requirements. Quick start on a suitable host:

```bash
export RANCHER_HOSTNAME=<host-lan-ip>.sslip.io
export HARVESTER_KUBECONFIG_B64=$(base64 -w0 < harvester.kubeconfig)
make test-import-gitops
```
