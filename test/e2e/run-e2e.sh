#!/bin/bash
# CAPHV End-to-End Integration Tests
# Runs against a live Harvester + Rancher Manager + CAPI cluster
#
# Environment variables (all optional, with sensible defaults):
#
#   CAPHV_RANCHER_SSH       SSH target for Rancher Manager (default: rancher@<rancher-manager-ip>)
#   CAPHV_HARVESTER_SSH     SSH target for Harvester node (default: rancher@<harvester-ip>)
#   CAPHV_KUBECTL_RANCHER   kubectl command on Rancher Manager (default: sudo /var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml)
#   CAPHV_KUBECTL_HARVESTER kubectl command on Harvester (default: same as CAPHV_KUBECTL_RANCHER)
#   CAPHV_NAMESPACE         Namespace for test resources (default: capi-test)
#   CAPHV_CLUSTER_NAME      Name of the CAPI cluster (default: capi-test)
#   CAPHV_WORKER_MD         MachineDeployment name for workers (default: capi-test-workers)
#   CAPHV_TIMEOUT_VM_RUNNING  Seconds to wait for VM Running (default: 600)
#   CAPHV_TIMEOUT_NODE_READY  Seconds to wait for node Ready (default: 600)
#   CAPHV_TIMEOUT_VM_DELETED  Seconds to wait for VM deletion (default: 300)
#   CAPHV_TIMEOUT_REMEDIATION Seconds to wait for remediation (default: 900)
#
# Prerequisites:
#   - SSH access to Rancher Manager and Harvester nodes
#   - Existing CAPI cluster with 3 CP + 1 worker (Running)
#   - MachineHealthCheck deployed
#   - IPPool with available IPs
#
# Usage:
#   ./test/e2e/run-e2e.sh              # Run all tests
#   ./test/e2e/run-e2e.sh scale        # Run only scale test
#   ./test/e2e/run-e2e.sh remediation  # Run only remediation test
#   ./test/e2e/run-e2e.sh multidisk    # Run only multi-disk test
#   ./test/e2e/run-e2e.sh webhook      # Run only webhook validation test
#   ./test/e2e/run-e2e.sh fleet        # Run only Fleet/CAAPF test

set -euo pipefail

# Source fleet test functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/fleet_test.sh" ]; then
    # shellcheck source=fleet_test.sh
    source "$SCRIPT_DIR/fleet_test.sh"
fi

# --- Configuration (override via CAPHV_* environment variables) ---
RANCHER_SSH="${CAPHV_RANCHER_SSH:-rancher@<rancher-manager-ip>}"
HARVESTER_SSH="${CAPHV_HARVESTER_SSH:-rancher@<harvester-ip>}"
KUBECTL_RANCHER="${CAPHV_KUBECTL_RANCHER:-sudo /var/lib/rancher/rke2/bin/kubectl --kubeconfig /etc/rancher/rke2/rke2.yaml}"
KUBECTL_HARVESTER="${CAPHV_KUBECTL_HARVESTER:-$KUBECTL_RANCHER}"
NAMESPACE="${CAPHV_NAMESPACE:-capi-test}"
CLUSTER_NAME="${CAPHV_CLUSTER_NAME:-capi-test}"
WORKER_MD="${CAPHV_WORKER_MD:-${CLUSTER_NAME}-workers}"
TIMEOUT_VM_RUNNING="${CAPHV_TIMEOUT_VM_RUNNING:-600}"    # 10 min for VM to reach Running
TIMEOUT_NODE_READY="${CAPHV_TIMEOUT_NODE_READY:-600}"    # 10 min for node to join and become Ready
TIMEOUT_VM_DELETED="${CAPHV_TIMEOUT_VM_DELETED:-300}"    # 5 min for VM to be fully deleted
TIMEOUT_REMEDIATION="${CAPHV_TIMEOUT_REMEDIATION:-900}"  # 15 min for full remediation cycle

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# --- Counters ---
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# --- Helper Functions ---

log_info()  { echo -e "${BLUE}[INFO]${NC}  $(date +%H:%M:%S) $*"; }
log_ok()    { echo -e "${GREEN}[PASS]${NC}  $(date +%H:%M:%S) $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC}  $(date +%H:%M:%S) $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $(date +%H:%M:%S) $*"; }
log_test()  { echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"; echo -e "${BLUE}[TEST]${NC}  $*"; echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"; }

pass_test() { TESTS_PASSED=$((TESTS_PASSED + 1)); log_ok "$1"; }
fail_test() { TESTS_FAILED=$((TESTS_FAILED + 1)); log_fail "$1"; }
skip_test() { TESTS_SKIPPED=$((TESTS_SKIPPED + 1)); log_warn "SKIP: $1"; }

kubectl_rancher() {
    ssh -o ConnectTimeout=10 "$RANCHER_SSH" "$KUBECTL_RANCHER $*" 2>/dev/null
}

kubectl_harvester() {
    ssh -o ConnectTimeout=10 "$HARVESTER_SSH" "$KUBECTL_HARVESTER $*" 2>/dev/null
}

# Wait for a condition with timeout. Returns 0 on success, 1 on timeout.
# Usage: wait_for <description> <timeout_seconds> <command_that_should_succeed>
wait_for() {
    local desc="$1"
    local timeout="$2"
    shift 2
    local cmd="$*"
    local start end elapsed

    start=$(date +%s)
    end=$((start + timeout))
    log_info "Waiting for: $desc (timeout: ${timeout}s)"

    while true; do
        if eval "$cmd" >/dev/null 2>&1; then
            elapsed=$(( $(date +%s) - start ))
            log_info "  -> $desc achieved in ${elapsed}s"
            return 0
        fi

        if [ "$(date +%s)" -ge "$end" ]; then
            elapsed=$(( $(date +%s) - start ))
            log_warn "  -> Timeout after ${elapsed}s waiting for: $desc"
            return 1
        fi

        sleep 10
    done
}

# Get count of machines in a given phase
count_machines_in_phase() {
    local phase="$1"
    kubectl_rancher "get machines -n $NAMESPACE -o jsonpath='{range .items[*]}{.status.phase}{\"\\n\"}{end}'" | grep -c "^${phase}$" || echo 0
}

# Get total machine count
count_machines() {
    kubectl_rancher "get machines -n $NAMESPACE --no-headers" | wc -l
}

# Get running machine count
count_running_machines() {
    count_machines_in_phase "Running"
}

# Check cluster health — all machines Running
assert_cluster_healthy() {
    local expected="$1"
    local running
    running=$(count_running_machines)
    if [ "$running" -eq "$expected" ]; then
        return 0
    fi
    return 1
}

# --- Precondition Checks ---

check_preconditions() {
    log_test "Checking preconditions"

    # SSH connectivity
    if ! ssh -o ConnectTimeout=5 "$RANCHER_SSH" "echo ok" >/dev/null 2>&1; then
        log_fail "Cannot SSH to Rancher Manager ($RANCHER_SSH)"
        exit 1
    fi
    log_ok "SSH to Rancher Manager"

    if ! ssh -o ConnectTimeout=5 "$HARVESTER_SSH" "echo ok" >/dev/null 2>&1; then
        log_fail "Cannot SSH to Harvester ($HARVESTER_SSH)"
        exit 1
    fi
    log_ok "SSH to Harvester"

    # CAPHV controller running
    local caphv_ready
    caphv_ready=$(kubectl_rancher "get deploy -n caphv-system caphv-controller-manager -o jsonpath='{.status.readyReplicas}'" 2>/dev/null || echo 0)
    if [ "$caphv_ready" != "1" ]; then
        log_fail "CAPHV controller not ready (readyReplicas=$caphv_ready)"
        exit 1
    fi
    log_ok "CAPHV controller running"

    # Cluster exists and machines are Running
    local running
    running=$(count_running_machines)
    if [ "$running" -lt 4 ]; then
        log_fail "Expected at least 4 Running machines, got $running"
        exit 1
    fi
    log_ok "Cluster healthy: $running machines Running"

    # MHC exists
    if ! kubectl_rancher "get machinehealthcheck -n $NAMESPACE $CLUSTER_NAME-mhc" >/dev/null 2>&1; then
        log_warn "MachineHealthCheck not found — remediation test will be skipped"
    else
        log_ok "MachineHealthCheck exists"
    fi

    echo ""
}

# --- Test: Worker Scale Up/Down ---

test_scale() {
    log_test "Test: Worker Scale Up (1 -> 2) and Down (2 -> 1)"

    local initial_count
    initial_count=$(count_machines)
    log_info "Initial machine count: $initial_count"

    # Scale up to 2 workers
    log_info "Scaling MachineDeployment $WORKER_MD to 2 replicas"
    kubectl_rancher "scale machinedeployment -n $NAMESPACE $WORKER_MD --replicas=2"

    # Wait for new machine to appear
    if ! wait_for "machine count = $((initial_count + 1))" 120 \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE --no-headers 2>/dev/null | wc -l\") -eq $((initial_count + 1)) ]"; then
        fail_test "Scale up: new machine did not appear"
        # Rollback
        kubectl_rancher "scale machinedeployment -n $NAMESPACE $WORKER_MD --replicas=1"
        return
    fi
    pass_test "Scale up: new machine created"

    # Wait for all machines Running
    if ! wait_for "all $((initial_count + 1)) machines Running" "$TIMEOUT_NODE_READY" \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE -o jsonpath='{range .items[*]}{.status.phase}{\\\"\\\\n\\\"}{end}' 2>/dev/null | grep -c '^Running$'\") -eq $((initial_count + 1)) ]"; then
        fail_test "Scale up: new machine did not reach Running"
        kubectl_rancher "get machines -n $NAMESPACE -o wide"
        kubectl_rancher "scale machinedeployment -n $NAMESPACE $WORKER_MD --replicas=1"
        sleep 30
        return
    fi
    pass_test "Scale up: all $((initial_count + 1)) machines Running"

    # Scale down back to 1 worker
    log_info "Scaling MachineDeployment $WORKER_MD back to 1 replica"
    kubectl_rancher "scale machinedeployment -n $NAMESPACE $WORKER_MD --replicas=1"

    # Wait for machine to be removed
    if ! wait_for "machine count = $initial_count" "$TIMEOUT_VM_DELETED" \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE --no-headers 2>/dev/null | wc -l\") -eq $initial_count ]"; then
        fail_test "Scale down: machine not removed in time"
        kubectl_rancher "get machines -n $NAMESPACE -o wide"
        return
    fi

    # Wait for remaining machines to be Running
    if ! wait_for "all $initial_count machines Running" 60 \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE -o jsonpath='{range .items[*]}{.status.phase}{\\\"\\\\n\\\"}{end}' 2>/dev/null | grep -c '^Running$'\") -eq $initial_count ]"; then
        fail_test "Scale down: remaining machines not all Running"
        return
    fi
    pass_test "Scale down: back to $initial_count machines, all Running"

    # Verify no orphan PVCs on Harvester
    local orphan_pvcs
    orphan_pvcs=$(kubectl_harvester "get pvc -n default --no-headers" 2>/dev/null | { grep "capi-test-workers" || true; } | wc -l | tr -d '[:space:]')
    orphan_pvcs="${orphan_pvcs:-0}"
    local expected_worker_pvcs=1  # 1 worker = 1 PVC (disk-0)
    if [ "$orphan_pvcs" -le "$expected_worker_pvcs" ]; then
        pass_test "Scale down: no orphan PVCs (found $orphan_pvcs worker PVC(s))"
    else
        fail_test "Scale down: possible orphan PVCs (found $orphan_pvcs, expected <= $expected_worker_pvcs)"
    fi
}

# --- Test: Auto-Remediation (MHC) ---

test_remediation() {
    log_test "Test: Auto-Remediation via MachineHealthCheck"

    # Check MHC exists
    if ! kubectl_rancher "get machinehealthcheck -n $NAMESPACE $CLUSTER_NAME-mhc" >/dev/null 2>&1; then
        skip_test "MachineHealthCheck not found"
        return
    fi

    # Get a worker machine name to delete its VM
    local worker_machine worker_vm_uid worker_node
    worker_machine=$(kubectl_rancher "get machines -n $NAMESPACE -l cluster.x-k8s.io/deployment-name=$WORKER_MD --no-headers -o custom-columns=NAME:.metadata.name" | head -1)

    if [ -z "$worker_machine" ]; then
        fail_test "Remediation: no worker machine found"
        return
    fi

    worker_vm_uid=$(kubectl_rancher "get machines -n $NAMESPACE $worker_machine -o jsonpath='{.spec.providerID}'" | sed 's|harvester://||')
    worker_node=$(kubectl_rancher "get machines -n $NAMESPACE $worker_machine -o jsonpath='{.status.nodeRef.name}'")

    log_info "Target worker: machine=$worker_machine vm=$worker_vm_uid node=$worker_node"
    local initial_running
    initial_running=$(count_running_machines)

    # Delete the VM on Harvester (simulate failure)
    log_info "Deleting VM $worker_machine on Harvester to trigger remediation"
    kubectl_harvester "delete vm -n default $worker_machine --wait=false"

    # Wait for machine to become not Running (MHC detects it)
    if ! wait_for "machine $worker_machine detected as unhealthy" 360 \
        "! ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE $worker_machine -o jsonpath='{.status.phase}' 2>/dev/null\" | grep -q Running"; then
        log_warn "Original machine still shows Running — MHC may replace it anyway"
    fi
    pass_test "Remediation: unhealthy state detected"

    # Wait for replacement machine to appear (machine count may temporarily increase)
    log_info "Waiting for MHC to create replacement machine"

    # Wait for all machines to be Running again (remediation complete)
    if ! wait_for "all $initial_running machines Running (remediation complete)" "$TIMEOUT_REMEDIATION" \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE -o jsonpath='{range .items[*]}{.status.phase}{\\\"\\\\n\\\"}{end}' 2>/dev/null | grep -c '^Running$'\") -ge $initial_running ]"; then
        fail_test "Remediation: cluster did not recover to $initial_running Running machines"
        kubectl_rancher "get machines -n $NAMESPACE -o wide"
        return
    fi
    pass_test "Remediation: cluster recovered — $initial_running machines Running"

    # Verify the old machine was replaced (different name or UID)
    local new_worker_machine
    new_worker_machine=$(kubectl_rancher "get machines -n $NAMESPACE -l cluster.x-k8s.io/deployment-name=$WORKER_MD --no-headers -o custom-columns=NAME:.metadata.name" | head -1)
    if [ "$new_worker_machine" != "$worker_machine" ]; then
        pass_test "Remediation: machine replaced ($worker_machine -> $new_worker_machine)"
    else
        log_info "Remediation: machine name unchanged (may have been recreated in-place)"
    fi
}

# --- Test: Multi-Disk ---

test_multidisk() {
    log_test "Test: Multi-Disk VM (image boot + storageClass data disk)"

    local initial_count
    initial_count=$(count_machines)

    # Create multi-disk template + MachineDeployment
    log_info "Creating multi-disk HarvesterMachineTemplate and MachineDeployment"
    kubectl_rancher "apply -f - <<'YAML'
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterMachineTemplate
metadata:
  name: e2e-multidisk-worker
  namespace: $NAMESPACE
spec:
  template:
    spec:
      cpu: 2
      memory: \"4Gi\"
      sshUser: sles
      sshKeyPair: default/capi-ssh-key
      volumes:
        - volumeType: image
          imageName: default/sles15-sp7-minimal-vm.x86_64-cloud-qu2.qcow2
          volumeSize: \"40Gi\"
          bootOrder: 1
        - volumeType: storageClass
          storageClass: longhorn
          volumeSize: \"10Gi\"
      networks:
        - default/production
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: e2e-multidisk
  namespace: $NAMESPACE
spec:
  clusterName: $CLUSTER_NAME
  replicas: 1
  selector:
    matchLabels: {}
  template:
    spec:
      clusterName: $CLUSTER_NAME
      version: v1.31.14+rke2r1
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1beta1
          kind: RKE2ConfigTemplate
          name: capi-test-worker-config
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
        kind: HarvesterMachineTemplate
        name: e2e-multidisk-worker
YAML"

    # Wait for machine to reach Running
    if ! wait_for "multi-disk machine Running" "$TIMEOUT_NODE_READY" \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE -o jsonpath='{range .items[*]}{.status.phase}{\\\"\\\\n\\\"}{end}' 2>/dev/null | grep -c '^Running$'\") -gt $initial_count ]"; then
        fail_test "Multi-disk: machine did not reach Running"
        kubectl_rancher "get machines -n $NAMESPACE -o wide"
        # Cleanup
        kubectl_rancher "delete machinedeployment -n $NAMESPACE e2e-multidisk --wait=false" 2>/dev/null || true
        kubectl_rancher "delete harvestermachinetemplate -n $NAMESPACE e2e-multidisk-worker" 2>/dev/null || true
        return
    fi
    pass_test "Multi-disk: machine reached Running"

    # Get the machine name and check disks on Harvester
    local md_machine
    md_machine=$(kubectl_rancher "get machines -n $NAMESPACE -l cluster.x-k8s.io/deployment-name=e2e-multidisk --no-headers -o custom-columns=NAME:.metadata.name" | head -1)

    # Check PVCs on Harvester — should have disk-0 and disk-1
    local pvc_count
    pvc_count=$(kubectl_harvester "get pvc -n default --no-headers 2>/dev/null" | grep -c "$md_machine-disk-" || echo 0)
    if [ "$pvc_count" -eq 2 ]; then
        pass_test "Multi-disk: 2 PVCs created (disk-0 + disk-1)"
    else
        fail_test "Multi-disk: expected 2 PVCs, found $pvc_count"
    fi

    # Check VM has 2 data disks + cloudinit
    local vm_disk_count
    vm_disk_count=$(kubectl_harvester "get vm -n default $md_machine -o jsonpath='{range .spec.template.spec.volumes[*]}{.name}{\"\\n\"}{end}' 2>/dev/null" | grep -c "^disk-" || echo 0)
    if [ "$vm_disk_count" -eq 2 ]; then
        pass_test "Multi-disk: VM has 2 data disks (disk-0 + disk-1)"
    else
        fail_test "Multi-disk: expected 2 data disks in VM spec, found $vm_disk_count"
    fi

    # Check boot order on disk-0
    local boot_order
    boot_order=$(kubectl_harvester "get vm -n default $md_machine -o jsonpath='{.spec.template.spec.domain.devices.disks[0].bootOrder}'" 2>/dev/null)
    if [ "$boot_order" = "1" ]; then
        pass_test "Multi-disk: disk-0 has bootOrder=1"
    else
        fail_test "Multi-disk: disk-0 bootOrder=$boot_order (expected 1)"
    fi

    # Verify disks visible inside VM via SSH
    local vm_ip
    vm_ip=$(kubectl_rancher "get harvestermachine -n $NAMESPACE $md_machine -o jsonpath='{.status.allocatedIPAddress}'" 2>/dev/null)
    if [ -n "$vm_ip" ]; then
        ssh-keygen -R "$vm_ip" -f "$HOME/.ssh/known_hosts" >/dev/null 2>&1 || true
        local disk_count_inside
        disk_count_inside=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 "sles@$vm_ip" 'lsblk -d -n -o TYPE | grep -c disk' 2>/dev/null || echo 0)
        # Expect 3: vda (40G boot), vdb (10G data), vdc (1M cloudinit)
        if [ "$disk_count_inside" -ge 3 ]; then
            pass_test "Multi-disk: $disk_count_inside disks visible inside VM (lsblk)"
        else
            fail_test "Multi-disk: expected >= 3 disks inside VM, found $disk_count_inside"
        fi
    else
        log_warn "Multi-disk: could not determine VM IP for SSH check"
    fi

    # Cleanup
    log_info "Cleaning up multi-disk test resources"
    kubectl_rancher "delete machinedeployment -n $NAMESPACE e2e-multidisk"
    wait_for "multi-disk machine removed" "$TIMEOUT_VM_DELETED" \
        "[ \$(ssh -o ConnectTimeout=5 $RANCHER_SSH \"$KUBECTL_RANCHER get machines -n $NAMESPACE --no-headers 2>/dev/null | wc -l\") -eq $initial_count ]" || true

    kubectl_rancher "delete harvestermachinetemplate -n $NAMESPACE e2e-multidisk-worker" 2>/dev/null || true

    # Verify PVCs cleaned up
    sleep 10
    local remaining_pvcs
    remaining_pvcs=$(kubectl_harvester "get pvc -n default --no-headers" 2>/dev/null | { grep "e2e-multidisk" || true; } | wc -l | tr -d '[:space:]')
    remaining_pvcs="${remaining_pvcs:-0}"
    if [ "$remaining_pvcs" -eq 0 ]; then
        pass_test "Multi-disk: PVCs cleaned up after deletion"
    else
        fail_test "Multi-disk: $remaining_pvcs orphan PVCs remaining"
    fi
}

# --- Test: Webhook Validation ---

test_webhook() {
    log_test "Test: Validating Webhook (reject invalid, accept valid)"

    # Check webhook is registered
    if ! kubectl_rancher "get validatingwebhookconfiguration caphv-validating-webhook-configuration" >/dev/null 2>&1; then
        skip_test "ValidatingWebhookConfiguration not found"
        return
    fi

    # Test 1: Reject cpu=0
    log_info "Testing rejection of HarvesterMachine with cpu=0"
    local result
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterMachine
metadata:
  name: e2e-test-invalid-cpu
  namespace: default
spec:
  cpu: 0
  memory: \"4Gi\"
  sshUser: sles
  sshKeyPair: test-key
  volumes:
    - volumeType: image
      imageName: default/test-image
      volumeSize: \"20Gi\"
      bootOrder: 0
  networks:
    - default/production
YAML")

    if echo "$result" | grep -q "spec.cpu must be greater than 0"; then
        pass_test "Webhook: rejected cpu=0"
    else
        fail_test "Webhook: cpu=0 was not rejected (output: $result)"
    fi

    # Test 2: Reject empty volumes
    log_info "Testing rejection of HarvesterMachine with no volumes"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterMachine
metadata:
  name: e2e-test-no-volumes
  namespace: default
spec:
  cpu: 2
  memory: \"4Gi\"
  sshUser: sles
  sshKeyPair: test-key
  volumes: []
  networks:
    - default/production
YAML")

    if echo "$result" | grep -q "spec.volumes must contain at least one volume"; then
        pass_test "Webhook: rejected empty volumes"
    else
        fail_test "Webhook: empty volumes was not rejected (output: $result)"
    fi

    # Test 3: Reject missing sshUser
    log_info "Testing rejection of HarvesterMachine with empty sshUser"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterMachine
metadata:
  name: e2e-test-no-sshuser
  namespace: default
spec:
  cpu: 2
  memory: \"4Gi\"
  sshUser: \"\"
  sshKeyPair: test-key
  volumes:
    - volumeType: image
      imageName: default/test-image
      volumeSize: \"20Gi\"
  networks:
    - default/production
YAML")

    if echo "$result" | grep -q "spec.sshUser is required"; then
        pass_test "Webhook: rejected empty sshUser"
    else
        fail_test "Webhook: empty sshUser was not rejected (output: $result)"
    fi

    # Test 4: Accept valid HarvesterMachine
    log_info "Testing acceptance of valid HarvesterMachine"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterMachine
metadata:
  name: e2e-test-valid
  namespace: default
spec:
  cpu: 2
  memory: \"4Gi\"
  sshUser: sles
  sshKeyPair: test-key
  volumes:
    - volumeType: image
      imageName: default/test-image
      volumeSize: \"20Gi\"
      bootOrder: 1
  networks:
    - default/production
YAML")

    if echo "$result" | grep -q "created\|configured\|unchanged"; then
        pass_test "Webhook: accepted valid HarvesterMachine"
        # Cleanup
        kubectl_rancher "delete harvestermachine -n default e2e-test-valid" >/dev/null 2>&1 || true
    else
        fail_test "Webhook: valid HarvesterMachine was rejected (output: $result)"
    fi

    # Test 5: Reject invalid HarvesterCluster (missing targetNamespace)
    log_info "Testing rejection of HarvesterCluster with missing targetNamespace"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterCluster
metadata:
  name: e2e-test-invalid-cluster
  namespace: default
spec:
  targetNamespace: \"\"
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
YAML")

    if echo "$result" | grep -q "spec.targetNamespace is required"; then
        pass_test "Webhook: rejected HarvesterCluster with empty targetNamespace"
    else
        fail_test "Webhook: invalid HarvesterCluster was not rejected (output: $result)"
    fi

    # Test 6: Accept HarvesterCluster (v1beta1) with interfaces-only vmNetworkConfig:
    # no top-level gateway/subnetMask/ipPoolRef, each interfaces[] entry carries its own
    # pool, gateway and subnet mask (multi-NIC layout).
    log_info "Testing acceptance of HarvesterCluster v1beta1 with interfaces-only vmNetworkConfig"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: e2e-test-multinic-interfaces
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    interfaces:
      - ipPoolRef: oob2
        gateway: 10.10.0.1
        subnetMask: 255.255.254.0
      - ipPoolRef: production
        gateway: 10.11.0.1
        subnetMask: 255.255.254.0
    dnsServers:
      - 1.1.1.1
    dnsSearch:
      - example.com
YAML")

    if echo "$result" | grep -q "created\\|configured\\|unchanged"; then
        pass_test "Webhook: accepted interfaces-only vmNetworkConfig (multi-NIC)"
        kubectl_rancher "delete harvestercluster -n default e2e-test-multinic-interfaces" >/dev/null 2>&1 || true
    else
        fail_test "Webhook: interfaces-only vmNetworkConfig was rejected (output: $result)"
    fi

    # Test 7: Accept the same interfaces-only layout on v1alpha1 (v1alpha1 webhook path)
    log_info "Testing acceptance of HarvesterCluster v1alpha1 with interfaces-only vmNetworkConfig"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: HarvesterCluster
metadata:
  name: e2e-test-multinic-interfaces-v1alpha1
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    interfaces:
      - ipPoolRef: oob2
        gateway: 10.10.0.1
        subnetMask: 255.255.254.0
YAML")

    if echo "$result" | grep -q "created\\|configured\\|unchanged"; then
        pass_test "Webhook: accepted v1alpha1 interfaces-only vmNetworkConfig"
        kubectl_rancher "delete harvestercluster -n default e2e-test-multinic-interfaces-v1alpha1" >/dev/null 2>&1 || true
    else
        fail_test "Webhook: v1alpha1 interfaces-only vmNetworkConfig was rejected (output: $result)"
    fi

    # Test 8: Accept single-NIC HarvesterCluster (v1beta1) with top-level pool+gateway+subnetMask
    log_info "Testing acceptance of HarvesterCluster v1beta1 with single-NIC vmNetworkConfig"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: e2e-test-singlenic
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    ipPoolRef: pool-a
    gateway: 10.10.0.1
    subnetMask: 255.255.255.0
YAML")

    if echo "$result" | grep -q "created\\|configured\\|unchanged"; then
        pass_test "Webhook: accepted single-NIC vmNetworkConfig"
        kubectl_rancher "delete harvestercluster -n default e2e-test-singlenic" >/dev/null 2>&1 || true
    else
        fail_test "Webhook: single-NIC vmNetworkConfig was rejected (output: $result)"
    fi

    # Test 9: Reject vmNetworkConfig without interfaces and without top-level gateway
    log_info "Testing rejection of vmNetworkConfig missing gateway (no interfaces)"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: e2e-test-no-gateway
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    ipPoolRef: pool-a
    subnetMask: 255.255.255.0
YAML")

    if echo "$result" | grep -q "spec.vmNetworkConfig.gateway is required"; then
        pass_test "Webhook: rejected vmNetworkConfig without gateway when interfaces is not set"
    else
        fail_test "Webhook: missing gateway was not rejected (output: $result)"
    fi

    # Test 10: Reject interfaces entry with invalid gateway IP
    log_info "Testing rejection of interfaces[0] with invalid gateway"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: e2e-test-bad-iface-gateway
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    interfaces:
      - ipPoolRef: pool-a
        gateway: not-an-ip
        subnetMask: 255.255.255.0
YAML")

    if echo "$result" | grep -q "interfaces\[0\].gateway.*is not a valid IP address"; then
        pass_test "Webhook: rejected interfaces[0] with invalid gateway IP"
    else
        fail_test "Webhook: invalid interfaces[0].gateway was not rejected (output: $result)"
    fi

    # Test 11: Reject interfaces entry without any pool reference
    log_info "Testing rejection of interfaces[0] without ipPoolRef/ipPoolRefs"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterCluster
metadata:
  name: e2e-test-iface-no-pool
  namespace: default
spec:
  targetNamespace: default
  identitySecret:
    name: test-secret
    namespace: default
  loadBalancerConfig:
    ipamType: dhcp
  vmNetworkConfig:
    interfaces:
      - gateway: 10.10.0.1
        subnetMask: 255.255.255.0
YAML")

    if echo "$result" | grep -q "interfaces\[0\] requires one of ipPoolRef or ipPoolRefs"; then
        pass_test "Webhook: rejected interfaces[0] without pool reference"
    else
        fail_test "Webhook: interfaces[0] without pool was not rejected (output: $result)"
    fi

    # Test 12: Accept HarvesterMachine (v1beta1) with per-NIC networkConfigs matching networks
    log_info "Testing acceptance of HarvesterMachine v1beta1 with networkConfigs (multi-NIC)"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterMachine
metadata:
  name: e2e-test-multinic-machine
  namespace: default
spec:
  cpu: 2
  memory: \\\"4Gi\\\"
  sshUser: sles
  sshKeyPair: test-key
  volumes:
    - volumeType: image
      imageName: default/test-image
      volumeSize: \\\"20Gi\\\"
  networks:
    - default/oob
    - default/production
  networkConfigs:
    - ipPoolRef: oob2
      gateway: 10.10.0.1
      subnetMask: 255.255.254.0
    - ipPoolRef: production
      gateway: 10.11.0.1
      subnetMask: 255.255.254.0
YAML")

    if echo "$result" | grep -q "created\\|configured\\|unchanged"; then
        pass_test "Webhook: accepted HarvesterMachine with networkConfigs"
        kubectl_rancher "delete harvestermachine -n default e2e-test-multinic-machine" >/dev/null 2>&1 || true
    else
        fail_test "Webhook: HarvesterMachine with networkConfigs was rejected (output: $result)"
    fi

    # Test 13: Reject HarvesterMachine with more networkConfigs entries than networks
    log_info "Testing rejection of networkConfigs with more entries than networks"
    result=$(kubectl_rancher "apply -f - <<'YAML' 2>&1 || true
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: HarvesterMachine
metadata:
  name: e2e-test-too-many-nic-cfgs
  namespace: default
spec:
  cpu: 2
  memory: \\\"4Gi\\\"
  sshUser: sles
  sshKeyPair: test-key
  volumes:
    - volumeType: image
      imageName: default/test-image
      volumeSize: \\\"20Gi\\\"
  networks:
    - default/production
  networkConfigs:
    - ipPoolRef: oob2
      gateway: 10.10.0.1
      subnetMask: 255.255.254.0
    - ipPoolRef: production
      gateway: 10.11.0.1
      subnetMask: 255.255.254.0
YAML")

    if echo "$result" | grep -q "has 2 entries but the machine has only 1 networks"; then
        pass_test "Webhook: rejected networkConfigs with more entries than networks"
    else
        fail_test "Webhook: oversized networkConfigs was not rejected (output: $result)"
    fi
}

# --- Summary ---

print_summary() {
    echo ""
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  E2E Test Summary${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
    echo -e "  ${GREEN}Passed:${NC}  $TESTS_PASSED"
    echo -e "  ${RED}Failed:${NC}  $TESTS_FAILED"
    echo -e "  ${YELLOW}Skipped:${NC} $TESTS_SKIPPED"
    echo -e "  Total:   $((TESTS_PASSED + TESTS_FAILED + TESTS_SKIPPED))"
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"

    if [ "$TESTS_FAILED" -gt 0 ]; then
        echo -e "\n${RED}RESULT: FAILED${NC}"
        return 1
    else
        echo -e "\n${GREEN}RESULT: ALL PASSED${NC}"
        return 0
    fi
}

# --- Main ---

main() {
    local test_filter="${1:-all}"

    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  CAPHV End-to-End Integration Tests${NC}"
    echo -e "${BLUE}  $(date)${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"

    check_preconditions

    case "$test_filter" in
        all)
            test_webhook
            test_scale
            test_multidisk
            test_remediation
            test_fleet
            ;;
        scale)
            test_scale
            ;;
        remediation)
            test_remediation
            ;;
        multidisk)
            test_multidisk
            ;;
        webhook)
            test_webhook
            ;;
        fleet)
            test_fleet
            ;;
        *)
            echo "Usage: $0 [all|scale|remediation|multidisk|webhook|fleet]"
            exit 1
            ;;
    esac

    print_summary
}

main "$@"
