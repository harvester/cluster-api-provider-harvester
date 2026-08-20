# CAPHV Production Operations Guide

Cluster API Provider Harvester (CAPHV) is a CAPI infrastructure provider that manages
Kubernetes clusters on Harvester HCI. This guide covers installation, cluster lifecycle,
monitoring, and disaster recovery for production environments.

---

## Installation via CAPIProvider (Rancher Turtles)

The recommended production deployment uses the Rancher Turtles `CAPIProvider` CRD. Turtles
watches for CAPIProvider resources and automatically downloads, installs, and manages the
provider lifecycle -- including the controller Deployment, CRDs, webhooks, RBAC, and
ServiceMonitor.

### Prerequisites

- A Rancher management cluster with Rancher Turtles installed
- cert-manager deployed on the management cluster (required for webhook certificates)
- Cluster API core provider already installed (Turtles handles this if configured)

### Deploy the CAPIProvider

Create a namespace and apply the CAPIProvider resource:

```bash
kubectl create namespace caphv-system
```

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

```bash
kubectl apply -f capiprovider-harvester.yaml
```

### What Turtles deploys

When the CAPIProvider is created, Turtles fetches `infrastructure-components.yaml` from the
specified URL and applies its contents to the cluster. This single manifest contains:

- **CRDs**: HarvesterCluster, HarvesterMachine, HarvesterClusterTemplate, HarvesterMachineTemplate
- **Controller Deployment**: `caphv-controller-manager` in namespace `caphv-system`
- **RBAC**: ClusterRole, ClusterRoleBinding, Role, RoleBinding for the controller
- **Webhooks**: ValidatingWebhookConfiguration for HarvesterCluster and HarvesterMachine resources
- **cert-manager resources**: Issuer and Certificate for webhook TLS
- **ServiceMonitor**: Prometheus scrape configuration for controller metrics
- **Service**: `caphv-webhook-service` (port 443 -> 9443) and `caphv-controller-manager-metrics-service` (port 8443 -> 8080)

### Verify the deployment

```bash
# Check CAPIProvider status
kubectl get capiprovider -n caphv-system

# Check the controller pod is running
kubectl get pods -n caphv-system

# Check CRDs are installed
kubectl get crd | grep harvester

# Check webhooks are registered
kubectl get validatingwebhookconfigurations | grep caphv
```

The CAPIProvider status should show `Installed` and the controller pod should be `Running`
with 2/2 containers ready (controller + kube-rbac-proxy).

### Automatic Provider Upgrades

Turtles supports automatic version upgrades via `enableAutomaticUpdate`. When enabled,
Turtles monitors for new releases and automatically updates the provider when a new version
is published.

**CAPIProvider with automatic updates**:

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
  enableAutomaticUpdate: true
  fetchConfig:
    url: https://github.com/harvester/cluster-api-provider-harvester/releases/latest/download/infrastructure-components.yaml
  configSecret:
    name: caphv-variables
```

Key differences from manual deployment:
- `enableAutomaticUpdate: true` - Turtles polls for new versions
- `fetchConfig.url` uses `/releases/latest/download/` - resolves to the latest release

**Manual upgrade** (if auto-update is disabled):

```bash
# 1. Update the CAPIProvider version and URL
kubectl patch capiprovider harvester -n caphv-system --type merge -p '{
  "spec": {
    "version": "v0.10.1",
    "fetchConfig": {
      "url": "https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.10.1/infrastructure-components.yaml"
    }
  }
}'

# 2. Watch the rollout
kubectl rollout status deploy/caphv-controller-manager -n caphv-system

# 3. Verify new version
kubectl get deploy caphv-controller-manager -n caphv-system -o jsonpath='{.spec.template.spec.containers[0].image}'
```

### Functional test: provider upgrade

1. **Deploy CAPIProvider** with current version:
```bash
kubectl apply -f capiprovider-harvester.yaml
kubectl wait --for=condition=Ready capiprovider/harvester -n caphv-system --timeout=120s
```

2. **Verify running version**:
```bash
kubectl get deploy caphv-controller-manager -n caphv-system \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
# Expected: ghcr.io/harvester/cluster-api-provider-harvester:v0.10.1
```

3. **Patch to new version** (manual upgrade test):
```bash
kubectl patch capiprovider harvester -n caphv-system --type merge -p '{
  "spec": {"version": "v0.10.1"}
}'
```

4. **Watch upgrade rollout**:
```bash
kubectl rollout status deploy/caphv-controller-manager -n caphv-system --timeout=120s
```

5. **Verify workload clusters unaffected**:
```bash
kubectl get clusters.cluster.x-k8s.io -A
kubectl get machines.cluster.x-k8s.io -A
# All clusters should remain Ready, machines Running
```

6. **Enable automatic updates** (optional):
```bash
kubectl patch capiprovider harvester -n caphv-system --type merge -p '{
  "spec": {"enableAutomaticUpdate": true}
}'
```

---

## Installation without Rancher (clusterctl / direct manifests)

Rancher Turtles is the recommended delivery vehicle, but it is **not** required.
CAPHV talks to Harvester directly through the identity Secret - Rancher is never
on that path. You can run CAPHV on any management cluster (a plain RKE2/k3s/kind
cluster, or even Harvester's own embedded cluster) by installing the CAPI stack
yourself.

The full stack for v0.10.1 is: **CAPI core v1.12.x + cluster-api-provider-rke2
v0.25.0 (bootstrap + control plane) + CAPHV v0.10.1**, plus cert-manager for the
webhooks.

Both methods below were validated on a clean cluster (originally with v0.3.0; the pins below track the current release): all four controllers reach
`1/1 Running` and the CAPHV webhook becomes Ready.

### Method A - clusterctl (recommended when Rancher is absent)

`clusterctl` resolves provider versions, installs cert-manager, and substitutes
the `${...}` parameters in each provider's components - so the RKE2 controllers
come up cleanly.

CAPHV is not a built-in clusterctl provider, so register it in the clusterctl
config:

```yaml
# ~/.cluster-api/clusterctl.yaml
providers:
  - name: harvester
    url: https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.10.1/infrastructure-components.yaml
    type: InfrastructureProvider
```

```bash
clusterctl init \
  --core cluster-api:v1.12.8 \
  --bootstrap rke2:v0.25.0 \
  --control-plane rke2:v0.25.0 \
  --infrastructure harvester:v0.10.1
```

> **clusterctl version**: use `clusterctl` **v1.12.x**. Older clusterctl
> (e.g. v1.10.x) defaults CAPI core to v1.10, which is incompatible with the
> v1beta2 contract CAPHV v0.3.0+ requires.

> **metadata.yaml requirement**: clusterctl maps each version to a CAPI contract
> via the release's `metadata.yaml`, which must contain the matching series
> (for v0.10.1, a `0.10` series with `contract: v1beta2`). Every release since
> v0.3.1 declares its series. If `clusterctl init` reports
> *"version vX.Y.Z ... does not match any release series"*, the release asset
> predates the fix - drop a corrected
> `metadata.yaml` into a clusterctl overrides folder:
> `~/.cluster-api/overrides/infrastructure-harvester/vX.Y.Z/metadata.yaml`
> with the `0.3 -> v1beta2` series, alongside a copy of
> `infrastructure-components.yaml`, and re-run.

Verify:

```bash
kubectl get pods -n capi-system -n rke2-bootstrap-system \
  -n rke2-control-plane-system -n caphv-system
# all controllers 1/1 Running; caphv-controller-manager Ready once cert-manager
# issues the webhook cert
```

### Method B - raw manifests (airgap / GitOps, no clusterctl)

The published `*-components.yaml` are designed for clusterctl/operator and carry
unrendered `${VAR:=default}` placeholders (this is the CAPI convention, true of
CAPI core too). Applying them raw leaves the controllers crash-looping on
`invalid argument "${CAPRKE2_...}"`. Render the defaults first:

```bash
# render ${VAR:=default} -> default, drop bare ${VAR}
render() { sed -E 's/\$\{[A-Za-z0-9_]+:=([^}]*)\}/\1/g; s/\$\{[A-Za-z0-9_]+\}//g'; }

# 1. cert-manager (webhook prerequisite)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=180s

# 2. CAPI core
curl -sL https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.12.8/core-components.yaml | render | kubectl apply -f -

# 3. RKE2 bootstrap + control plane
for c in bootstrap-components control-plane-components; do
  curl -sL https://github.com/rancher/cluster-api-provider-rke2/releases/download/v0.25.0/$c.yaml | render | kubectl apply -f -
done

# 4. CAPHV (no placeholders to render)
kubectl apply -f https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.10.1/infrastructure-components.yaml
```

> Without the `render` step, the RKE2 controllers crash-loop on the literal
> `${CAPRKE2_INSECURE_DIAGNOSTICS:=false}` argument. The `sed` substitution
> applies the same defaults clusterctl would.

After either method, create clusters exactly as documented elsewhere in this
guide (ClusterClass + `caphv-generate`, or the CAPIProvider examples - the
CRDs and controller are identical, only the install path differs).

---

## Migration from Manual Deploy to CAPIProvider

If CAPHV was previously deployed manually (via `kubectl apply -f` or Helm), follow this
procedure to migrate to the Turtles-managed CAPIProvider approach without disrupting existing
workload clusters.

### Step 1: Document existing state

```bash
# Record all CAPI resources for each managed cluster
for ns in $(kubectl get clusters.cluster.x-k8s.io -A -o jsonpath='{.items[*].metadata.namespace}' | tr ' ' '\n' | sort -u); do
  echo "=== Namespace: $ns ==="
  kubectl get cluster,machine,harvestercluster,harvestermachine,machinedeployment,machineset -n "$ns"
done

# Save current controller deployment for reference
kubectl get deploy caphv-controller-manager -n caphv-system -o yaml > caphv-deploy-backup.yaml
```

### Step 2: Back up CAPI resources

```bash
# Export all CAPHV-related resources
kubectl get harvesterclusters.infrastructure.cluster.x-k8s.io -A -o yaml > backup-harvesterclusters.yaml
kubectl get harvestermachines.infrastructure.cluster.x-k8s.io -A -o yaml > backup-harvestermachines.yaml
kubectl get clusters.cluster.x-k8s.io -A -o yaml > backup-clusters.yaml
kubectl get machines.cluster.x-k8s.io -A -o yaml > backup-machines.yaml
kubectl --kubeconfig <harvester-kubeconfig> get ippools.loadbalancer.harvesterhci.io -o yaml > backup-ippools.yaml
```

### Step 3: Remove the manual deployment

If deployed via raw manifests:

```bash
# Delete only the controller deployment, service, and associated RBAC
# Do NOT delete CRDs -- they hold your cluster state
kubectl delete deploy caphv-controller-manager -n caphv-system
kubectl delete service caphv-controller-manager-metrics-service -n caphv-system
kubectl delete service caphv-webhook-service -n caphv-system
kubectl delete validatingwebhookconfiguration caphv-validating-webhook-configuration
kubectl delete clusterrole caphv-manager-role caphv-metrics-reader caphv-proxy-role
kubectl delete clusterrolebinding caphv-manager-rolebinding caphv-proxy-rolebinding
```

If deployed via Helm:

```bash
helm uninstall caphv -n caphv-system
```

**Important**: Helm uninstall removes CRDs only if they were installed by Helm and
`keep` annotations are absent. Verify CRDs still exist after uninstall:

```bash
kubectl get crd | grep harvester
```

If CRDs were removed, re-apply them before proceeding:

```bash
curl -sL https://github.com/harvester/cluster-api-provider-harvester/releases/download/v0.10.1/infrastructure-components.yaml | kubectl apply -f - --server-side --field-manager=crd-restore --prune=false
```

### Step 4: Create the CAPIProvider resource

```bash
kubectl apply -f capiprovider-harvester.yaml
```

### Step 5: Verify Turtles picks up the provider

```bash
# Watch until status shows Installed
kubectl get capiprovider -n caphv-system -w

# Verify the controller pod is running
kubectl get pods -n caphv-system

# Verify controller logs show no errors
kubectl logs -n caphv-system deploy/caphv-controller-manager -c manager --tail=50
```

### Step 6: Verify existing clusters are intact

```bash
# All clusters should show Phase=Provisioned, Ready=True
kubectl get clusters.cluster.x-k8s.io -A

# All machines should show Phase=Running
kubectl get machines.cluster.x-k8s.io -A

# Reconciliation resumes automatically -- check controller logs
kubectl logs -n caphv-system deploy/caphv-controller-manager -c manager | grep -i reconcil | tail -20
```

Existing workload clusters are unaffected by this migration. The CAPI resources (Cluster,
Machine, etc.) remain in etcd, and the new controller instance picks up reconciliation
immediately.

---

## Cluster Lifecycle

### Creating a cluster

#### Via ClusterClass (recommended)

The `caphv-generate` CLI generates all required manifests from a minimal set of parameters.
This is the recommended approach for production clusters.

```bash
caphv-generate \
  --name production-cluster \
  --namespace prod-ns \
  --image default/sles15-sp7-minimal-vm.x86_64-cloud-qu2.qcow2 \
  --ssh-keypair default/capi-ssh-key \
  --network default/production \
  --gateway 172.16.0.1 \
  --subnet-mask 255.255.0.0 \
  --ip-pool prod-ip-pool \
  --dns 172.16.0.1 \
  --harvester-kubeconfig ~/.kube/harvester.yaml \
  --cp-replicas 3 \
  --worker-replicas 2 \
  --cpu 4 \
  --memory 8Gi \
  --disk-size 80Gi \
  --k8s-version v1.31.14 \
  --apply
```

Or use interactive mode:

```bash
caphv-generate --interactive
```

The generator produces 10 objects: Namespace, Secret (Harvester kubeconfig), Cluster (with
topology referencing ClusterClass `harvester-rke2`), 3 ConfigMaps (CCM, CSI, Calico addons),
3 ClusterResourceSets, and a MachineHealthCheck. This reduces cluster creation from ~200
lines of YAML to a single command.

**Important**: The ClusterClass `harvester-rke2` must exist in the same namespace as the
Cluster (ClusterClass is namespace-scoped in CAPI). It is deployed automatically with the
controller when `clusterClass.enabled=true` in Helm, or included in
`infrastructure-components.yaml`.

#### Via manual templates

For full control over every resource, create each object individually:

1. HarvesterCluster -- Defines identity secret, target namespace, IP pool, network config
2. HarvesterMachineTemplate -- CPU, memory, volumes, networks, SSH config
3. RKE2ControlPlane -- Control plane configuration, Kubernetes version, replicas
4. MachineDeployment -- Worker node configuration, replicas
5. ConfigMaps + ClusterResourceSets -- Cloud provider (CCM), CSI driver, CNI addons
6. Cluster -- References the above, ties everything together

See the `examples/` directory for complete manifests.

### Scaling

#### Scale workers via ClusterClass topology

```bash
kubectl patch cluster my-cluster -n my-ns --type merge -p '{
  "spec": {
    "topology": {
      "workers": {
        "machineDeployments": [{
          "class": "default-worker",
          "name": "md-0",
          "replicas": 3
        }]
      }
    }
  }
}'
```

#### Scale workers via MachineDeployment (manual templates)

```bash
kubectl scale machinedeployment my-cluster-md-0 -n my-ns --replicas=3
```

#### Scale control plane

```bash
# ClusterClass topology
kubectl patch cluster my-cluster -n my-ns --type merge -p '{
  "spec": {
    "topology": {
      "controlPlane": {
        "replicas": 5
      }
    }
  }
}'

# Or directly on RKE2ControlPlane
kubectl patch rke2controlplane my-cluster-control-plane -n my-ns --type merge -p '{"spec":{"replicas":5}}'
```

Control plane scaling adds or removes nodes one at a time, maintaining etcd quorum
throughout the operation.

### Kubernetes version upgrade

Change `spec.topology.version` on the Cluster object:

```bash
kubectl patch cluster my-cluster -n my-ns --type merge -p '{
  "spec": {
    "topology": {
      "version": "v1.32.2"
    }
  }
}'
```

The rolling upgrade proceeds as follows:

1. Control plane nodes are upgraded one at a time (respecting etcd quorum)
2. Each CP node: cordon -> drain -> delete VM -> create new VM with new version -> wait for Ready
3. After all CP nodes are upgraded, workers are upgraded (one at a time per MachineDeployment)
4. Worker upgrade follows the same cordon -> drain -> replace cycle

Monitor the upgrade:

```bash
# Watch machine status in real time
kubectl get machines -n my-ns -w

# Check RKE2ControlPlane rollout status
kubectl get rke2controlplane -n my-ns

# Verify Kubernetes version on nodes (from workload cluster)
kubectl get nodes -o wide
```

A typical 3 CP + 1 worker upgrade takes approximately 35 minutes.

### Deleting a cluster

```bash
kubectl delete cluster my-cluster -n my-ns
```

CAPI cascades the deletion through the ownership chain:

1. Cluster deletion triggers Machine deletion
2. CAPHV deletes the Harvester VM for each Machine
3. CAPHV deletes associated PVCs (all volumes)
4. CAPHV deletes cloud-init secrets on Harvester
5. CAPHV releases allocated IPs back to the IPPool
6. CAPI garbage-collects remaining objects (MachineSet, MachineDeployment, etc.)

To verify cleanup is complete:

```bash
# All machines should be gone
kubectl get machines -n my-ns

# Check Harvester for orphaned VMs (should be empty)
kubectl get vm -n <harvester-target-namespace> --kubeconfig <harvester-kubeconfig>
```

### Pausing and resuming a cluster (free up Harvester resources)

A CAPHV cluster can be parked without deleting it: the VM disks stay on
Harvester and the CAPI objects stay on the management cluster, so a resume
takes minutes instead of a full re-provision.

**Pause:**

```bash
# 1. Scale workers to zero FIRST, while the control plane is still up.
#    The node drain needs a reachable workload API server.
kubectl -n my-ns patch cluster my-cluster --type=merge \
  -p '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"workers","replicas":0}]}}}}'
# wait for the worker Machines/VMs to be deleted

# 2. Pause CAPI reconciliation so CAPHV stops restarting the VMs.
kubectl -n my-ns patch cluster my-cluster --type=merge -p '{"spec":{"paused":true}}'
#    CAPHV acknowledges by setting condition Paused=True on the HarvesterCluster
#    and HarvesterMachines (v1beta2 contract); their status is frozen until resume:
kubectl -n my-ns get harvestercluster my-cluster \
  -o jsonpath='{.status.conditions[?(@.type=="Paused")].status}'

# 3. Halt the control-plane VM(s) on Harvester (disks are preserved).
kubectl --kubeconfig <harvester-kubeconfig> -n <target-ns> patch vm <cp-vm-name> \
  --type=merge -p '{"spec":{"runStrategy":"Halted"}}'
```

> **Do not scale the control plane to zero** - the RKE2ControlPlane webhook
> rejects `replicas <= 0`. Halting the VM with the cluster paused achieves
> the same effect.

> **Ordering pitfall:** if you halt the control plane *before* the worker
> scale-down finishes, the Machine deletion blocks on node drain (the
> workload API server is unreachable). Recover with:
> `kubectl -n my-ns annotate machine <worker-machine> \
>    machine.cluster.x-k8s.io/exclude-node-draining=true \
>    machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach=true`
> A Machine stuck in `Deleting` while the cluster is paused will only
> finalize after unpausing - or remove its finalizers manually once you
> have confirmed the Harvester VM and PVC are gone.

**Resume:**

```bash
# 1. Restart the control-plane VM and wait for the workload API server.
kubectl --kubeconfig <harvester-kubeconfig> -n <target-ns> patch vm <cp-vm-name> \
  --type=merge -p '{"spec":{"runStrategy":"Always"}}'

# 2. Unpause; CAPHV reconciles from the existing state.
kubectl -n my-ns patch cluster my-cluster --type=merge -p '{"spec":{"paused":false}}'

# 3. Scale the workers back up (fresh VMs are provisioned, ~10 min each).
kubectl -n my-ns patch cluster my-cluster --type=merge \
  -p '{"spec":{"topology":{"workers":{"machineDeployments":[{"class":"default-worker","name":"workers","replicas":1}]}}}}'
```

Expect the control plane to report `Ready` a few minutes after the VM boots
(RKE2 restart + etcd recovery), and `MachineHealthCheck` to hold off as long
as `nodeStartupTimeout` allows.

---

## MachineHealthCheck and Auto-Remediation

CAPHV supports automatic machine remediation via the standard CAPI MachineHealthCheck (MHC)
resource. The `caphv-generate` CLI creates an MHC by default with production-appropriate
settings.

### Default MHC configuration

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: my-cluster-mhc
  namespace: my-ns
spec:
  clusterName: my-cluster
  maxUnhealthy: 34%
  nodeStartupTimeout: 20m
  unhealthyConditions:
    - type: Ready
      status: "False"
      timeout: 5m
    - type: Ready
      status: Unknown
      timeout: 5m
```

- **maxUnhealthy: 34%** -- Prevents cascading remediation. With 3 CP nodes, this allows at
  most 1 simultaneous remediation (34% of 3 = 1.02, rounded down to 1). This protects
  against etcd quorum loss.
- **nodeStartupTimeout: 20m** -- Time allowed for a new Machine to become a Ready node.
  Accounts for VM creation, OS boot, RKE2 installation, and node initialization.
- **unhealthyConditions** -- A node is considered unhealthy after 5 minutes of `Ready=False`
  or `Ready=Unknown`.

### Remediation flow

When a node becomes unhealthy:

1. MHC detects the condition after the configured timeout (5 minutes)
2. MHC marks the Machine for deletion
3. CAPHV controller handles the Machine deletion:
   - Removes the etcd member from the cluster (for CP nodes, via `etcdctl member remove`)
   - Deletes the VM on Harvester
   - Deletes associated PVCs and cloud-init secrets
   - Releases the IP back to the pool
4. CAPI creates a replacement Machine
5. CAPHV provisions a new VM with a new IP from the pool
6. RKE2 installs, the node joins the cluster, and becomes Ready

The full cycle (detection through recovery) takes approximately 9 minutes.

### Monitoring MHC

```bash
# Check MHC status
kubectl get machinehealthcheck -n my-ns

# Check for machines marked for remediation
kubectl get machines -n my-ns -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,NODE:.status.nodeRef.name,HEALTHY:.status.conditions[0].status

# Watch the remediation in real time
kubectl get machines -n my-ns -w
```

### Tuning MHC for production

For large clusters, consider adjusting:

```yaml
spec:
  # Allow more simultaneous remediations for large worker pools
  maxUnhealthy: 20%
  # Increase startup timeout for slower infrastructure
  nodeStartupTimeout: 30m
  # Longer unhealthy timeout to avoid false positives during rolling upgrades
  unhealthyConditions:
    - type: Ready
      status: "False"
      timeout: 10m
```

---

## Monitoring

### Prometheus metrics

The CAPHV controller exposes Prometheus metrics on port 8080, served through the
kube-rbac-proxy sidecar on port 8443. A ServiceMonitor resource is included in the
deployment for automatic Prometheus discovery.

### Metric reference

All metrics use the `caphv_` namespace prefix.

#### Machine lifecycle

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `caphv_machine_create_total` | Counter | -- | Total VM creation attempts |
| `caphv_machine_create_errors_total` | Counter | -- | Failed VM creation attempts |
| `caphv_machine_creation_duration_seconds` | Histogram | -- | VM creation duration (buckets: 1s to ~512s) |
| `caphv_machine_delete_total` | Counter | -- | Total VM deletion attempts |
| `caphv_machine_delete_errors_total` | Counter | -- | Failed VM deletion attempts |
| `caphv_machine_status` | Gauge | `cluster`, `machine` | Current machine status (1=ready, 0=not ready) |
| `caphv_machine_reconcile_duration_seconds` | Histogram | `operation` | Machine reconciliation duration (operation: "normal" or "delete") |

#### IP pool

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `caphv_ippool_allocations_total` | Counter | -- | Total IP allocation attempts |
| `caphv_ippool_allocation_errors_total` | Counter | -- | Failed IP allocation attempts |
| `caphv_ippool_releases_total` | Counter | -- | Total IP releases |

#### Cluster

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `caphv_cluster_reconcile_duration_seconds` | Histogram | `operation` | Cluster reconciliation duration (operation: "normal" or "delete") |
| `caphv_cluster_ready` | Gauge | `cluster` | Cluster ready status (1=ready, 0=not ready) |

#### etcd management

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `caphv_etcd_member_remove_total` | Counter | -- | Total etcd member removal attempts |
| `caphv_etcd_member_remove_errors_total` | Counter | -- | Failed etcd member removals |

#### Node initialization

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `caphv_node_init_total` | Counter | -- | Total node initialization attempts |
| `caphv_node_init_errors_total` | Counter | -- | Failed node initializations |
| `caphv_node_init_duration_seconds` | Histogram | -- | Node initialization duration (buckets: 0.5s to ~64s) |

### Grafana dashboard

Import the pre-built dashboard from the repository:

```bash
# File location in the repository
config/grafana/caphv-dashboard.json
```

Import via the Grafana UI (Dashboards -> Import -> Upload JSON file) or via the Grafana API:

```bash
curl -X POST http://grafana:3000/api/dashboards/db \
  -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  -d @config/grafana/caphv-dashboard.json
```

### Recommended alerts

Set up the following Prometheus alerting rules for production:

```yaml
groups:
  - name: caphv
    rules:
      - alert: CAPHVMachineCreateErrors
        expr: increase(caphv_machine_create_errors_total[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "CAPHV machine creation errors detected"
          description: "{{ $value }} VM creation errors in the last 5 minutes."

      - alert: CAPHVMachineNotReady
        expr: caphv_machine_status == 0
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "CAPHV machine {{ $labels.machine }} not ready"
          description: "Machine {{ $labels.machine }} in cluster {{ $labels.cluster }} has been not ready for more than 10 minutes."

      - alert: CAPHVIPPoolAllocationErrors
        expr: increase(caphv_ippool_allocation_errors_total[5m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "CAPHV IP pool allocation errors"
          description: "IP allocation failures detected. Check pool exhaustion or configuration."

      - alert: CAPHVClusterNotReady
        expr: caphv_cluster_ready == 0
        for: 15m
        labels:
          severity: critical
        annotations:
          summary: "CAPHV cluster {{ $labels.cluster }} not ready"
          description: "Cluster {{ $labels.cluster }} has been not ready for more than 15 minutes."

      - alert: CAPHVEtcdRemoveErrors
        expr: increase(caphv_etcd_member_remove_errors_total[10m]) > 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "CAPHV etcd member removal errors"
          description: "Failed etcd member removals detected. Check etcd cluster health."

      - alert: CAPHVSlowMachineCreation
        expr: histogram_quantile(0.95, rate(caphv_machine_creation_duration_seconds_bucket[30m])) > 300
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CAPHV machine creation is slow"
          description: "95th percentile VM creation time exceeds 5 minutes."
```

---

## Webhooks

CAPHV includes validating admission webhooks for `HarvesterCluster` and `HarvesterMachine`
resources. These reject invalid configurations at admission time, before the controller
attempts to reconcile them.

### Enabling webhooks

Webhooks are controlled by the `--enable-webhooks` flag on the controller. When deployed via
CAPIProvider/Turtles, webhooks are enabled by default in `infrastructure-components.yaml`.

Requirements:

- **cert-manager** must be installed on the management cluster
- cert-manager creates a self-signed Issuer and Certificate, producing the Secret
  `caphv-webhook-tls` mounted at `/tmp/k8s-webhook-server/serving-certs/` in the controller
  pod
- The ValidatingWebhookConfiguration uses the `cert-manager.io/inject-ca-from` annotation for
  automatic CA bundle injection

### What is validated

**HarvesterCluster**:

| Field | Validation |
|-------|-----------|
| `spec.targetNamespace` | Required, must not be empty |
| `spec.identitySecret.name` | Required |
| `spec.identitySecret.namespace` | Required |
| `spec.loadBalancerConfig.ipamType` | Must be `"dhcp"` or `"pool"` |
| `spec.vmNetworkConfig.gateway` | Required, must be a valid IP address |
| `spec.vmNetworkConfig.subnetMask` | Required, must be a valid IP address format |
| `spec.vmNetworkConfig.ipPoolRef` or `ipPoolRefs` or `ipPool` | At least one must be set when vmNetworkConfig is specified |

**HarvesterMachine**:

| Field | Validation |
|-------|-----------|
| `spec.cpu` | Must be greater than 0 |
| `spec.memory` | Required, must be a valid Kubernetes resource quantity (e.g., `4Gi`, `8192Mi`) |
| `spec.sshUser` | Required |
| `spec.sshKeyPair` | Required |
| `spec.volumes` | At least one volume required |
| `spec.volumes[].volumeType` | Must be `"image"` or `"storageClass"` |
| `spec.volumes[].imageName` | Required when volumeType is `"image"` |
| `spec.volumes[].storageClass` | Required when volumeType is `"storageClass"` |
| `spec.networks` | At least one network required |
| `spec.networkConfig.address` | Required when networkConfig is set |
| `spec.networkConfig.gateway` | Required and must be a valid IP when networkConfig is set |
| `spec.vmNetworkConfig` | Mutually exclusive with `networkConfig`; requires `ipPoolRef` or `ipPoolRefs` (inline `ipPool` is rejected at the machine level); `gateway` and `subnetMask` required and must be valid IPs |
| `spec.firmware.secureBoot` | Requires `spec.firmware.efi` to be true |

### Troubleshooting webhook issues

If resources are being rejected unexpectedly:

```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations caphv-validating-webhook-configuration -o yaml

# Verify the webhook certificate is valid
kubectl get certificate -n caphv-system
kubectl get secret caphv-webhook-tls -n caphv-system

# Check webhook service endpoint
kubectl get endpoints caphv-webhook-service -n caphv-system

# Test with a dry-run create
kubectl apply --dry-run=server -f my-harvestermachine.yaml
```

If the webhook is down and blocking operations, temporarily remove it (use with caution):

```bash
kubectl delete validatingwebhookconfiguration caphv-validating-webhook-configuration
```

The webhook configuration ships with the components: re-apply the release manifests (or let the CAPIProvider reconcile) to restore it; the controller does not re-create it by itself.

---

## Multi-Pool IP Allocation

### Overview

When a single IPPool is not large enough for a deployment (e.g., hundreds of VMs across
multiple subnets), you can configure multiple IPPools with ordered fallback.

### Configuration

**Option A - Single pool** (backward compatible):

```yaml
spec:
  vmNetworkConfig:
    ipPoolRef: "capi-vm-pool"
    gateway: "172.16.0.1"
    subnetMask: "255.255.0.0"
```

**Option B - Multiple pools with fallback**:

```yaml
spec:
  vmNetworkConfig:
    ipPoolRefs:
      - "capi-pool-subnet-a"
      - "capi-pool-subnet-b"
      - "capi-pool-subnet-c"
    gateway: "172.16.0.1"
    subnetMask: "255.255.0.0"
```

Pools are tried in order. When `capi-pool-subnet-a` is exhausted, allocation falls back to
`capi-pool-subnet-b`, then `capi-pool-subnet-c`. Each machine tracks which pool it allocated
from in `status.allocatedPoolRef` for accurate IP release on deletion.

**Network-aware selection**: a pool whose Harvester `spec.selector.network` designates a VM
network is only considered for machines attached to that network; a pool without a selector
network is considered for any machine. This makes separate address ranges per machine role
possible with a single list, for example a dedicated control-plane network and a worker
network:

```yaml
# Harvester side: one pool per network
# ippool "pool-cp":      spec.selector.network: default/net-cp
# ippool "pool-workers": spec.selector.network: default/net-workers
spec:
  vmNetworkConfig:
    ipPoolRefs:
      - "pool-cp"
      - "pool-workers"
```

With the control-plane HarvesterMachineTemplate attached to `default/net-cp` and the worker
template attached to `default/net-workers`, each machine allocates from the pool matching its
own network. If no listed pool matches a machine's networks, allocation fails with an explicit
error rather than handing out an address from another network.

### CLI usage

```bash
# Single pool (existing behavior)
caphv-generate --ip-pool my-pool ...

# Multiple pools
caphv-generate --ip-pool-refs "pool-a,pool-b,pool-c" ...
```

### Functional test procedure

1. **Create two small IPPools** on Harvester (e.g., 2 IPs each):

```bash
# pool-a: 172.16.3.40-41 (2 IPs)
# pool-b: 172.16.3.42-43 (2 IPs)
```

2. **Deploy a cluster with `ipPoolRefs`** referencing both pools:

```bash
caphv-generate --name multipool-test --ip-pool-refs "pool-a,pool-b" [other flags...] --apply
```

3. **Scale up to 4 machines** (2 CP + 2 workers) - should allocate from both pools:

```bash
# Verify allocations
kubectl get harvestermachines -n multipool-test -o custom-columns=\
  NAME:.metadata.name,IP:.status.allocatedIPAddress,POOL:.status.allocatedPoolRef

# Expected: first 2 machines from pool-a, next 2 from pool-b
```

4. **Delete one machine** - verify its IP is released from the correct pool:

```bash
# Before delete: check pool-a status.allocated
kubectl get ippools.loadbalancer.harvesterhci.io pool-a -o jsonpath='{.status.allocated}' | jq .

# Delete a machine
kubectl delete machine <machine-name> -n multipool-test

# After delete: IP removed from the correct pool (pool-a or pool-b)
kubectl get ippools.loadbalancer.harvesterhci.io pool-a -o jsonpath='{.status.allocated}' | jq .
```

5. **Unit tests** (5 new tests):
   - `allocateVMIP` fallback from pool-1 to pool-2 when pool-1 exhausted
   - `allocateVMIP` error when all pools exhausted
   - `allocateVMIP` backward compat with single `ipPoolRef`
   - `allocateVMIP` sets `AllocatedPoolRef` correctly
   - `releaseVMIP` uses `AllocatedPoolRef` for targeted release

---

### Per machine type network configuration

Network-aware selection picks the right pool, but the cluster-level `vmNetworkConfig` still
carries a single `gateway`, `subnetMask` and DNS set for every machine. When the control-plane
and worker networks are different subnets (for example to comply with the German BSI
APP.4.4.A7 requirement of separate control-plane and worker networks), set `vmNetworkConfig`
directly in the HarvesterMachineTemplate of the machine type that lives in the other subnet:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterMachineTemplate
metadata:
  name: workers
spec:
  template:
    spec:
      networks:
        - "default/net-workers"
      vmNetworkConfig:
        ipPoolRefs:
          - "pool-workers"
        gateway: "10.20.0.1"
        subnetMask: "255.255.255.0"
        dnsServers:
          - "10.20.0.53"
      # cpu, memory, volumes, ...
```

A machine-level `vmNetworkConfig` fully replaces the cluster-level one for the machines
created from that template: IP allocation uses the pools referenced here and the cloud-init
static network configuration uses this gateway, subnet mask and DNS. Machine types without it
(for example the control-plane template) keep using the cluster-level configuration, so the
override is fully backward compatible. Notes:

- The machine-level variant only accepts pool references (`ipPoolRef` or `ipPoolRefs`);
  the inline `ipPool` definition stays cluster-level.
- `vmNetworkConfig` and the static per-machine `networkConfig` are mutually exclusive on
  the same machine.
- Network-aware selection applies to the machine-level pool list as well.

## Encrypted storage

VM disks can be encrypted at rest by Longhorn (dm-crypt under the hood) without
any CAPHV-specific configuration: the provider resolves the StorageClass of an
image volume from the image itself, so an encrypted image transparently yields
encrypted root disks. Setup on the Harvester side:

1. Create the encryption key secret (the `CRYPTO_PBKDF` field is required by
   the Harvester validation):

```bash
kubectl create secret generic vm-encryption-key -n default \
  --from-literal=CRYPTO_KEY_VALUE="$(openssl rand -base64 32)" \
  --from-literal=CRYPTO_KEY_PROVIDER=secret \
  --from-literal=CRYPTO_KEY_CIPHER=aes-xts-plain64 \
  --from-literal=CRYPTO_KEY_HASH=sha256 \
  --from-literal=CRYPTO_KEY_SIZE=256 \
  --from-literal=CRYPTO_PBKDF=argon2i
```

2. Create an encrypted StorageClass referencing the secret (`encrypted: "true"`
   plus the six `csi.storage.k8s.io/*-secret-name/namespace` parameters).

3. Clone the node image into an encrypted one: a `VirtualMachineImage` with
   `sourceType: clone`, `securityParameters: {cryptoOperation: encrypt,
   sourceImageName: ..., sourceImageNamespace: ...}` and the annotation
   `harvesterhci.io/storageClassName: <encrypted-storage-class>`.

4. Reference the encrypted image in the machine template
   (`volumes[].imageName`). Root disks then live on the image's encrypted
   StorageClass. For encrypted data volumes, use `volumeType: storageClass`
   with the encrypted StorageClass directly.

Size the volumes strictly larger than the virtual size of the source image:
the LUKS header consumes space, and Longhorn rejects an encrypted volume equal
to the backing image size ("volume size should be larger than the backing
image size"). For a 10Gi image, request at least 11Gi.

This is the building block for the BSI APP.4.4.A20 requirement (encrypted data
storage); see `docs/compliance.md` for the full mapping.

## Failure domains

The provider discovers the failure domains of the target Harvester cluster and
publishes them in `HarvesterCluster.status.failureDomains`: one domain per
`topology.kubernetes.io/zone` value when every Harvester host carries the
label, otherwise one domain per host. CAPI aggregates them on the `Cluster`
object, the control plane provider spreads control plane machines across them,
and CAPHV pins each VM to its assigned domain with a required node affinity
(combined with any user provided `nodeAffinity`). This is what the BSI
APP.4.4.A19 elevated-protection requirement (control plane distributed across
failure zones) builds on.

Nothing needs to be configured: label the Harvester hosts with
`topology.kubernetes.io/zone` to group them into zones, or leave them
unlabeled to treat each host as its own domain. On a single host cluster there
is one domain and the spread is a no-op. Discovery requires the kubeconfig of
the identity secret to be allowed to list nodes on the Harvester cluster;
when it is not, the provider logs a warning and skips publication without
blocking the reconciliation.

## UEFI Secure Boot and vTPM

VMs boot with BIOS firmware by default. For measured or attested boot setups
(for example the BSI APP.4.4.A17 node attestation requirement), request UEFI,
Secure Boot and an emulated TPM per machine type in the HarvesterMachineTemplate:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterMachineTemplate
spec:
  template:
    spec:
      firmware:
        efi: true
        secureBoot: true
      tpm:
        enabled: true
        persistent: true
      # cpu, memory, volumes, ...
```

Notes:

- `secureBoot: true` requires `efi: true` (enforced by the webhook) and turns on
  the SMM CPU feature that Secure Boot needs.
- The guest image must be UEFI capable, and Secure Boot additionally needs a
  signed bootloader chain in the image; standard SLES and openSUSE cloud images
  qualify.
- `tpm.persistent: true` keeps the TPM state across reboots (supported by the
  KubeVirt version shipped with current Harvester releases).
- Machines without these blocks keep booting exactly as before.

## Backup and Disaster Recovery

### What to back up

| Component | Contains | Backup method |
|-----------|----------|---------------|
| Management cluster etcd | All CAPI resources (Cluster, Machine, HarvesterCluster, HarvesterMachine, IPPool, etc.) | etcd snapshot |
| Harvester cluster | VMs, PVCs, VM images, network configurations | Harvester backup / Longhorn backup |
| Identity secrets | Harvester kubeconfig used by CAPHV to communicate with Harvester | kubectl export or Vault |
| ClusterResourceSet ConfigMaps | CCM, CSI, CNI addon configurations | kubectl export or Git |
| IPPool resources | IP allocation state | kubectl export |

### Management cluster etcd backup

This is the most critical backup. All CAPI state lives in the management cluster's etcd.

For RKE2-based management clusters:

```bash
# On the management cluster node
sudo /var/lib/rancher/rke2/bin/etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
  --cert=/var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
  --key=/var/lib/rancher/rke2/server/tls/etcd/server-client.key \
  snapshot save /tmp/etcd-snapshot-$(date +%Y%m%d-%H%M%S).db
```

RKE2 also takes automatic snapshots (default: every 12 hours, 5 retained). Check with:

```bash
sudo ls -la /var/lib/rancher/rke2/server/db/snapshots/
```

### Export CAPI resources

For a portable backup of CAPI resources (useful for migration to a new management cluster):

```bash
#!/bin/bash
BACKUP_DIR="caphv-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

for kind in clusters.cluster.x-k8s.io machines.cluster.x-k8s.io \
  machinedeployments.cluster.x-k8s.io machinesets.cluster.x-k8s.io \
  rke2controlplanes.controlplane.cluster.x-k8s.io \
  harvesterclusters.infrastructure.cluster.x-k8s.io \
  harvestermachines.infrastructure.cluster.x-k8s.io \
  clusterresourcesets.addons.cluster.x-k8s.io \
  machinehealthchecks.cluster.x-k8s.io; do
  kubectl get "$kind" -A -o yaml > "$BACKUP_DIR/$kind.yaml" 2>/dev/null
done

# Export secrets (kubeconfig, cloud-init)
kubectl get secrets -A -l cluster.x-k8s.io/cluster-name -o yaml > "$BACKUP_DIR/secrets.yaml"

echo "Backup saved to $BACKUP_DIR/"
```

### Disaster recovery: rebuild management cluster

If the management cluster is lost but Harvester and its VMs are intact:

1. **Deploy a new management cluster** with RKE2, Rancher, and Turtles
2. **Install CAPHV** via CAPIProvider (see Installation section above)
3. **Re-apply CAPI resources** from the backup, pointing to the same Harvester:

```bash
# Apply in dependency order
kubectl apply -f "$BACKUP_DIR/harvesterclusters.infrastructure.cluster.x-k8s.io.yaml"
kubectl apply -f "$BACKUP_DIR/harvestermachines.infrastructure.cluster.x-k8s.io.yaml"
kubectl apply -f "$BACKUP_DIR/clusters.cluster.x-k8s.io.yaml"
kubectl apply -f "$BACKUP_DIR/machines.cluster.x-k8s.io.yaml"
# ... remaining resources
```

4. **Verify reconciliation**: The CAPHV controller discovers the existing VMs on Harvester
   and adopts them. No VMs are recreated if they already exist and match the expected state.

```bash
kubectl get clusters.cluster.x-k8s.io -A
kubectl get machines.cluster.x-k8s.io -A
```

### Harvester-side backup

Harvester VMs are backed by Longhorn volumes. Use Harvester's built-in VM backup feature or
Longhorn's volume backup to an S3-compatible target:

```bash
# Via Harvester API or UI: create a VM backup
# This captures all volumes (boot + data disks) as Longhorn snapshots
```

This is a secondary safety net. The primary recovery path is to re-provision workload
clusters from the management cluster's CAPI state, since CAPHV can recreate VMs from
scratch.

---

## Operational Runbooks

### Controller not reconciling

```bash
# Check controller logs
kubectl logs -n caphv-system deploy/caphv-controller-manager -c manager --tail=100

# Check if leader election is stuck (multi-replica setups)
kubectl get lease -n caphv-system

# Restart the controller
kubectl rollout restart deploy/caphv-controller-manager -n caphv-system
```

### Machine stuck in Provisioning

```bash
# Check the machine status
kubectl describe machine <machine-name> -n <ns>

# Check the HarvesterMachine status
kubectl describe harvestermachine <machine-name> -n <ns>

# Check the VM on Harvester
kubectl get vm -n <target-ns> --kubeconfig <harvester-kubeconfig>

# Check cloud-init status on the VM (SSH into it)
ssh <user>@<vm-ip> 'sudo cloud-init status --long'
```

### IP pool exhausted

```bash
# Check current allocations
kubectl get ippools.loadbalancer.harvesterhci.io -o yaml   # on the Harvester cluster; cluster-scoped

# Look at allocated IPs in status
kubectl get ippools.loadbalancer.harvesterhci.io <pool-name> -o jsonpath='{.status.allocated}' | jq .
# Note: use the full resource name (the short name resolves to Calico's IPPool on
# Harvester) and edit allocations by patching the main object: this CRD has no
# status subresource.

# If IPs are leaked (allocated but no corresponding machine), manually edit the IPPool:
kubectl edit ippool <pool-name> -n <ns>
# Remove stale entries from status.allocated
```

**Multi-pool fallback**: When `ipPoolRefs` is configured with multiple pools, the controller
tries pools in order and automatically falls back to the next pool when one is exhausted.
Check all pools if machines fail to allocate:

```bash
# List all configured pools
kubectl get harvesterclusters -n <ns> -o jsonpath='{.items[*].spec.vmNetworkConfig.ipPoolRefs}'

# Check each pool's allocation
for pool in pool-a pool-b pool-c; do
  echo "--- $pool ---"
  kubectl get ippool "$pool" -o jsonpath='{.status.allocated}' | jq .
done

# Check which pool a machine allocated from
kubectl get harvestermachine <name> -n <ns> -o jsonpath='{.status.allocatedPoolRef}'
```

### etcd member removal failed during remediation

If `caphv_etcd_member_remove_errors_total` is increasing:

```bash
# Check etcd member list from a healthy CP node
kubectl exec -it <rke2-cp-pod> -n kube-system -- etcdctl member list

# Manually remove a stale member if needed
kubectl exec -it <rke2-cp-pod> -n kube-system -- etcdctl member remove <member-id>
```

### Webhook certificate expired

```bash
# Check certificate status
kubectl get certificate -n caphv-system

# Force renewal
kubectl delete secret caphv-webhook-tls -n caphv-system
# cert-manager will automatically re-issue the certificate
```
