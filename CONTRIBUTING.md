# Contributing to CAPHV

Thank you for your interest in contributing to CAPHV (Cluster API Provider Harvester)!

## Development Setup

### Prerequisites

- Go 1.26+ (see `go.mod` for exact version)
- `kubectl` configured with access to a management cluster
- Access to a Harvester HCI cluster (v1.7+)
- [cert-manager](https://cert-manager.io/) installed on the management cluster (for webhooks)
- Docker or Podman (for building container images)

### Clone and Build

```bash
git clone https://github.com/harvester/cluster-api-provider-harvester.git
cd cluster-api-provider-harvester

# Build the controller binary
make build

# Build the container image
make docker-build IMG=ghcr.io/harvester/cluster-api-provider-harvester:dev

# Build the CLI generator
go build -o bin/caphv-generate ./cmd/caphv-generate/
```

## Running Tests

### Unit Tests

```bash
make test
```

### End-to-End Integration Tests

E2E tests run against a live Harvester + Rancher Manager + CAPI environment. They require an existing cluster with control plane and worker nodes.

```bash
# Configure your environment
export CAPHV_RANCHER_SSH="rancher@<your-rancher-manager-ip>"
export CAPHV_HARVESTER_SSH="rancher@<your-harvester-ip>"

# Run all tests
./test/e2e/run-e2e.sh

# Run a specific test suite
./test/e2e/run-e2e.sh webhook
./test/e2e/run-e2e.sh scale
./test/e2e/run-e2e.sh multidisk
./test/e2e/run-e2e.sh remediation
```

#### E2E Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CAPHV_RANCHER_SSH` | `rancher@<rancher-manager-ip>` | SSH target for Rancher Manager |
| `CAPHV_HARVESTER_SSH` | `rancher@<harvester-ip>` | SSH target for Harvester node |
| `CAPHV_KUBECTL_RANCHER` | `sudo .../kubectl --kubeconfig ...` | kubectl command on Rancher |
| `CAPHV_KUBECTL_HARVESTER` | Same as Rancher | kubectl command on Harvester |
| `CAPHV_NAMESPACE` | `capi-test` | Namespace for test resources |
| `CAPHV_CLUSTER_NAME` | `capi-test` | CAPI cluster name |
| `CAPHV_WORKER_MD` | `<cluster>-workers` | MachineDeployment name |
| `CAPHV_TIMEOUT_VM_RUNNING` | `600` | Timeout (s) for VM Running |
| `CAPHV_TIMEOUT_NODE_READY` | `600` | Timeout (s) for node Ready |
| `CAPHV_TIMEOUT_VM_DELETED` | `300` | Timeout (s) for VM deletion |
| `CAPHV_TIMEOUT_REMEDIATION` | `900` | Timeout (s) for remediation cycle |

## Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Make your changes, ensuring all tests pass
3. Write clear commit messages describing **why** the change was made
4. Open a PR with a description of the change and any relevant issue references
5. Ensure CI passes (unit tests, lint)

### Commit Conventions

- Use imperative mood: "Add feature" not "Added feature"
- Reference issues when applicable: "Fix memory.guest bug (#139)"
- Keep commits focused: one logical change per commit

## Code Style

This project uses [golangci-lint](https://golangci-lint.run/) with the configuration in `.golangci.yml`.

```bash
# Run linter
golangci-lint run ./...
```

Key conventions:
- Follow standard Go project layout
- Use `controller-runtime` patterns for reconcilers
- Admission webhooks implement the typed `admission.Validator[T]` (not the deprecated `webhook.Validator`)
- API types live in `api/v1beta1/` (hub and stored version; `api/v1alpha1/` is deprecated and served through conversion)
- Controller logic lives in `internal/controller/`
- Utility functions live in `util/`

## Reporting Issues

- Bug reports: include Harvester version, CAPHV version, controller logs, and steps to reproduce
- Feature requests: describe the use case and proposed approach
