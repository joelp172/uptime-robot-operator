# Contributing

Thank you for contributing to the Uptime Robot Operator.

## Before You Start

- Check [existing issues](https://github.com/joelp172/uptime-robot-operator/issues) before creating new ones
- For major changes, open an issue first to discuss the approach
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md)

## Development Setup

See the [Development Guide](docs/development.md) for:
- Setting up your environment
- Running tests
- Local development workflow
- Project structure

## Pull Request Process

### 1. Create Your Branch

```bash
git checkout -b feat/my-feature
```

### 2. Make Your Changes

- Write clear, focused commits
- Add tests for new functionality
- Update documentation if needed

### 3. Run Pre-Commit Checks

```bash
make manifests generate fmt vet lint test
```

All checks must pass before submitting.

### 4. Commit Message Format

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(monitor): add DNS monitor support
fix(controller): handle rate limiting correctly
docs: update installation guide
chore: update dependencies
```

**Release triggers:**

| Type | Version Bump |
|------|--------------|
| `feat:` | Minor (1.x.0) |
| `fix:` | Patch (1.0.x) |
| `docs:`, `chore:`, `ci:`, `refactor:`, `test:` | None |

### 5. Submit Pull Request

1. Push your branch to your fork
2. Create a draft PR while work is in progress
3. Mark as ready for review when tests pass
4. Ensure CI passes before requesting review

### PR Checklist

- [ ] Tests pass locally (`make test`)
- [ ] Linting passes (`make lint`)
- [ ] Code formatted (`make fmt`)
- [ ] Generated files updated (`make manifests generate`)
- [ ] E2E tests pass (if applicable)
- [ ] Documentation updated
- [ ] Commit messages follow convention

## Adding New Fields

When adding fields to CRDs:

1. Update API types in `api/v1alpha1/`
2. Add validation tags
3. Run `make manifests generate`
4. Update controller logic
5. Add unit and e2e tests
6. Update documentation

See `.cursor/rules/new-field-checklist.mdc` for the complete checklist.

## Code Review

Expect feedback on:
- Code quality and style
- Test coverage
- Documentation clarity
- Breaking changes

Be responsive to feedback and iterate on your PR.

## Getting Help

- [Development Guide](docs/development.md) - Setup and testing
- [GitHub Issues](https://github.com/joelp172/uptime-robot-operator/issues) - Bug reports and features
- [Discussions](https://github.com/joelp172/uptime-robot-operator/discussions) - Questions and ideas
