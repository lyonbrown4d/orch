# Beta Release

This page defines the release bar for `v0.1.0-beta.*`.

## Supported Beta Scope

The beta is intended for developer trials and small controlled environments.

Supported paths:

- CLI/server binaries for Linux, macOS, and Windows.
- Linux packages through GoReleaser/nFPM: `.deb`, `.rpm`, and `.apk`.
- Docker runtime deploy lifecycle: `apply`, `get`, `describe`, `stop`, `start`,
  `restart`, and `delete`.
- Worker dispatch between scheduler and worker server processes.
- Workload DNS for supported container paths, with configurable upstream DNS.
- Built-in HTTP ingress through `github.com/arcgolabs/vale`.
- Static Raft bootstrap, basic add/remove voter operations, and follower write
  forwarding when `cluster.nodes` maps leader IDs to API URLs.
- Baseline explicit `migrate`, `failover`, and `rebalance` operations.

Experimental in beta:

- `containerd` CRI runtime status/recovery behavior.
- Firecracker TAP/bridge and image preparation workflow.
- Linux `systemd` runtime and Windows `windows-service` runtime.
- Host DNS installer behavior outside common Linux `systemd-resolved`, macOS
  resolver, and Windows registry/resolver setups.

Not promised in beta:

- Automatic node failure detection and automatic failover.
- Stateful volume/data migration.
- Raft quorum safety guardrails for every membership edit.
- Production hardening for rolling upgrades and rollback.
- Full TCP/UDP ingress parity.

## Release Gate

Run these before tagging:

```bash
go mod tidy
golangci-lint run ./... --allow-serial-runners
go test ./...
task goreleaser-check
task release-snapshot
task smoke:local-raft-forwarding
```

Run these smoke tests on a host with the required runtime:

```powershell
task smoke:local-docker
task smoke:local-docker-dns
task smoke:local-docker-worker-dispatch
task smoke:local-podman
task smoke:local-podman-dns
task smoke:local-podman-worker-dispatch
task smoke:docker-raft-stack-suite
task smoke:docker-raft-stack-suite-dind # validates per-node runtime movement (migrate/rebalance/failover path)
task smoke:docker-raft-stack-suite-full-dind # optional dind runtime full-state scenarios
task smoke:docker-raft-stack-suite-full  # optional stateful scenarios (nextcloud/seaweed)
task smoke:full-chain-full-dind # optional full e2e DinD path
task smoke:full-chain-full # optional full e2e shared path
```

Run the package/systemd smoke on hosts that allow privileged Docker containers:

```powershell
task smoke:systemd-docker
```

`smoke:local-docker` and `smoke:local-podman` require Docker/Podman respectively.
`smoke:local-docker-dns` and `smoke:local-podman-dns` additionally require host
DNS port `53` availability.

`smoke:local-podman` and `smoke:local-podman-dns` require Podman installed and
available on PATH.

`smoke:local-docker-dns` requires Docker and the host DNS port used by the smoke test
server to be available.

The Taskfile exposes the same checks as:

```bash
task release-gate:static
task release-gate
task release-gate:runtime
task release-gate:full
task release-gate:full-dind
task release-gate:e2e
```

`release-gate:static` runs the non-runtime checks plus the local Raft forwarding
smoke. `release-gate` runs baseline + runtime smoke checks; `release-gate:runtime` runs only runtime smoke checks; `release-gate:full` runs shared full-chain e2e (baseline + runtime + shared runtime movement); and `release-gate:full-dind` runs the DinD movement full-chain path.
`smoke:systemd-docker` is kept separate because it requires privileged Docker and systemd-in-container support.

To run the same staged checks locally:

```bash
task release-gate:static
task release-gate:runtime
task release-gate:full
task release-gate:full-dind
```

### Manual Gate Workflow

You can run the manual release gate workflow through `workflow_dispatch` on `.github/workflows/release-gate.yml` with optional toggles. The workflow delegates to `task release-gate:local` with the corresponding args.

`include_runtime` enables container runtime checks.
`include_e2e` enables the extended full-chain e2e gate.
`include_e2e_dind` enables the DinD full-chain path for placement/migrate/rebalance/failover coverage.

Workflow input examples:

```yaml
# Baseline only (same as task release-gate:static)
include_runtime: false
include_e2e: false
include_e2e_dind: false
```

```yaml
# Runtime smoke checks
include_runtime: true
include_e2e: false
include_e2e_dind: false
```

```yaml
# Full e2e (runtime + e2e) in one pass
include_runtime: true
include_e2e: true
include_e2e_dind: false
```
```yaml
# Full e2e with DinD movement path
include_runtime: true
include_e2e: true
include_e2e_dind: true
```

Execution mapping:

```text
include_runtime=false, include_e2e=false => task release-gate:local
include_runtime=true,  include_e2e=false => task release-gate:local CLI_ARGS="-Runtime"
include_runtime=true,  include_e2e=true  => task release-gate:local CLI_ARGS="-Runtime -E2E"
include_runtime=true,  include_e2e=true, include_e2e_dind=true  => task release-gate:local CLI_ARGS="-Runtime -E2E -E2EDind"
include_runtime=false, include_e2e=true => task release-gate:local CLI_ARGS="-E2E"
include_runtime=false, include_e2e=true, include_e2e_dind=true => task release-gate:local CLI_ARGS="-E2E -E2EDind"
```

When `include_runtime` is enabled, the workflow runs `task smoke:local-container-runtimes` after `task release-gate:static` and before optional `release-gate:e2e` (or DinD when `include_e2e_dind` is true).

## Tagging

Use prerelease semver tags:

```bash
git tag -a v0.1.0-beta.1 -m "v0.1.0-beta.1"
git push origin v0.1.0-beta.1
```

Pushing a `v*` tag runs `.github/workflows/release.yml`, which publishes
archives, checksums, and Linux packages through GoReleaser.

## Manual Snapshot

For a local dry run without publishing:

```bash
task release-snapshot
```

Artifacts are written to `dist/`.
