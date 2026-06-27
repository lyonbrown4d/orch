# orch Roadmap

Snapshot date: June 27, 2026

## Completed

- [x] Modular server composition and lifecycle wiring through `dix`.
- [x] Config loading from defaults, env, and optional config files.
- [x] Canonical deploy model in `internal/deploy/v1alpha1`.
- [x] Compatibility loaders for YAML, Docker Compose import, and the short
      native `.orch` DSL.
- [x] Docker deploy lifecycle with CLI/API `apply`, `get`, `describe`, `logs`,
      `stop`, `start`, `restart`, and `delete`.
- [x] Stack-oriented CLI surface for apply/update/server-side rollback, list,
      status, and lifecycle operations.
- [x] Host port conflict preflight against incoming stack manifests and existing
      desired apps before Raft state is accepted.
- [x] Explicit rollout strategy validation for workload update policy fields.
- [x] Runtime abstraction with providers for `docker`, `containerd`, `podman`,
      `firecracker`, `process`, `systemd`, and `windows-service`.
- [x] Runtime-neutral spec shape with `run.artifact`, `run.exec`, resources,
      endpoints, mounts, and typed `runtimeOptions`.
- [x] Workload DNS lifecycle for deploy/stop/delete and per-workload DNS
      injection for supported container paths.
- [x] Built-in HTTP ingress through `github.com/arcgolabs/vale`.
- [x] Host DNS installer baseline for Linux `systemd-resolved`, macOS resolver
      files, and Windows NRPT-style setup.
- [x] Dragonboat-backed Raft service with single-node persistence, TCP
      transport, static peer bootstrap, status APIs, and basic add/remove voter
      operations.
- [x] Follower write forwarding for deploy lifecycle and Raft membership writes
      when `cluster.nodes` maps node IDs to API URLs.
- [x] Scheduler assignment records replicated through Raft.
- [x] Worker dispatch baseline through the worker API.
- [x] Baseline explicit `migrate`, `failover`, and `rebalance` operations.
- [x] CLI output improvements for app/workload/assignment/node and readiness
      flows.
- [x] Local smoke harnesses for Docker lifecycle, workload DNS, worker dispatch,
      and three-node Raft forwarding.
- [x] Docker-based integration manifests for full-stack, Nextcloud, SeaweedFS,
      Kafka, and PostgreSQL HA scenarios.
- [x] GoReleaser/nFPM packaging for archives and Linux `.deb`, `.rpm`, `.apk`.
- [x] Repository lint is clean under `golangci-lint run ./...`.

## In Progress

- [ ] Beta release gate hardening across lint, tests, GoReleaser snapshot, Raft
      smoke, Docker lifecycle smoke, workload DNS smoke, worker dispatch smoke,
      and Docker-based integration smoke.
- [ ] Stack reliability hardening for multi-node deploy, cross-node forwarding,
      dependency ordering, health-gated readiness, and node-failure behavior.
- [ ] Update and rollback hardening beyond previous-revision rollback: rollout
      progress deadlines, automatic rollback on failed replacement, rollout
      history, and disruption budgets.
- [ ] Runtime parity hardening across `containerd`, `podman`, `firecracker`,
      `process`, `systemd`, and `windows-service`.
- [ ] Containerd CRI status/log/recovery parity and richer network semantics.
- [ ] Firecracker production ergonomics: TAP/bridge management, jailer
      integration, recovery, and rootfs/image preparation workflow.
- [ ] Systemd and Windows Service provider parity: status, logs, recovery, and
      service wrapper ergonomics.
- [ ] Worker dispatch hardening: retries/backoff, idempotency, auth/token
      handling, remote status, and remote logs.
- [ ] Raft multi-node hardening: membership guardrails, restart recovery
      coverage, leader transfer/failover behavior, and snapshot/restore checks.
- [ ] Stateful-safe migration/failover/rebalance policy beyond the current
      guardrails.
- [ ] Operational docs for single-node, local cluster, and beta deployment
      flows.

## Planned

### Runtime and orchestration

- [ ] Provider compatibility matrix with documented lifecycle support per
      runtime.
- [ ] Health checks and lifecycle hooks tied into scheduler/reconcile behavior.
- [ ] Secrets injection and secret source integrations.
- [ ] Policy-driven drain, migration, rollback, and disruption budgets.
- [ ] Remote worker log aggregation and historical assignment events.
- [ ] Shared storage story for stateful migration: local runtime mounts first,
      then documented external/shared-volume backends.

### Cluster and platform capabilities

- [ ] Stronger Raft membership safety checks before quorum-affecting edits.
- [ ] Fine-grained API authn/authz and token lifecycle management.
- [ ] Multi-node ingress hardening around Vale integration, certificate/DNS
      state, and Raft-backed ingress metadata.
- [ ] Production upgrade and rollback playbooks.
- [ ] Resource usage benchmark automation for idle, cluster, and scheduling
      scenarios.
- [ ] Packaging/install hardening for host DNS setup across Linux, macOS, and
      Windows installers.

### Developer and user experience

- [ ] Complete `.orch` DSL examples for common app shapes.
- [ ] Docker-based e2e suites that drive `orch-cli stack apply/status/delete`
      against a three-node orch cluster and gateway request path.
- [ ] Pack format, metadata, and remote registry/distribution model.
- [ ] External dashboard integration boundary documentation.
- [ ] Release notes and migration notes for each beta cut.

## Out of Scope For Current Beta

- [ ] New Docker Compose migration/conversion tooling. The existing Compose
      import compatibility path can stay, but stack management should use orch
      manifests as the primary source of truth.
