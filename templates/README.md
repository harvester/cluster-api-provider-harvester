# Cluster Templates

This directory contains YAML templates for creating clusters using the Harvester Cluster API provider. Each template can be used to generate cluster manifests using `clusterctl generate yaml` that can be applied to a CAPI management cluster.

## Templates

### cluster-template-kubeadm.yaml
Creates a cluster using kubeadm bootstrap provider.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-kubeadm.yaml \
  CLUSTER_NAME=my-cluster \
  KUBERNETES_VERSION=v1.32.1 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=ubuntu-20.04 \
  SSH_KEYPAIR=default/my-keypair \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

### cluster-template-rke2.yaml
Creates a cluster using RKE2 bootstrap provider.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-rke2.yaml \
  CLUSTER_NAME=my-rke2-cluster \
  KUBERNETES_VERSION=v1.32.1+rke2r1 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=ubuntu-20.04 \
  SSH_KEYPAIR=default/my-keypair \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

### cluster-template-rke2-dhcp.yaml
Creates an RKE2 cluster with DHCP-based load balancer configuration.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-rke2-dhcp.yaml \
  CLUSTER_NAME=my-rke2-dhcp-cluster \
  KUBERNETES_VERSION=v1.32.1+rke2r1 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=ubuntu-20.04 \
  SSH_KEYPAIR=default/my-keypair \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

### cluster-template-rke2-generateCPI.yaml
Creates an RKE2 cluster with cloud provider interface generation.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-rke2-generateCPI.yaml \
  CLUSTER_NAME=my-rke2-cpi-cluster \
  KUBERNETES_VERSION=v1.32.1+rke2r1 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=ubuntu-20.04 \
  SSH_KEYPAIR=default/my-keypair \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

### cluster-template-talos.yaml
Creates a cluster using Talos Linux with Cilium CNI and Harvester cloud provider.

**Usage:**
```bash
# For DHCP mode:
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-talos.yaml \
  CLUSTER_NAME=my-talos-cluster \
  KUBERNETES_VERSION=v1.32.1 \
  TALOS_VERSION=v1.5.0 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=talos-v1.5.0 \
  SSH_KEYPAIR=default/my-keypair \
  IPAM_TYPE=dhcp \
  IP_POOL_CONFIG="" \
  > cluster-manifest.yaml

# For IP pool mode:
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/cluster-template-talos.yaml \
  CLUSTER_NAME=my-talos-cluster \
  KUBERNETES_VERSION=v1.32.1 \
  TALOS_VERSION=v1.5.0 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=talos-v1.5.0 \
  SSH_KEYPAIR=default/my-keypair \
  IPAM_TYPE=pool \
  IP_POOL_NAME=my-ip-pool \
  IP_POOL_CONFIG="
    ipPoolRef: \${IP_POOL_NAME}
    ipPool:
      subnet: 192.168.1.0/24
      gateway: 192.168.1.1
      vmNetwork: default/vm-network
      rangeStart: 192.168.1.100
      rangeEnd: 192.168.1.200" \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

## ClusterClass Templates

### clusterclass/rke2/clusterclass-harvester-rke2-example.yaml
Defines a ClusterClass for RKE2 clusters that can be reused.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/clusterclass/rke2/clusterclass-harvester-rke2-example.yaml \
  CLUSTER_CLASS_NAME=harvester-rke2 \
  > clusterclass.yaml

kubectl apply -f clusterclass.yaml
```

### clusterclass/rke2/cluster-template-rke2-clusterclass-generateCPI.yaml
Creates a cluster using the RKE2 ClusterClass with CPI generation.

**Usage:**
```bash
clusterctl generate yaml \
  --from https://github.com/harvester/cluster-api-provider-harvester/blob/main/templates/clusterclass/rke2/cluster-template-rke2-clusterclass-generateCPI.yaml \
  CLUSTER_NAME=my-clusterclass-cluster \
  CLUSTER_CLASS_NAME=harvester-rke2 \
  KUBERNETES_VERSION=v1.32.1+rke2r1 \
  HARVESTER_ENDPOINT=https://harvester.example.com \
  HARVESTER_KUBECONFIG_B64=$(base64 -w 0 -i /path/to/harvester-kubeconfig.yaml) \
  TARGET_HARVESTER_NAMESPACE=default \
  VM_NETWORK=default/vm-network \
  VM_IMAGE_NAME=ubuntu-20.04 \
  SSH_KEYPAIR=default/my-keypair \
  > cluster-manifest.yaml

kubectl apply -f cluster-manifest.yaml
```

## Common Environment Variables

- `CLUSTER_NAME`: Name of the cluster
- `KUBERNETES_VERSION`: Kubernetes version to use
- `HARVESTER_ENDPOINT`: Harvester cluster endpoint URL
- `HARVESTER_KUBECONFIG_B64`: Base64-encoded kubeconfig for Harvester cluster
- `TARGET_HARVESTER_NAMESPACE`: Namespace in Harvester where VMs will be created
- `VM_NETWORK`: Network configuration for VMs (format: namespace/network-name)
- `VM_IMAGE_NAME`: VM image to use for cluster nodes
- `SSH_KEYPAIR`: SSH key pair for VM access (format: namespace/keypair-name)
- `CONTROL_PLANE_MACHINE_COUNT`: Number of control plane nodes (default: 3)
- `WORKER_MACHINE_COUNT`: Number of worker nodes (default: 2)
- `IPAM_TYPE`: IP address management type (dhcp or pool)
- `IP_POOL_CONFIG`: IP pool configuration (for non-DHCP mode)

## Prerequisites

Before using these templates, ensure you have:

1. A running CAPI management cluster
2. Harvester Cluster API provider installed on the management cluster
3. Appropriate bootstrap providers (kubeadm, RKE2, or Talos)
4. A Harvester cluster with necessary resources (images, networks, SSH keys)
5. Valid kubeconfig for the Harvester cluster
6. `clusterctl` CLI tool installed

## Applying the Manifests

After generating the manifest using `clusterctl generate yaml`, apply it to your CAPI management cluster:

```bash
kubectl apply -f cluster-manifest.yaml
```

Monitor the cluster creation:

```bash
kubectl get clusters
kubectl get harvesterclusters
kubectl get machines
```