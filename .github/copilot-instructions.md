# Copilot Agent Instructions: Uptime Robot Operator

## Repository Overview

**Purpose**: Kubernetes operator that manages UptimeRobot monitors as Kubernetes Custom Resources. Enables declarative monitor configuration, automatic drift detection/correction, and GitOps workflows.

**Technology Stack**:
- Language: Go 1.24+ (go.mod: 1.24.1)
- Framework: Kubebuilder v4 (controller-runtime v0.20.3)
- Kubernetes: v0.32.3
- Testing: Ginkgo/Gomega
- Size: ~70 Go files, ~18,679 lines of code
- CRDs: Monitor, Account, Contact, MaintenanceWindow, MonitorGroup, SlackIntegration

**Key Features**: All monitor types (HTTPS, Keyword, Ping, Port, Heartbeat, DNS), drift detection, maintenance windows, alert contacts, adoption of existing monitors.

## Project Layout

```
/
├── api/v1alpha1/           # CRD type definitions (Monitor, Account, Contact, etc.)
├── cmd/main.go             # Operator entrypoint
├── config/                 # Kustomize manifests
│   ├── crd/bases/          # Generated CRD definitions (auto-synced to charts/*/crds/)
│   ├── default/            # Default deployment (includes webhooks, cert-manager)
│   ├── manager/            # Controller manager deployment config
│   ├── rbac/               # RBAC roles
│   ├── webhook/            # Webhook configurations
│   ├── certmanager/        # Certificate resources (requires cert-manager)
│   └── samples/            # Example Monitor/Account/Contact resources
├── internal/
│   ├── controller/         # Reconciliation logic for each CRD
│   └── uptimerobot/        # UptimeRobot API client
├── test/
│   ├── e2e/                # Ginkgo e2e tests (requires Kind cluster)
│   └── utils/              # Test utilities
├── charts/uptime-robot-operator/  # Helm chart (CRDs auto-synced from config/crd/bases/)
├── hack/                   # Development scripts (setup-dev-cluster.sh)
├── Makefile                # Build automation (manifests, generate, test, build, lint, deploy)
├── .golangci.yml           # Linter configuration
├── .pre-commit-config.yaml # Pre-commit hooks (gitleaks, go-mod-tidy, golangci-lint, manifests, generate)
└── Dockerfile              # Multi-stage build using distroless base
```

**Configuration Files**:
- `.golangci.yml`: Linter rules (copyloopvar, errcheck, staticcheck, govet, etc.)
- `.pre-commit-config.yaml`: Pre-commit hooks for code generation and linting
- `.gitleaksignore`: Gitleaks exceptions
- `.releaserc.yaml`: Semantic-release configuration

## Build and Validation

### Prerequisites
- Go 1.23+ (tested with 1.24.13)
- Docker (tested with 29.1.5)
- kubectl
- make
- Kind (for e2e tests)

### Essential Command Sequence

**CRITICAL**: Always run commands in this order for code changes:

```bash
make manifests generate fmt vet lint test
```

**Individual Commands**:

1. **Format code**: `make fmt` (instant, uses `go fmt ./...`)
2. **Vet code**: `make vet` (takes ~60 seconds, uses `go vet ./...`)
3. **Generate CRDs**: `make manifests` (takes ~5-10 seconds)
   - Generates CRDs in `config/crd/bases/`
   - Auto-syncs to `charts/uptime-robot-operator/crds/`
   - **ALWAYS run after modifying API types in `api/v1alpha1/`**
4. **Generate DeepCopy methods**: `make generate` (takes ~10 seconds)
   - **ALWAYS run after modifying API types in `api/v1alpha1/`**
5. **Lint**: `make lint` (takes ~90 seconds, downloads golangci-lint v1.63.4 on first run)
   - Fix auto-fixable issues: `make lint-fix`
6. **Test**: `make test` (takes ~270-300 seconds)
   - Downloads setup-envtest on first run
   - Sets up Kubernetes 1.32 test environment
   - Runs all non-e2e tests with coverage
7. **Build binary**: `make build` (takes ~60 seconds)
   - Runs manifests, generate, fmt, vet first
   - Outputs to `bin/manager`

### Docker Build

```bash
make docker-build IMG=operator:dev
```
- Uses multi-stage Dockerfile with `golang:1.24.1-alpine` and `gcr.io/distroless/static:nonroot`
- Build time: ~2-3 minutes on first run (with caching)

### E2E Testing

**Basic tests** (no API key required, tests deployment and metrics only):
```bash
make dev-cluster           # Creates Kind cluster with cert-manager
make test-e2e              # Skips real API tests (takes ~5-10 minutes)
```

**Full tests** (requires UPTIME_ROBOT_API_KEY):
```bash
export UPTIME_ROBOT_API_KEY=your-test-key
make test-e2e-real         # Tests real API interactions (takes ~20 minutes)
```

**All tests**:
```bash
make test-e2e-all          # Runs both basic and real API tests
```

**E2E Prerequisites**:
- Kind cluster must exist: `kind get clusters | grep kind` or custom name via `KIND_CLUSTER=name`
- Cert-manager is auto-installed (pinned to v1.16.2)
- Tests use Ginkgo label filters to skip/run real API tests

**Cleanup**:
```bash
kubectl delete maintenancewindows,monitors,contacts,accounts --all
make dev-cluster-delete
```

### Local Development

**Run operator locally** (fastest iteration):
```bash
make install               # Install CRDs to cluster
make run                   # Run operator outside cluster
```

**Run in cluster**:
```bash
make dev-cluster                                    # Creates Kind cluster
make docker-build IMG=operator:dev                  # Build image
kind load docker-image operator:dev                 # Load into Kind
make deploy IMG=operator:dev                        # Deploy operator
```

**After code changes**:
```bash
make docker-build IMG=operator:dev
kind load docker-image operator:dev --name kind
kubectl rollout restart -n uptime-robot-system deployment/uptime-robot-controller-manager
```

### CI/CD Workflows

**On PR/Push to main** (`.github/workflows/build.yml`):
1. **lint**: golangci-lint v2.0.2 (via GitHub Action)
2. **verify-crds**: Ensures `make manifests` was run (checks git diff)
3. **test**: Runs `make test`
4. **build**: Builds multi-platform Docker image (amd64, arm/v7, arm64/v8)

**Helm chart validation** (`.github/workflows/helm.yml`):
- Runs on changes to `charts/**`
- Tests: `helm lint`, `helm template`, Kind cluster install/upgrade/uninstall

**E2E tests** (`.github/workflows/e2e.yml`):
- Triggered by `/run-e2e` comment on PRs (requires write access)
- Builds operator, deploys to Kind, runs real API tests

## Common Issues and Workarounds

### Issue: CRD changes not reflected
**Symptom**: Modified API types don't show up in cluster  
**Fix**:
```bash
make manifests generate    # Regenerate CRDs and code
make uninstall             # Remove old CRDs
make install               # Install updated CRDs
```

### Issue: Tests timeout
**Symptom**: `make test` hangs or times out  
**Root cause**: Controller tests can take 250+ seconds  
**Fix**: Increase timeout or wait longer (normal behavior)

### Issue: go vet takes very long
**Symptom**: `make vet` appears stuck  
**Root cause**: Downloads many dependencies on first run, takes 60+ seconds  
**Fix**: Wait at least 60 seconds before assuming it's hung

### Issue: E2E tests fail with "cluster not found"
**Symptom**: `make test-e2e` fails with "No Kind cluster 'kind' is running"  
**Fix**: Create cluster first with `make dev-cluster` or `kind create cluster`

### Issue: Webhooks fail in local deployment
**Symptom**: Webhook endpoints not ready  
**Root cause**: Cert-manager not installed  
**Fix**:
```bash
make cert-manager-install  # Installs pinned v1.16.2
make cert-manager-wait     # Wait for it to be ready
```

### Issue: Docker build fails with platform errors
**Symptom**: Cross-platform build fails  
**Root cause**: Dockerfile uses `--platform=$BUILDPLATFORM`  
**Fix**: For local builds, use `make docker-build` (not raw `docker build`)

## Validation Checklist

**Before committing**:
```bash
# Run all checks (required for CI to pass)
make manifests generate fmt vet lint test

# Verify no uncommitted generated files
git status
# Should show no changes in:
# - api/v1alpha1/zz_generated.deepcopy.go
# - config/crd/bases/*.yaml
# - charts/uptime-robot-operator/crds/*.yaml
```

**For CRD changes**:
1. Modify types in `api/v1alpha1/`
2. Add validation tags: `+kubebuilder:validation:...`
3. Run `make manifests generate`
4. Verify CRDs in `config/crd/bases/` and `charts/uptime-robot-operator/crds/`
5. Update controller logic in `internal/controller/`
6. Add/update unit tests
7. Add/update e2e tests (if API key available)
8. Update docs in `docs/` or `charts/uptime-robot-operator/README.md`

**For controller changes**:
1. Modify reconciliation logic in `internal/controller/`
2. Run `make fmt vet lint`
3. Add/update tests in `internal/controller/*_test.go`
4. Run `make test` (takes ~270 seconds)
5. If changing API client: update `internal/uptimerobot/`

## Build Times Reference

| Command | First Run | Subsequent Runs | Notes |
|---------|-----------|-----------------|-------|
| `make fmt` | 2-3s | <1s | Downloads deps on first run |
| `make vet` | 60s | 60s | Always scans entire codebase |
| `make manifests` | 10s | 5s | Downloads controller-gen on first run |
| `make generate` | 10s | 5s | Uses same controller-gen |
| `make lint` | 90s | 60s | Downloads golangci-lint on first run |
| `make test` | 300s | 270s | Downloads setup-envtest, runs all tests |
| `make build` | 120s | 60s | Includes manifests, generate, fmt, vet |
| `make docker-build` | 180s | 60s | Uses build cache |
| `make test-e2e` | 600s | 300s | Creates Kind cluster, installs operator |
| `make test-e2e-real` | 1200s | 1200s | Makes real API calls, 20m timeout |

## Critical Notes

1. **ALWAYS run `make manifests generate` after modifying types in `api/v1alpha1/`**. The CI will fail if generated files are out of sync.

2. **CRDs are automatically synced** from `config/crd/bases/` to `charts/uptime-robot-operator/crds/` by the `make manifests` target (via `make sync-crds`).

3. **Tests are slow** (~270 seconds). This is normal due to controller reconciliation logic. Don't assume a hang until 300+ seconds.

4. **E2E tests have two modes**: basic (no API key) and real (requires `UPTIME_ROBOT_API_KEY`). Basic tests only validate deployment/metrics.

5. **Webhooks require cert-manager**. Local deployments need `make cert-manager-install` before `make deploy`.

6. **Pre-commit hooks** run manifests, generate, go-mod-tidy, go-fumpt, go-vet, and golangci-lint. Install with `pre-commit install` (optional but recommended).

7. **Semantic versioning** is automatic via `.releaserc.yaml`. Commit types (`feat:`, `fix:`, etc.) determine version bumps.

8. **Tool versions** are pinned in Makefile:
   - controller-gen: v0.17.2
   - kustomize: v5.5.0
   - golangci-lint: v1.63.4
   - cert-manager: v1.16.2

9. **Trust these instructions**. Only search the codebase if information is missing or found to be incorrect. The repository structure and build process are stable.
