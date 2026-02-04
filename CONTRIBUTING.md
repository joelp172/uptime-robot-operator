# Contributing to Uptime Robot Operator

Thank you for your interest in contributing. This guide covers how to set up a development environment and run tests locally before submitting a pull request.

## Prerequisites

- Go 1.23+
- Docker
- kubectl
- [Kind](https://kind.sigs.k8s.io/) or [minikube](https://minikube.sigs.k8s.io/)
- make

## Development Setup

1. Clone the repository:

   ```bash
   git clone https://github.com/joelp172/uptime-robot-operator.git
   cd uptime-robot-operator
   ```

2. Install dependencies:

   ```bash
   go mod download
   ```

3. Install pre-commit hooks (optional but recommended):

   ```bash
   pip install pre-commit
   pre-commit install
   ```

## Running Tests

### Unit Tests

Run unit tests with:

```bash
make test
```

This runs all unit tests using the controller-runtime envtest framework (in-memory Kubernetes API server).

### Linting

Run the linter:

```bash
make lint
```

Fix auto-fixable lint issues:

```bash
make lint-fix
```

### E2E Tests (Local)

E2E tests require a Kubernetes cluster. You can use Kind or minikube.

#### Option 1: Quick Setup with Kind

```bash
# Create a Kind cluster
kind create cluster --name e2e-test

# Build and load the operator image
make docker-build IMG=uptime-robot-operator:dev
kind load docker-image uptime-robot-operator:dev --name e2e-test

# Install CRDs and deploy the operator
make install
make deploy IMG=uptime-robot-operator:dev

# Verify the operator is running
kubectl wait --for=condition=Available deployment/uptime-robot-controller-manager \
  -n uptime-robot-system --timeout=2m
```

#### Option 2: Use the Dev Cluster Script

```bash
# Create a development cluster (uses minikube by default)
make dev-cluster

# Or use Kind
make dev-cluster-kind
```

#### Running Basic E2E Tests

Basic tests verify the operator starts and serves metrics (no UptimeRobot API key needed):

```bash
# Run basic e2e tests
make test-e2e
```

#### Running Full E2E Tests with Real API

Full e2e tests create actual monitors in UptimeRobot. You'll need an API key from a **test account** (not production):

```bash
# Set your test API key
export UPTIME_ROBOT_API_KEY=your-test-api-key

# Run full e2e tests
make test-e2e-real
```

**Warning**: Full e2e tests create and delete real monitors in UptimeRobot. Use a dedicated test account.

#### Cleanup

```bash
# Delete test resources
kubectl delete maintenancewindows,monitors,contacts,accounts --all

# Undeploy the operator
make undeploy
make uninstall

# Delete the Kind cluster
kind delete cluster --name e2e-test
```

## Before Submitting a PR

Please ensure all the following pass locally:

### Checklist

- [ ] **Unit tests pass**: `make test`
- [ ] **Linting passes**: `make lint`
- [ ] **Code is formatted**: `make fmt`
- [ ] **Generated files are up to date**: `make generate manifests`
- [ ] **E2E tests pass** (if you have an API key): `make test-e2e-real`

### Quick Validation Script

Run all checks at once:

```bash
make generate manifests fmt vet lint test
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```
feat(monitor): add support for DNS monitor type
fix(controller): handle API rate limiting
docs: update installation instructions
chore: update dependencies
```

**Release Triggers** (used by semantic-release):

| Type | Release | Version Bump |
|------|---------|--------------|
| `feat:` | Yes | Minor (1.x.0) |
| `fix:` | Yes | Patch (1.0.x) |
| `docs:`, `chore:`, `ci:`, `refactor:`, `test:` | No | - |

To trigger a release, your PR title must start with `feat:` or `fix:`.

## Pull Request Guidelines

1. **Create a draft PR** while work is in progress (e2e tests are skipped for drafts)
2. **Mark as ready for review** when tests pass locally
3. **Ensure CI passes** before requesting review
4. **Keep PRs focused** - one feature or fix per PR
5. **Update documentation** if adding new features

## Running the Operator Locally (Outside Cluster)

For rapid development, you can run the operator outside the cluster:

```bash
# Install CRDs
make install

# Run the operator locally (uses your kubeconfig)
make run
```

This is useful for debugging as you get immediate log output.

## Project Structure

```
.
├── api/v1alpha1/       # CRD type definitions
├── cmd/                # Main entrypoint
├── config/             # Kustomize manifests
│   ├── crd/            # CRD definitions
│   ├── default/        # Default deployment
│   ├── manager/        # Controller manager
│   └── samples/        # Example resources
├── internal/
│   ├── controller/     # Reconciliation logic
│   └── uptimerobot/    # UptimeRobot API client
├── test/
│   └── e2e/            # End-to-end tests
└── hack/               # Development scripts
```

## Getting Help

- Open an [issue](https://github.com/joelp172/uptime-robot-operator/issues) for bugs or feature requests
- Check existing issues before creating a new one
