# Development

Set up a development environment and run tests.

## Prerequisites

- Go 1.23+
- Docker
- kubectl
- [Kind](https://kind.sigs.k8s.io/)
- make

## Setup

Clone and install dependencies:

```bash
git clone https://github.com/joelp172/uptime-robot-operator.git
cd uptime-robot-operator
go mod download
```

## Running Tests

### Unit Tests

```bash
make test
```

### Linting

```bash
make lint
```

Fix auto-fixable issues:

```bash
make lint-fix
```

### E2E Tests

E2E tests require a Kind cluster named `kind`.
The local test flow installs pinned cert-manager (`v1.16.2` by default) for webhook TLS.
Override the version with `CERT_MANAGER_VERSION=<version>`.

#### Basic Tests (No API Key Required)

Tests operator deployment and metrics only:

```bash
make dev-cluster
make test-e2e

# Optional manual install/update of the pinned cert-manager release
make cert-manager-install
```

#### Full Tests (Requires API Key)

Creates real monitors in UptimeRobot. Use a test account:

```bash
export UPTIME_ROBOT_API_KEY=your-test-key
make test-e2e-real
```

Enable debug logging:

```bash
E2E_DEBUG=1 make test-e2e-real
```

#### Cleanup

```bash
kubectl delete maintenancewindows,monitors,contacts,accounts --all
make dev-cluster-delete
```

## Local Development

### Run Operator Locally

Run outside the cluster for faster iteration:

```bash
make install  # Install CRDs
make run      # Run operator locally
```

### Run in Cluster

```bash
make dev-cluster                                    # Create Kind cluster
make docker-build IMG=operator:dev                  # Build image
kind load docker-image operator:dev                 # Load into Kind
make deploy IMG=operator:dev                        # Deploy
```

## Code Generation

After modifying CRD types:

```bash
make manifests  # Generate CRDs
make generate   # Generate code
```

## Before Committing

Run all checks:

```bash
make manifests generate fmt vet lint test
```

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

## Adding New Fields

When adding fields to CRDs:

1. Update API types in `api/v1alpha1/`
2. Add validation tags (`+kubebuilder:validation:...`)
3. Run `make manifests generate`
4. Update controller logic in `internal/controller/`
5. Add unit tests
6. Add e2e tests validating against real API
7. Update documentation

See `.cursor/rules/new-field-checklist.mdc` for complete checklist.

## Testing Against Mock API

To test against a mock UptimeRobot API:

```bash
export UPTIME_ROBOT_API=http://localhost:8080
make test-e2e-real
```

## Troubleshooting

### E2E Tests Hang

Enable debug logging to see API calls:

```bash
E2E_DEBUG=1 make test-e2e-real
```

### Operator Not Starting

Check logs:

```bash
kubectl logs -n uptime-robot-system deployment/uptime-robot-controller-manager
```

### CRD Changes Not Applied

Reinstall CRDs:

```bash
make uninstall
make install
```

## Release Process

Releases are automated via semantic-release. Commit messages determine version bumps:

| Commit Type | Version Bump | Example |
|-------------|--------------|---------|
| `feat:` | Minor (1.x.0) | `feat(monitor): add DNS support` |
| `fix:` | Patch (1.0.x) | `fix(controller): handle rate limiting` |
| `docs:`, `chore:`, `ci:`, `refactor:`, `test:` | None | `docs: update README` |

Breaking changes trigger major version bump:

```
feat(api)!: remove deprecated fields

BREAKING CHANGE: Removed `oldField` from Monitor spec
```

## Getting Help

- [GitHub Issues](https://github.com/joelp172/uptime-robot-operator/issues)
- [Discussions](https://github.com/joelp172/uptime-robot-operator/discussions)
