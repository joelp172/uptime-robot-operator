# Security Policy

This document explains how to verify Uptime Robot Operator images, report vulnerabilities, and apply deployment security best practices.

## Contents

- [Container image security](#container-image-security)
- [Verify image signatures](#verify-image-signatures)
- [Software Bill of Materials (SBOM)](#software-bill-of-materials-sbom)
- [Report vulnerabilities](#report-security-vulnerabilities)
- [Deployment best practices](#deployment-best-practices)

## Container Image Security

All images published by the Uptime Robot Operator are scanned for vulnerabilities, signed with Cosign, and include Software Bill of Materials (SBOM) attestations.

### Image scanning

Every image is scanned using [Trivy](https://github.com/aquasecurity/trivy) for known vulnerabilities before release. The build fails if any critical or high-severity vulnerabilities are detected.

- Scan results are uploaded to the GitHub Security tab
- Vulnerabilities are tracked and remediated promptly
- Images are rebuilt regularly to incorporate security patches

### Image signing

All images are signed using [Cosign](https://github.com/sigstore/cosign) with keyless signing via GitHub Actions OpenID Connect (OIDC). This ensures image authenticity and integrity.

### Base image

The Uptime Robot Operator uses [distroless](https://github.com/GoogleContainerTools/distroless) base images (`gcr.io/distroless/static:nonroot`):

- Contains only the application and its runtime dependencies
- No shell, package manager, or unnecessary tools
- Runs as a non-root user
- Minimises attack surface

## Verify Image Signatures

**Prerequisites:** [Cosign](https://docs.sigstore.dev/cosign/installation) installed.

To verify a signed image:

```bash
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/build.yml@refs/heads/main" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:latest
```

For a specific release version, use the release workflow identity:

```bash
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/release.yml@refs/tags/v1.0.0" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/joelp172/uptime-robot-operator:v1.0.0
```

Successful verification outputs:

```
Verification for ghcr.io/joelp172/uptime-robot-operator:latest --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates
```

## Software Bill of Materials (SBOM)

Each release includes SBOM files in both SPDX and CycloneDX formats. SBOMs provide a complete inventory of all software components in the image.

### Download SBOMs from releases

1. Go to the [Releases](https://github.com/joelp172/uptime-robot-operator/releases) page
2. Download `sbom-spdx.json` or `sbom-cyclonedx.json` from the release assets

### Verify SBOM attestations

SBOMs are attested to the images. Verify them with:

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

### Scan SBOMs for vulnerabilities

Use Trivy to analyse SBOMs for known vulnerabilities:

```bash
trivy sbom sbom-spdx.json
```

## Report Security Vulnerabilities

If you discover a security vulnerability, report it by:

1. **DO NOT** open a public issue
2. Use GitHub's private vulnerability reporting feature: https://github.com/joelp172/uptime-robot-operator/security/advisories/new
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We respond within 48 hours and work with you to address the issue promptly.

## Deployment Best Practices

When you deploy the operator, follow these practices:

### Use specific image tags

Use specific version tags instead of `latest` or `beta`:

```yaml
# Good
image: ghcr.io/joelp172/uptime-robot-operator:v1.0.0

# Avoid
image: ghcr.io/joelp172/uptime-robot-operator:latest
```

### Verify images before deployment

Add image verification to your deployment pipeline:

```bash
#!/bin/bash
set -e

IMAGE="ghcr.io/joelp172/uptime-robot-operator:v1.0.0"

# Use release workflow identity for versioned images
cosign verify \
  --certificate-identity="https://github.com/joelp172/uptime-robot-operator/.github/workflows/release.yml@refs/tags/v1.0.0" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  "${IMAGE}"

# Deploy only if verification succeeds (exit code 0)
kubectl apply -f deployment.yaml
```

### Enable Pod Security Standards

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

### Restrict traffic with network policies

Implement network policies to restrict traffic to and from the operator:

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

### Set resource limits

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

### Store secrets securely

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

Do not commit secrets to version control or store them in plain text.

## Security Scanning Schedule

| Target | When |
|--------|------|
| Images | Every build |
| Dependencies | Dependabot |
| Code | golangci-lint, CodeQL (if enabled) |
| Secrets | Gitleaks on every push and PR |

## Supported Versions

| Version | Supported |
| ------- | --------- |
| latest  | Yes       |
| < 1.0   | No        |

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
