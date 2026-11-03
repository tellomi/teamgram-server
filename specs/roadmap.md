# Roadmap

This document outlines short- and medium-term goals. What the server implements is in `docs/`. The old upstream split between a community tree and an enterprise tree is not a map of this repository.

## Short-term

- **Docs and specs**: Keep specs (architecture, protocol, dependencies, contributing, security, release, roadmap) in sync with the codebase.
- **CI**: Keep the workflows in `.github/workflows/` (Publish images, `go test`, client regression, UI acceptance) green.
- **Doc consistency**: Keep README, `docker-compose-env.yaml`, and the install guides describing the files that exist.

## Medium-term

- **Test coverage**: Add unit and integration tests for critical paths; define coverage expectations over time.
- **Observability and ops**: Document runbooks and usage of the monitoring stack (Prometheus, Grafana, Jaeger, ELK).
- **Install and deploy**: Support more environments (e.g. other Linux distros, Kubernetes examples) and keep them consistent with docker-compose-env.

## Scope

Feature behavior lives in `docs/` (calls, storage, push, stickers, and the API gap list). Do not treat an upstream Teamgram enterprise catalog as the list of what this tree lacks or ships. Production operations are in [docs/SECURITY-OPS.md](../docs/SECURITY-OPS.md).
