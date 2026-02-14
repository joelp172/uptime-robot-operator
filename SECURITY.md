# Security Policy

## Container Image Security

All container images published by this project are scanned for vulnerabilities, signed with Cosign, and include Software Bill of Materials (SBOM) attestations.

### Image Scanning

Every container image is scanned using [Trivy](https://github.com/aquasecurity/trivy) for known vulnerabilities before release. The build will fail if any critical or high-severity vulnerabilities are detected.

- Scan results are uploaded to GitHub Security tab
- Vulnerabilities are tracked and remediated promptly
- Images are rebuilt regularly to incorporate security patches

### Image Signing and Verification

All container images are signed using [Cosign](https://github.com/sigstore/cosign) with keyless signing via GitHub Actions OIDC (OpenID Connect). This ensures image authenticity and integrity.

#### Verifying Image Signatures

To verify a signed image, install Cosign and run:

```bash
# Install cosign (if not already installed)
# See: https://docs.sigstore.dev/cosign/installation

# Verify the image signature
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/build.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:latest
```

For a specific version (use the release workflow identity):

```bash
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/release.yml@refs/tags/v1.0.0" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:v1.0.0
```

A successful verification will output:

```
Verification for ghcr.io/joelp172/uptime-robot-operator:latest --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates
```

### Software Bill of Materials (SBOM)

Each release includes SBOM files in both SPDX and CycloneDX formats. SBOMs provide a complete inventory of all software components in the container image.

#### Downloading SBOMs from Releases

SBOMs are attached to each GitHub release:

1. Go to the [Releases](https://github.com/joelp172/uptime-robot-operator/releases) page
2. Download `sbom-spdx.json` or `sbom-cyclonedx.json` from the release assets

#### Verifying SBOM Attestations

SBOMs are also attested to the container images and can be verified:

```bash
# Verify SPDX SBOM attestation
cosign verify-attestation \
  --type spdx \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/build.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:latest | jq -r .payload | base64 -d | jq .

# Verify CycloneDX SBOM attestation
cosign verify-attestation \
  --type cyclonedx \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/build.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:latest | jq -r .payload | base64 -d | jq .
```

#### Analyzing SBOMs

You can analyze SBOMs for known vulnerabilities using Trivy:

```bash
# Download the SBOM from a release or extract from attestation
# Then scan it:
trivy sbom sbom-spdx.json
```

### Base Image

This project uses [distroless](https://github.com/GoogleContainerTools/distroless) base images (`gcr.io/distroless/static:nonroot`) which:

- Contains only the application and its runtime dependencies
- Has no shell, package manager, or unnecessary tools
- Runs as a non-root user
- Minimizes attack surface

### Reporting Security Vulnerabilities

If you discover a security vulnerability in this project, please report it by:

1. **DO NOT** open a public issue
2. Use GitHub's private vulnerability reporting feature: https://github.com/joelp172/uptime-robot-operator/security/advisories/new
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will respond within 48 hours and work with you to address the issue promptly.

### Security Best Practices

When deploying the operator, follow these best practices:

#### 1. Use Specific Image Tags

Always use specific version tags instead of `latest` or `beta`:

```yaml
# Good
image: ghcr.io/joelp172/uptime-robot-operator:v1.0.0

# Avoid
image: ghcr.io/joelp172/uptime-robot-operator:latest
```

#### 2. Verify Images Before Deployment

Add image verification to your deployment pipeline:

```bash
#!/bin/bash
set -e

IMAGE="ghcr.io/joelp172/uptime-robot-operator:v1.0.0"

# Verify signature (adjust workflow identity based on whether it's from build.yml or release.yml)
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/release.yml@refs/tags/v1.0.0" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  "${IMAGE}"

# Deploy only if verification succeeds
kubectl apply -f deployment.yaml
```

#### 3. Enable Pod Security Standards

Use Kubernetes Pod Security Standards to enforce security policies:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: uptime-robot-system
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

#### 4. Network Policies

Implement network policies to restrict traffic to/from the operator:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: uptime-robot-operator
  namespace: uptime-robot-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
  - Ingress
  - Egress
  egress:
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443  # UptimeRobot API
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
    ports:
    - protocol: TCP
      port: 443  # Kubernetes API
```

#### 5. Resource Limits

Set appropriate resource limits to prevent resource exhaustion:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 10m
    memory: 64Mi
```

#### 6. Secret Management

Use Kubernetes secrets with appropriate RBAC permissions:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: uptime-robot-api-key
  namespace: uptime-robot-system
type: Opaque
stringData:
  apiKey: "your-api-key-here"
```

Never commit secrets to version control or store them in plain text.

## Security Scanning Schedule

- **Container images**: Scanned on every build
- **Dependencies**: Monitored via Dependabot
- **Code**: Scanned via golangci-lint and CodeQL (if enabled)
- **Secrets**: Scanned via Gitleaks on every push and PR

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < 1.0   | :x:                |

We support the latest released version. Security patches are backported on a case-by-case basis.

## Security Tools Used

- **Trivy**: Container image and SBOM scanning
- **Cosign**: Image signing and verification
- **Gitleaks**: Secret scanning
- **golangci-lint**: Static code analysis
- **Distroless**: Minimal base images
- **Dependabot**: Dependency updates

## Additional Resources

- [Sigstore Documentation](https://docs.sigstore.dev/)
- [SLSA Framework](https://slsa.dev/)
- [Supply Chain Levels for Software Artifacts](https://slsa.dev/)
- [NIST SSDF](https://csrc.nist.gov/Projects/ssdf)
- [CycloneDX SBOM Standard](https://cyclonedx.org/)
- [SPDX SBOM Standard](https://spdx.dev/)
