# CAPHV Troubleshooting Guide

This guide covers common issues encountered when operating CAPHV (Cluster API Provider Harvester), their symptoms, root causes, and fixes. It assumes familiarity with Cluster API concepts (Cluster, Machine, MachineDeployment, ClusterClass) and Harvester HCI (VMs, IPPools, cloud-init).

All commands assume you have `kubectl` configured to reach the management cluster unless stated otherwise. Commands targeting the Harvester cluster or a workload cluster are explicitly noted.

---

## Table of Contents

- [IPPool Issues](#ippool-issues)
- [Cloud-Init Issues](#cloud-init-issues)
- [DHCP Issues](#dhcp-issues)
- [Turtles / Rancher Import Issues](#turtles--rancher-import-issues)
- [VM Creation Issues](#vm-creation-issues)
- [Machine Not Becoming Ready](#machine-not-becoming-ready)
- [etcd Issues](#etcd-issues)
- [Deletion Issues](#deletion-issues)
- [Useful Commands for Debugging](#useful-commands-for-debugging)

---

## IPPool Issues

### IP Exhaustion (Pool Full)

**Symptoms:**
- New HarvesterMachine objects stay in `Provisioning` state indefinitely.
- The HarvesterMachine condition `VMIPAllocated` shows `False` with reason `VMIPPoolExhausted`.
- Controller logs show: `failed to allocate new IP from pool: no IP addresses available in range set`.

**Cause:**
All IPs in the configured IPPool range have been allocated. This can happen when the pool is undersized for the cluster, or when previously deleted machines left leaked allocations (see IP Leak below).

**Fix:**
1. Check the current pool state on the Harvester cluster:
   ```bash
   # On Harvester
   kubectl get ippools.loadbalancer.harvesterhci.io <pool-name> -n <namespace> -o jsonpath='{.status.available}'
   ```
2. If the available count is 0, either expand the pool range or free leaked IPs (see IP Leak section).
3. To expand the pool, edit the IPPool on Harvester:
   ```bash
   # On Harvester
   kubectl edit ippool <pool-name> -n <namespace>
   ```
   Adjust `spec.ranges[0].rangeEnd` to include more addresses.
4. If you created the IPPool via the HarvesterCluster spec (inline `ipPool`), update the HarvesterCluster object's `vmNetworkConfig.ipPool.rangeEnd` field and the controller will reconcile the pool.

---

### Pool Not Found (Wrong IPPoolRef or Namespace Mismatch)

**Symptoms:**
- HarvesterCluster condition `VMIPPoolReady` is `False`.
- Controller logs show: `failed to get IPPool` or `IPPool not found`.
- Machines never receive an IP allocation.

**Cause:**
The `vmNetworkConfig.ipPoolRef` in the HarvesterCluster spec references a pool name that does not exist in the target namespace on Harvester, or the namespace does not match `spec.targetNamespace`.

**Fix:**
1. Verify the pool exists on Harvester in the correct namespace:
   ```bash
   # On Harvester
   kubectl get ippools.loadbalancer.harvesterhci.io -A
   ```
2. Confirm the `ipPoolRef` value in your HarvesterCluster matches `<namespace>/<name>` or just `<name>` if the pool is in the same namespace as `targetNamespace`:
   ```bash
   kubectl get harvestercluster <name> -n <ns> -o jsonpath='{.spec.vmNetworkConfig.ipPoolRef}'
   ```
3. If using ClusterClass with a topology variable for `ipPoolRef`, verify the variable value in the Cluster's `spec.topology.variables`.
4. The `ipPoolRef` format must be `<namespace>/<name>` when the pool is in a different namespace from `targetNamespace`. A bare name is resolved against `targetNamespace`.

---

### IP Leak (Machine Deleted But IP Not Released)

**Symptoms:**
- A machine was deleted but the pool's `status.available` count did not increase.
- The IP still appears in `status.allocated` of the IPPool on Harvester.
- New machines fail to get that IP even though the original machine no longer exists.

**Cause:**
The CAPHV controller calls `Store.Release()` during machine deletion to free the IP. If the controller was not running during deletion (e.g., it was restarting or the finalizer was removed manually), the release may not have happened. The IPPool `status.allocated` map retains the stale entry.

**Fix:**
1. Identify the leaked IP:
   ```bash
   # On Harvester
   kubectl get ippools.loadbalancer.harvesterhci.io <pool-name> -n <ns> -o jsonpath='{.status.allocated}' | python3 -m json.tool
   ```
2. Cross-reference with existing HarvesterMachine objects:
   ```bash
   kubectl get harvestermachine -A -o custom-columns=NAME:.metadata.name,IP:.status.allocatedIPAddress
   ```
3. For any IP in `status.allocated` whose machine no longer exists, manually release it:
   ```bash
   # On Harvester
   kubectl edit ippool <pool-name> -n <ns>
   ```
   Remove the leaked entry from `status.allocated` and increment `status.available` by the number of entries removed.
4. To prevent future leaks, ensure the CAPHV controller is always running and that HarvesterMachine finalizers (`harvestermachine.infrastructure.cluster.x-k8s.io/finalizer` (older objects may carry the legacy `harvestermachine.infrastructure.cluster.x-k8s.io`)) are never removed manually.

---

### Dual Allocation (Same IP Assigned to Two Machines)

**Symptoms:**
- Two VMs on Harvester have the same IP address.
- IP conflicts on the network (ARP flapping, intermittent connectivity).
- Both HarvesterMachine objects show the same value in `status.allocatedIPAddress`.

**Cause:**
This was a bug in versions prior to v0.2.0 where `Store.Reserve()` did not update `status.allocated` correctly. Every call to `AllocateVMIPFromPool()` saw an empty allocated map and returned the first IP in the range.

**Fix:**
1. Upgrade CAPHV to v0.2.0 or later. The `Store.Reserve()` function now properly writes to `status.allocated` before returning.
2. For an already-affected cluster, manually resolve the conflict:
   ```bash
   # On Harvester - check allocated map
   kubectl get ippools.loadbalancer.harvesterhci.io <pool-name> -n <ns> -o jsonpath='{.status.allocated}' | python3 -m json.tool
   ```
3. Delete one of the conflicting machines and let CAPHV recreate it with a new unique IP:
   ```bash
   kubectl delete machine <conflicting-machine-name> -n <ns>
   ```
4. The MachineHealthCheck (if configured) will detect the missing machine and trigger replacement automatically.

---

## Cloud-Init Issues

### Network Config Format (v1 vs. v2)

**Symptoms:**
- The VM boots but network interfaces are unconfigured.
- `cloud-init status --long` on the VM shows success, but no IP is set on eth0.
- `/var/log/cloud-init.log` shows warnings about unrecognized network-config format.

**Cause:**
Harvester uses cloud-init network-config version 1. SLES/openSUSE with wicked requires v1 format. If the network config is accidentally provided in v2 format (e.g., from a custom template or manual edit), wicked ignores it entirely.

CAPHV generates v1 format automatically via `buildNetworkDataStatic()`:
```yaml
version: 1
config:
  - type: physical
    name: eth0
    subnets:
      - type: static
        address: 172.16.3.40
        netmask: 255.255.0.0
        gateway: 172.16.0.1
```

**Fix:**
1. SSH into the VM and check the rendered network config:
   ```bash
   cat /var/lib/cloud/seed/nocloud/network-config
   ```
2. Confirm it starts with `version: 1`. If it shows `version: 2`, the cloud-init secret on Harvester was built incorrectly.
3. Check the cloud-init secret on Harvester:
   ```bash
   # On Harvester
   kubectl get secret <machine-name>-cloud-init -n <target-ns> -o jsonpath='{.data.networkdata}' | base64 -d
   ```
4. If using custom bootstrap data, ensure your bootstrap provider does not inject a network-config v2 payload that overrides the CAPHV-generated v1 config.

---

### Cloud-Init Secret Keys Must Be Lowercase

**Symptoms:**
- The VM boots but cloud-init does not apply userdata or networkdata.
- `cloud-init status --long` shows "no datasource" or the config appears empty.
- The cloud-init secret exists on Harvester but the VM ignores it.

**Cause:**
KubeVirt's CloudInitNoCloud datasource reads secret keys as lowercase `userdata` and `networkdata`. If the keys are camelCase (`userData`, `networkData`), they are silently ignored. This is a KubeVirt requirement, not a cloud-init one.

**Fix:**
1. Check the secret key names on Harvester:
   ```bash
   # On Harvester
   kubectl get secret <machine-name>-cloud-init -n <target-ns> -o jsonpath='{.data}' | python3 -m json.tool
   ```
2. The keys must be exactly `userdata` and `networkdata` (all lowercase). If they are wrong, the secret was created outside of CAPHV or by a modified version.
3. CAPHV always generates lowercase keys. If you are manually creating secrets, ensure the key names match exactly.

---

### Cloud-Init Not Applying

**Symptoms:**
- The VM is running but packages are not installed, SSH keys are not configured, or RKE2 is not bootstrapped.
- `cloud-init status --long` on the VM shows an error or `status: not started`.

**Cause:**
Multiple possible causes:
- The cloud-init secret does not exist on Harvester.
- The secret exists but is in the wrong namespace.
- The qemu-guest-agent is not running (prevents IP reporting, not cloud-init itself).
- Cloud-init ran but encountered errors in the userdata script.

**Fix:**
1. SSH into the VM (or use the Harvester VNC console) and check cloud-init status:
   ```bash
   cloud-init status --long
   ```
2. Check cloud-init logs:
   ```bash
   cat /var/log/cloud-init.log
   cat /var/log/cloud-init-output.log
   ```
3. Verify the cloud-init secret exists on Harvester:
   ```bash
   # On Harvester
   kubectl get secret <machine-name>-cloud-init -n <target-ns>
   ```
4. Verify the VM references the secret correctly:
   ```bash
   # On Harvester
   kubectl get vm <machine-name> -n <target-ns> -o jsonpath='{.spec.template.spec.volumes}' | python3 -m json.tool
   ```
   Look for the `cloudInitNoCloud` volume source with the correct secret name.
5. If cloud-init completed with errors, fix the userdata and delete + recreate the machine to trigger a fresh cloud-init run. Cloud-init only runs once per instance ID.

---

### Static IP Not Set (Wicked Nanny Override)

**Symptoms:**
- The VM boots and cloud-init reports success, but the static IP configured in networkdata is replaced by a link-local address or no address at all.
- Running `ip addr show eth0` shows a different IP than expected, or `169.254.x.x`.

**Cause:**
SLES uses wicked as its network manager. The wicked "nanny" daemon periodically reconciles interface configuration. If the networkdata secret was not properly generated or applied, wicked falls back to its default behavior (often link-local or no address).

**Fix:**
1. SSH into the VM and check what wicked thinks the config should be:
   ```bash
   wicked show-config
   wicked ifstatus eth0
   ```
2. Check if cloud-init wrote the network config to the correct location:
   ```bash
   cat /etc/sysconfig/network/ifcfg-eth0
   ```
3. Verify that the `networkdata` key exists in the cloud-init secret on Harvester (see Cloud-Init Secret Keys section).
4. If the networkdata is correct in the secret but not applied, check that the cloud-init NoCloud datasource found the network config:
   ```bash
   cat /var/lib/cloud/seed/nocloud/network-config
   ```
5. As a last resort, manually configure the interface:
   ```bash
   ip addr add <address>/<prefix> dev eth0
   ip route add default via <gateway>
   ```

---

## DHCP Issues

### Wicked BPF Bug (DHCP Drops on KubeVirt)

**Symptoms:**
- A VM configured for DHCP never gets an IP address.
- `wicked ifup eth0` hangs or times out.
- `journalctl -u wicked` shows no DHCP offers received.
- Running `tcpdump -i eth0 port 67 or port 68` shows DHCP offers arriving at the interface but wicked does not process them.

**Cause:**
This is a kernel/wicked incompatibility on KubeVirt's virtio-net interfaces. Wicked uses `AF_PACKET` with `SOCK_DGRAM` and attaches a BPF filter that uses link-layer (Ethernet header) offsets. However, `SOCK_DGRAM` strips the link-layer header before delivering data to BPF, so BPF sees network-layer data at link-layer offsets. The result: every DHCP response is silently dropped by the filter.

This affects SLES, openSUSE, and any distro using wicked as the DHCP client on KubeVirt/virtio-net.

**Fix:**
CAPHV v0.2.3+ automatically works around this by injecting ISC dhclient via cloud-init `bootcmd`. ISC dhclient uses `AF_PACKET` with `SOCK_RAW` (Linux Packet Filter / LPF), which preserves the link-layer header for BPF. This makes DHCP work correctly.

If you are on an older CAPHV version:
1. Upgrade to v0.2.3 or later.
2. If you cannot upgrade, manually install `dhcp-client` (ISC dhclient) in the VM image and configure it instead of wicked for DHCP.

---

### KubeVirt Bridge Binding Intercepts External DHCP

**Symptoms:**
- The VM is connected to a network with a DHCP server, but DHCP requests from the VM never reach the external DHCP server.
- The VM gets an IP from KubeVirt's internal DHCP (often a 10.x.x.x address) instead of the expected subnet.

**Cause:**
KubeVirt's default bridge binding creates a bridge between the pod network and the VM interface. This bridge runs its own in-VM DHCP server that intercepts DHCP traffic. External DHCP servers on the physical network cannot be reached directly.

**Fix:**
1. CAPHV handles this correctly: it uses an in-VM dhclient that gets its lease from the KubeVirt bridge DHCP server, which in turn mirrors the pod IP.
2. If you need external DHCP (e.g., from a physical DHCP server on the VLAN), you must use masquerade or passthrough binding instead of bridge. However, CAPHV's default configuration uses bridge binding, which works with the dhclient workaround.
3. For standard CAPHV usage (IPPool-based static IPs or DHCP mode), no action is needed. The in-VM DHCP client obtains the correct address.

---

### dhclient Flags: `-1` vs `-d`

**Symptoms:**
- The VM hangs during boot and never finishes cloud-init.
- `cloud-init status` shows `status: running` indefinitely.
- RKE2 never starts because cloud-init never completes.

**Cause:**
If dhclient is started with the `-d` flag (foreground/debug mode), it never forks to the background. Since it is launched from a cloud-init `bootcmd`, cloud-init waits for the command to exit. dhclient in foreground mode runs forever, blocking all subsequent cloud-init stages (including RKE2 bootstrap).

**Fix:**
1. CAPHV uses `-1` (try once, fork to background): dhclient sends one DHCP request, obtains a lease, and the parent process exits so cloud-init can continue. The child process remains in the background to handle lease renewals.
2. Never use `-d` in cloud-init bootcmd. If you see a custom bootstrap template using `-d`, replace it with `-1`.
3. The correct dhclient invocation generated by CAPHV is:
   ```bash
   dhclient -1 -sf /usr/local/bin/dhclient-script-caphv.sh -lf /tmp/dhclient-eth0.lease -pf /tmp/dhclient-eth0.pid eth0
   ```

---

### No Networkdata in DHCP Mode

**Symptoms:**
- In DHCP mode, the VM gets an IP via dhclient, but shortly after boot the IP disappears or changes.
- `wicked ifstatus eth0` shows wicked reconfiguring the interface.

**Cause:**
If a `networkdata` key is present in the cloud-init secret, cloud-init writes network configuration files that wicked's nanny daemon picks up. The nanny then overwrites whatever dhclient configured, replacing the DHCP-assigned IP with whatever wicked thinks the config should be (often nothing, since the networkdata may not have a valid DHCP stanza for wicked).

**Fix:**
1. CAPHV correctly omits the `networkdata` key from the cloud-init secret when operating in DHCP mode. This prevents wicked from interfering.
2. Verify there is no `networkdata` key in the secret:
   ```bash
   # On Harvester
   kubectl get secret <machine-name>-cloud-init -n <target-ns> -o jsonpath='{.data}' | python3 -m json.tool
   ```
   In DHCP mode, only `userdata` should be present.
3. If `networkdata` is present in DHCP mode, check if a custom bootstrap template is injecting it. Remove any network-config injection from your bootstrap provider configuration.

---

## Turtles / Rancher Import Issues

### `cacerts` Empty Error

**Symptoms:**
- The CAPI Cluster has the label `cluster-api.cattle.io/rancher-auto-import=true` but the cluster never appears in Rancher.
- Turtles controller logs show: `ca-certs setting value is empty`.
- The `clusters.provisioning.cattle.io` object is created but the management cluster object is not.

**Cause:**
When Rancher is deployed with `tls=external` (TLS is terminated by an external load balancer or reverse proxy like Traefik), Rancher does not set the `cacerts` setting by default. Turtles in strict TLS mode (`agent-tls-mode=true`) requires `cacerts` to be non-empty to validate the CA chain for the cattle-cluster-agent.

**Fix:**
1. Set the `cacerts` setting on Rancher to the CA certificate chain used by the TLS terminator:
   ```bash
   # On Rancher management cluster
   # Get the current setting
   kubectl get settings.management.cattle.io cacerts -o yaml

   # Replace with your CA chain (e.g., Let's Encrypt E7 intermediate + ISRG Root X1)
   kubectl replace -f - <<'EOF'
   apiVersion: management.cattle.io/v3
   kind: Setting
   metadata:
     name: cacerts
   value: |
     -----BEGIN CERTIFICATE-----
     <your intermediate CA cert>
     -----END CERTIFICATE-----
     -----BEGIN CERTIFICATE-----
     <your root CA cert>
     -----END CERTIFICATE-----
   EOF
   ```
2. After changing `cacerts`, restart the Turtles controller:
   ```bash
   kubectl rollout restart deploy/rancher-turtles-controller-manager -n cattle-turtles-system
   ```
3. Verify the setting took effect:
   ```bash
   kubectl get settings.management.cattle.io cacerts -o jsonpath='{.value}' | head -2
   ```

---

### Cluster Stuck in "Waiting for Agent"

**Symptoms:**
- The cluster appears in Rancher but shows status "Waiting for agent to connect".
- The `cattle-cluster-agent` deployment in the workload cluster is in CrashLoopBackOff or not running.

**Cause:**
The cattle-cluster-agent running on the workload cluster cannot connect back to the Rancher server. Common causes:
- DNS resolution failure: the workload cluster cannot resolve the Rancher hostname.
- Certificate mismatch: the agent does not trust the Rancher server certificate.
- Network connectivity: firewall rules blocking HTTPS from the workload cluster to Rancher.

**Fix:**
1. Check the cattle-cluster-agent logs on the workload cluster:
   ```bash
   # On workload cluster
   kubectl logs deploy/cattle-cluster-agent -n cattle-system -f
   ```
2. Verify DNS resolution from within the workload cluster:
   ```bash
   # On workload cluster
   kubectl run -it --rm dnstest --image=busybox --restart=Never -- nslookup rancher.example.com
   ```
3. Check the `serverca` ConfigMap in `cattle-system`:
   ```bash
   # On workload cluster
   kubectl get configmap serverca -n cattle-system -o yaml
   ```
   This should contain the CA certificate chain that the agent uses to verify Rancher's TLS certificate.
4. If the CA is wrong, update the `serverca` ConfigMap with the correct chain and restart the agent:
   ```bash
   # On workload cluster
   kubectl rollout restart deploy/cattle-cluster-agent -n cattle-system
   ```
5. Test HTTPS connectivity from a pod in the workload cluster:
   ```bash
   kubectl run -it --rm curltest --image=curlimages/curl --restart=Never -- \
     curl -vk https://rancher.example.com/healthz
   ```

---

### Auto-Import Not Triggering

**Symptoms:**
- The CAPI Cluster object exists and is healthy, but Rancher never creates a management cluster entry for it.
- No `clusters.provisioning.cattle.io` object is created.

**Cause:**
Turtles auto-import requires a specific label on the CAPI Cluster object. Without it, Turtles ignores the cluster.

**Fix:**
1. Verify the label is present:
   ```bash
   kubectl get cluster <name> -n <ns> -o jsonpath='{.metadata.labels.cluster-api\.cattle\.io/rancher-auto-import}'
   ```
2. If missing, add it:
   ```bash
   kubectl label cluster <name> -n <ns> cluster-api.cattle.io/rancher-auto-import=true
   ```
3. Verify Turtles is running and watching the correct namespace:
   ```bash
   kubectl get deploy rancher-turtles-controller-manager -n cattle-turtles-system
   kubectl logs deploy/rancher-turtles-controller-manager -n cattle-turtles-system -f
   ```
4. If using ClusterClass, ensure the label is included in the Cluster topology metadata, not just the template. CAPHV's ClusterClass generator includes this label by default when Rancher integration is enabled.

---

### After Changing `cacerts`

**Symptoms:**
- You updated the `cacerts` setting but existing imports are still failing with the old CA.
- New imports work but previously failed imports remain stuck.

**Cause:**
The Turtles controller caches the CA setting. After changing `cacerts`, a controller restart is required for the new value to take effect.

**Fix:**
1. Restart the Turtles controller:
   ```bash
   kubectl rollout restart deploy/rancher-turtles-controller-manager -n cattle-turtles-system
   ```
2. For clusters that were already stuck, delete and re-create the CAPI import resources:
   ```bash
   # Delete the stuck provisioning cluster (Turtles will recreate it)
   kubectl delete clusters.provisioning.cattle.io <name> -n default
   ```
3. Wait for Turtles to re-trigger the import. Monitor the Turtles logs:
   ```bash
   kubectl logs deploy/rancher-turtles-controller-manager -n cattle-turtles-system -f
   ```

---

## VM Creation Issues

### VM Stuck in Scheduling

**Symptoms:**
- The VM object exists on Harvester but never transitions past `Scheduling` phase.
- `kubectl get vm <name> -n <ns>` shows the VM in Scheduling state.
- No VMI (VirtualMachineInstance) is created.

**Cause:**
- Insufficient resources on Harvester nodes (CPU, memory).
- The VM image referenced in the volume does not exist.
- Node affinity rules prevent scheduling on any available node.

**Fix:**
1. Check Harvester node resources:
   ```bash
   # On Harvester
   kubectl get nodes -o custom-columns=NAME:.metadata.name,CPU:.status.allocatable.cpu,MEM:.status.allocatable.memory
   ```
2. Check VM events:
   ```bash
   # On Harvester
   kubectl describe vm <name> -n <ns>
   ```
   Look for scheduling-related events at the bottom.
3. Verify the VM image exists:
   ```bash
   # On Harvester
   kubectl get virtualmachineimages -n <ns>
   ```
4. If using nodeAffinity in HarvesterMachineSpec, verify the labels match nodes on Harvester:
   ```bash
   # On Harvester
   kubectl get nodes --show-labels
   ```

---

### VM Stuck in Starting

**Symptoms:**
- The VM shows as `Starting` but never transitions to `Running`.
- The VMI exists but shows `Scheduling` or `Pending`.
- Events mention missing secrets or SSH keypairs.

**Cause:**
- The cloud-init secret referenced by the VM does not exist.
- The SSH keypair referenced in the HarvesterMachine does not exist on Harvester.
- PVCs backing the VM disks are not bound.

**Fix:**
1. Check the VMI events:
   ```bash
   # On Harvester
   kubectl describe vmi <name> -n <ns>
   ```
2. Verify the cloud-init secret exists:
   ```bash
   # On Harvester
   kubectl get secret <machine-name>-cloud-init -n <target-ns>
   ```
3. Verify the SSH keypair exists on Harvester:
   ```bash
   # On Harvester
   kubectl get keypairs.harvesterhci.io -n <ns>
   ```
4. Check PVC status:
   ```bash
   # On Harvester
   kubectl get pvc -n <target-ns> -l harvesterhci.io/creator=caphv
   ```
   All PVCs should be in `Bound` state.

---

### PVC Not Created

**Symptoms:**
- The VM is not created because the controller failed to create PVCs.
- Controller logs show: `failed to create PVC` or `invalid image name`.
- HarvesterMachine condition `VMProvisioningReady` is `False` with reason `VMProvisioningFailed`.

**Cause:**
- The `imageName` in the Volume spec does not match the format `namespace/name`. Image names with underscores (e.g., `default/sles15-sp7-minimal-vm.x86_64-cloud-qu2`) are valid as of v0.2.0 (the `CheckNamespacedName` regex was fixed to allow underscores).
- The Longhorn storage class for the image does not exist on Harvester.
- The volume spec references a `storageClass` that does not exist.

**Fix:**
1. Verify the image exists on Harvester and note its exact name:
   ```bash
   # On Harvester
   kubectl get virtualmachineimages -A
   ```
2. Verify the storage class exists:
   ```bash
   # On Harvester
   kubectl get storageclass
   ```
   Since v0.5.2 the storage class of an image volume is read from
`VirtualMachineImage.status.storageClassName` (recent Harvester names them
`lh-<uuid>`); the legacy `longhorn-<imageName>` convention is only a fallback
for images whose status is not populated yet.
3. Fix the `imageName` in the HarvesterMachineTemplate to use `namespace/name` format:
   ```yaml
   volumes:
     - volumeType: image
       imageName: default/sles15-sp7-minimal-vm
       volumeSize: 40Gi
       bootOrder: 1
   ```
4. Check controller logs for the exact error:
   ```bash
   kubectl logs deploy/caphv-controller-manager -n caphv-system | grep -i "pvc\|volume\|image"
   ```

---

### Boot Order Wrong

**Symptoms:**
- The VM boots from the wrong disk (e.g., a blank data disk instead of the OS image disk).
- The VM enters a boot loop or PXE boot screen.

**Cause:**
The `bootOrder` field in the Volume spec determines which disk KubeVirt tries to boot from first. Lower numbers boot first. If bootOrder is not set (0), disks boot in the order they appear in the spec.

**Fix:**
1. Check the current boot order on the VM:
   ```bash
   # On Harvester
   kubectl get vm <name> -n <ns> -o jsonpath='{.spec.template.spec.domain.devices.disks}' | python3 -m json.tool
   ```
2. Set explicit bootOrder in the HarvesterMachineTemplate:
   ```yaml
   volumes:
     - volumeType: image
       imageName: default/sles15-sp7-minimal-vm
       volumeSize: 40Gi
       bootOrder: 1    # Boot from this disk first
     - volumeType: storageClass
       storageClass: longhorn
       volumeSize: 10Gi
       bootOrder: 2    # Data disk, do not boot from this
   ```
3. After updating the template, existing machines are not affected. Delete and recreate machines to apply the new boot order, or scale down and back up for MachineDeployment-managed workers.

---

## Machine Not Becoming Ready

### Machine stuck in Provisioned with a Ready node (pre-v0.10.0)

**Symptoms:** the workload node is `Ready` but its `spec.providerID` is empty,
the Machine stays in `Provisioned` phase forever, and the controller logs stay
silent about the machine.

**Cause:** before v0.10.0, a node registering after the machine's last
event-driven reconcile was never initialized (no requeue). Clusters with a
MachineHealthCheck were shielded (its probes keep generating events); clusters
without one exposed the gap.

**Fix:** upgrade to v0.10.0+. On older versions, force one reconcile:
`kubectl annotate harvestermachine <name> -n <ns> debug/kick=$(date +%s) --overwrite`.

### ProviderID Not Set

**Symptoms:**
- The Machine object in CAPI stays in `Provisioned` state but never becomes `Running`.
- The node exists in the workload cluster but `kubectl get nodes -o wide` shows an empty `PROVIDER-ID`.
- The cloud-provider-harvester pod is in CrashLoopBackOff or Pending on the workload cluster.

**Cause:**
The cloud-provider-harvester typically sets the providerID on each node. However, it cannot schedule or function until CNI (Calico/Flannel) is running, and CNI cannot run until the `node.cloudprovider.kubernetes.io/uninitialized` taint is removed. This is a chicken-and-egg problem.

CAPHV v0.2.0+ solves this by setting the providerID and removing the taint directly from the management cluster via `InitializeWorkloadNode()`.

**Fix:**
1. Upgrade CAPHV to v0.2.0 or later. The controller automatically:
   - Sets `spec.providerID` on the workload node via a Kubernetes API patch.
   - Removes the `node.cloudprovider.kubernetes.io/uninitialized` taint.
2. If you cannot upgrade, manually fix the node:
   ```bash
   # On workload cluster
   # Set providerID (use the Harvester VM's name as the ID)
   kubectl patch node <node-name> --type=merge -p '{"spec":{"providerID":"harvester://<vm-name>"}}'

   # Remove the taint
   kubectl taint nodes <node-name> node.cloudprovider.kubernetes.io/uninitialized-
   ```
3. Verify the cloud-provider-harvester deployment on the workload cluster has the correct bootstrap configuration:
   - `hostNetwork: true` and `dnsPolicy: ClusterFirstWithHostNet` (CNI not ready at boot).
   - Toleration for `node.cloudprovider.kubernetes.io/uninitialized`.
   - `replicas: 1` (hostNetwork prevents port binding conflicts with multiple replicas).

---

### Node Has Uninitialized Taint

**Symptoms:**
- The workload cluster node exists but no pods (including CNI) schedule on it.
- `kubectl describe node <name>` shows `Taints: node.cloudprovider.kubernetes.io/uninitialized:NoSchedule`.
- CNI pods are Pending.

**Cause:**
Same chicken-and-egg as above. When `--cloud-provider=external` is set, kubelet adds this taint at startup. It expects an external cloud provider to remove it. If the cloud provider cannot run (because CNI is not ready, which requires this taint to be removed), nothing progresses.

**Fix:**
CAPHV v0.2.0+ handles this automatically. See the ProviderID Not Set section above.

Manual fix:
```bash
# On workload cluster
kubectl taint nodes <node-name> node.cloudprovider.kubernetes.io/uninitialized-
```

---

### IP Addresses Not Found on VMI

**Symptoms:**
- The VM is Running on Harvester.
- The HarvesterMachine object stays in `Provisioning` state with condition `VMRunning=True` but `MachineCreated=False`.
- Controller logs show: `waiting for VM IP addresses to be reported`.

**Cause:**
The CAPHV controller reads IP addresses from the VMI's `status.interfaces` field. These are populated by the qemu-guest-agent running inside the VM. If the guest agent is not installed, not running, or not yet started, no IPs are reported.

**Fix:**
1. Check the VMI status on Harvester:
   ```bash
   # On Harvester
   kubectl get vmi <name> -n <ns> -o jsonpath='{.status.interfaces}' | python3 -m json.tool
   ```
2. If interfaces is empty, SSH into the VM and check the guest agent:
   ```bash
   systemctl status qemu-guest-agent
   ```
3. If the agent is not installed, CAPHV's cloud-init userdata installs it automatically (`packages: [qemu-guest-agent]`). If it was not installed:
   ```bash
   # On the VM
   zypper install -y qemu-guest-agent
   systemctl enable --now qemu-guest-agent
   ```
4. Wait 30-60 seconds after the agent starts for KubeVirt to poll and update the VMI status. The controller reconciles every 30 seconds.

---

## etcd Issues

### Stale etcd Member After Control Plane Machine Deletion

**Symptoms:**
- A control plane machine was deleted (e.g., by MachineHealthCheck) and a new one was created, but the etcd cluster still has a member entry for the old node.
- `etcdctl member list` shows a member that is "unstarted" or has no name.
- The etcd cluster reports unhealthy because the stale member cannot be reached.
- New control plane nodes fail to join etcd because the cluster has an unresolvable member.

**Cause:**
When a control plane machine is deleted, the corresponding etcd member should be removed. CAPHV performs automatic etcd member cleanup during the machine deletion reconcile via `RemoveEtcdMember()`. If the cleanup fails (e.g., workload cluster unreachable, no healthy etcd pod available), the stale member persists.

RKE2's own control plane controller also handles etcd member removal in most cases. The CAPHV cleanup is a safety net.

**Fix:**
1. Identify the stale member:
   ```bash
   # On workload cluster (from any healthy CP node)
   ETCDCTL_API=3 etcdctl \
     --cacert /var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
     --cert /var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
     --key /var/lib/rancher/rke2/server/tls/etcd/server-client.key \
     --endpoints https://127.0.0.1:2379 \
     member list -w table
   ```
2. Remove the stale member by its hex ID:
   ```bash
   # On workload cluster
   ETCDCTL_API=3 etcdctl \
     --cacert /var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
     --cert /var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
     --key /var/lib/rancher/rke2/server/tls/etcd/server-client.key \
     --endpoints https://127.0.0.1:2379 \
     member remove <hex-member-id>
   ```
3. Alternatively, use kubectl exec from the management cluster (this is what CAPHV does internally):
   ```bash
   # Find a healthy etcd pod
   kubectl get pods -n kube-system -l component=etcd,tier=control-plane --kubeconfig <workload-kubeconfig>

   # List members
   kubectl exec -n kube-system etcd-<healthy-node> --kubeconfig <workload-kubeconfig> -- \
     etcdctl --cacert /var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
       --cert /var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
       --key /var/lib/rancher/rke2/server/tls/etcd/server-client.key \
       --endpoints https://127.0.0.1:2379 \
       member list -w json
   ```
4. After removing the stale member, the replacement control plane node should be able to join the etcd cluster. If it is still stuck, restart the rke2-server service on the replacement node:
   ```bash
   # On the replacement CP node
   systemctl restart rke2-server
   ```

---

## Deletion Issues

### Identity Secret Stuck in Terminating

**Symptoms:**
- `kubectl get secret <identity-secret>` shows a deletion timestamp and the Secret
  never goes away.
- Its finalizers include `harvester.infrastructure.cluster.x-k8s.io/identity-<uid>`.

**Cause:**
Each HarvesterCluster puts this finalizer on its identity Secret, so that the
Secret survives until the cluster resources in Harvester are deleted, and removes
it at the end of its own deletion. A finalizer remains when its HarvesterCluster
went away without that deletion running: the HarvesterCluster was moved with
`clusterctl move` (the source objects are deleted while paused), or its
`spec.identitySecret` was changed to another Secret.

**Fix:**
Check that no HarvesterCluster with that UID exists any more
(`kubectl get harvesterclusters -A -o custom-columns=NAME:.metadata.name,UID:.metadata.uid`),
then remove the finalizer:

```bash
kubectl patch secret <identity-secret> -n <namespace> --type json \
  -p '[{"op": "remove", "path": "/metadata/finalizers/<index>"}]'
```

A HarvesterCluster being deleted also waits for the HarvesterMachines of its
cluster: its log shows `Waiting for the HarvesterMachines of the cluster to be
deleted` until they are gone.

---

## Useful Commands for Debugging

### Management Cluster (where CAPHV runs)

List all CAPI and CAPHV resources:
```bash
kubectl get cluster,machine,harvestermachine,harvestermachinetemplate -A
```

Watch CAPHV controller logs:
```bash
kubectl logs deploy/caphv-controller-manager -n caphv-system -f
```

Increase controller log verbosity (add to the deployment args):
```bash
kubectl edit deploy/caphv-controller-manager -n caphv-system
# Add --v=5 to the container args
```

Check HarvesterMachine conditions:
```bash
kubectl describe harvestermachine <name> -n <ns>
```

List all CAPI clusters with status:
```bash
kubectl get cluster -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PHASE:.status.phase,READY:.status.conditions[0].status
```

Check MachineHealthCheck status:
```bash
kubectl get machinehealthcheck -A
kubectl describe machinehealthcheck <name> -n <ns>
```

### Harvester Cluster

List all IPPools and their availability:
```bash
kubectl get ippools.loadbalancer.harvesterhci.io -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,AVAILABLE:.status.available
```

Check IP pool allocations (detailed):
```bash
kubectl get ippools.loadbalancer.harvesterhci.io <name> -n <ns> -o jsonpath='{.status.allocated}' | python3 -m json.tool
```

List VMs with their status:
```bash
kubectl get vm -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,STATUS:.status.printableStatus
```

List VMIs with IP addresses:
```bash
kubectl get vmi -A -o custom-columns=NS:.metadata.namespace,NAME:.metadata.name,PHASE:.status.phase,IPS:.status.interfaces[*].ipAddress
```

Check PVCs created by CAPHV:
```bash
kubectl get pvc -A -l harvesterhci.io/creator=caphv
```

Check cloud-init secret contents:
```bash
# Userdata
kubectl get secret <machine>-cloud-init -n <ns> -o jsonpath='{.data.userdata}' | base64 -d

# Networkdata (only present in static IP mode)
kubectl get secret <machine>-cloud-init -n <ns> -o jsonpath='{.data.networkdata}' | base64 -d
```

### Workload Cluster (the cluster created by CAPI)

Check cloud-init status:
```bash
cloud-init status --long
```

Check cloud-init logs:
```bash
cat /var/log/cloud-init.log
cat /var/log/cloud-init-output.log
```

Check RKE2 service status:
```bash
journalctl -u rke2-server -f    # Control plane nodes
journalctl -u rke2-agent -f     # Worker nodes
```

Check node status and providerID:
```bash
kubectl get nodes -o custom-columns=NAME:.metadata.name,STATUS:.status.conditions[-1].type,PROVIDER-ID:.spec.providerID,TAINTS:.spec.taints[*].key
```

Check etcd cluster health:
```bash
ETCDCTL_API=3 etcdctl \
  --cacert /var/lib/rancher/rke2/server/tls/etcd/server-ca.crt \
  --cert /var/lib/rancher/rke2/server/tls/etcd/server-client.crt \
  --key /var/lib/rancher/rke2/server/tls/etcd/server-client.key \
  --endpoints https://127.0.0.1:2379 \
  endpoint health
```

Check cattle-cluster-agent (Rancher import):
```bash
kubectl logs deploy/cattle-cluster-agent -n cattle-system -f
kubectl get configmap serverca -n cattle-system -o yaml
```

Check network configuration on a VM:
```bash
ip addr show
ip route show
cat /etc/resolv.conf
wicked ifstatus --verbose eth0    # SLES only
```
