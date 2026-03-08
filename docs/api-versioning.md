# API Versioning and Stability Policy

This page explains what the current `uptimerobot.com/v1alpha1` API means
for you, how versions will evolve, and how upgrades will be handled.

## Current Stability Level (`v1alpha1`)

All Custom Resource Definitions (CRDs) are currently served as
`uptimerobot.com/v1alpha1`.

`v1alpha1` means:

- The API is usable in production, but it is still evolving.
- Fields and behavior may change between minor operator releases.
- Breaking API changes are possible before `v1` unless otherwise noted.

If you run this operator in production, pin the operator version and read
release notes before upgrading.

## Graduation Plan (`v1alpha1` -> `v1beta1` -> `v1`)

There is no fixed calendar date for graduating to `v1beta1` or `v1`.
Graduation is criteria-based.

### Criteria for `v1beta1`

The project will target `v1beta1` after these conditions are met:

- Core schemas are stable across multiple releases.
- Validation rules and defaulting behavior are well-defined.
- Controller behavior is covered by unit and end-to-end tests.
- Upgrade and rollback paths are documented and tested.

### Criteria for `v1`

The project will target `v1` when:

- The API shape is stable and broadly validated by real-world usage.
- Remaining planned breaking changes have been completed.
- Versioned conversion and migration paths are proven in releases.

## Version Changes and Conversion Webhooks

When a new API version is introduced (for example, `v1beta1`), the project
will provide a migration path before removing older served versions.

The project commitment is:

- Add versioned CRD support for old and new versions during migration.
- Provide conversion webhooks when schema differences require conversion.
- Document field-level mapping and behavior changes for each version jump.

## Deprecation Policy for Alpha Versions

When a new API version supersedes `v1alpha1`, `v1alpha1` enters deprecation.

The deprecation policy is:

- Keep deprecated versions served for at least two minor operator releases
  after deprecation is announced.
- Announce deprecations in release notes with clear upgrade guidance.
- Remove a deprecated version only after migration documentation is
  available and conversion behavior is documented.

## Upgrade and Migration Guidance

When API versions change, use this process:

1. Review release notes for version and schema changes.
2. Apply the new operator version and CRD updates.
3. Validate existing resources reconcile successfully.
4. Update manifests to the new `apiVersion`.
5. Remove deprecated versions only after your workloads are migrated.

For practical migration examples, see:

- [Migration Guide](migration-guide.md)
- [API Reference](api-reference.md)
