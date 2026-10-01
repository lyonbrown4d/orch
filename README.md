# orch

orch is a lightweight runtime and control layer for long-lived services (database, message queue, object storage, and other always-on workloads) outside traditional container orchestration systems.

## Status

orch is being built in Go and focuses on:

- Multi-runtime workload execution (`docker`, `containerd`, `firecracker`, `process`, `systemd`, `windows-service`)
- Built-in ingress and DNS record lifecycle
- Raft-aware scheduling and operations (deploy/stop/delete plus migrate/failover/rebalance baseline)
- Declarative Workload DSL direction with compatibility inputs and `plan/render/apply/delete`

Current core stack:

- DI: `github.com/arcgolabs/dix`
- Logging: `github.com/arcgolabs/logx`
- HTTP server/API: `github.com/arcgolabs/httpx` (`fiber` adapter)
- Ingress: `github.com/arcgolabs/vale` runtime/proxy (round-robin); optional **Let's Encrypt** via `ingress.tls` (`golang.org/x/crypto/acme/autocert`, TLS-ALPN-01).
- CLI: `github.com/spf13/cobra`
- Consensus path: `github.com/lni/dragonboat/v4` over TCP transport with static peer bootstrap, basic membership commands, and follower write forwarding when `cluster.nodes` maps leader IDs to API URLs

## Project Structure

```text
cmd/
  orch-cli/              # cobra entrypoint
  orch-server/           # process entrypoint + graceful shutdown
internal/
  config/                # env-based config
  logging/               # logx factory
  security/auth/         # authx guard module
  httpserver/            # httpx bootstrap (adapter/fiber + openapi)
  api/                   # typed route registration
  deploy/v1alpha1/       # canonical deploy YAML model
  runtime/
    docker/              # docker provider (v1)
    containerd/          # containerd provider (v1)
    firecracker/         # Linux Firecracker microVM provider (v1)
    process/             # local subprocess provider (v1)
    systemd/             # Linux systemd unit provider (v1)
    windowsservice/      # Windows Service provider (v1)
  services/
    registry/            # workload registry
    task/                # deploy orchestration service
  raftsvc/               # Dragonboat-backed consensus service boundary
```

## Quick Start

### Parse a deploy YAML file

```bash
go run ./cmd/orch-cli parse --file path/to/app.yaml --json
```

### Run server (skeleton)

```bash
go run ./cmd/orch-server
```

### Worker dispatch (dev)

When placement selects a node other than the local server, orch dispatches the workload to that node's worker API. Configure node IDs to API base URLs with `cluster.nodes`:

```yaml
cluster:
  nodes:
    node-a: http://10.0.0.11:17443
    node-b: http://10.0.0.12:17443
  # Optional when HTTP auth is enabled on worker nodes:
  worker_token: "<bearer-token>"
```

The worker endpoint executes the assigned workload locally and does not mutate Raft desired state.
The scheduler records workload assignment results in Raft; inspect apps with
`orch get apps` / `orch describe app NAME`, or inspect lower-level assignments
with `orch get assignments --json` (or the legacy `orch assignments --json`) or
`GET /api/v1/assignments`.

Start/stop/delete an app through the control plane:

```bash
go run ./cmd/orch-cli stop app my-app -n default
go run ./cmd/orch-cli start app my-app -n default
go run ./cmd/orch-cli restart app my-app -n default
go run ./cmd/orch-cli delete app my-app -n default
go run ./cmd/orch-cli migrate app my-app --to node-b -n default
go run ./cmd/orch-cli failover app my-app -n default
go run ./cmd/orch-cli rebalance app my-app -n default
```

`stop` stops assigned workloads while keeping the desired app document. `start`
uses retained desired state to run the app again, `restart` does stop then
start, and `delete` stops assigned workloads first before removing the desired
app from Raft. `migrate` moves selected workloads to `--to`, `failover` moves
failed workloads (or explicitly selected workloads) to another node, and
`rebalance` re-runs placement and only moves workloads whose selected node
changes.

Raft membership basics:

```bash
go run ./cmd/orch-cli raft status
go run ./cmd/orch-cli raft members
go run ./cmd/orch-cli raft add-voter node-b 10.0.0.12:7444
go run ./cmd/orch-cli raft remove-voter node-b
```

When `cluster.nodes` maps the current leader ID to its HTTP API URL, follower nodes forward deploy lifecycle writes and Raft membership writes to the leader. Use `raft status` to see local state, the known leader, and the configured leader API URL. Dynamically joined nodes should start with `raft.bootstrap: false`.

### Local Docker smoke test

Run a complete single-node smoke flow (server -> CLI deploy -> Docker runtime -> CLI status checks -> CLI stop/start/restart/delete):

```powershell
task smoke:local-docker
task smoke:local-docker-dns
task smoke:local-docker-worker-dispatch
task smoke:local-raft-forwarding
task smoke:vagrant
```

The lifecycle smoke manifest is `examples/local-docker-smoke.yaml`; details are
in [Local Docker Smoke Test](docs/local-docker-smoke.md). The DNS smoke uses
`examples/local-docker-dns-smoke.yaml` to verify workload DNS with
`dns-backend.default.svc.orch.local`; details are in
[Local Docker DNS Smoke Test](docs/local-docker-dns-smoke.md). The worker
dispatch smoke starts scheduler and worker server processes and verifies
dispatch through the worker API; details are in
[Local Docker Worker Dispatch Smoke Test](docs/local-docker-worker-dispatch-smoke.md).
The Raft forwarding smoke starts a local three-node cluster and verifies
apply/delete through a follower; details are in
[Local Raft Cluster](docs/local-raft.md).
The virtualized flow runs three VMs end-to-end and is documented in
[Local Vagrant Smoke Test](docs/local-vagrant-smoke.md).
For a reproducible virtualized 3-node end-to-end flow, use `task smoke:vagrant`
with `scripts/vagrant/smoke.ps1` and `examples/local-vagrant-smoke.yaml`.

For a complete application shape in the short native `.orch` DSL (frontend, backend,
Postgres, Redis, ingress), see `examples/fullstack-docker.orch` and
[Full-Stack Docker Example](docs/fullstack-docker.md).

## Documentation (mdBook)

Docs are now maintained with `mdBook`.

```bash
mdbook serve docs
```

See:

- [Introduction](docs/introduction.md)
- [Project Status](docs/project-status.md)
- [Quick Start](docs/quick-start.md)
- [Beta Release](docs/release-beta.md)
- [Runtime Compatibility Matrix](docs/runtime-compatibility.md)
- [Gossip Cluster Discovery](docs/gossip-clustering.md)
- [Local Docker Smoke Test](docs/local-docker-smoke.md)
- [Local Docker Worker Dispatch Smoke Test](docs/local-docker-worker-dispatch-smoke.md)
- [Local Vagrant Smoke Test](docs/local-vagrant-smoke.md)
- [Full-Stack Docker Example](docs/fullstack-docker.md)
- [Workload DSL v1 (EN)](docs/dsl.md)
- [Workload DSL v1（中文）](docs/dsl.zh.md)
- [Ingress Design v1 (EN)](docs/ingress.md)
- [Ingress 设计 v1（中文）](docs/ingress.zh.md)
- [Local Raft Cluster](docs/local-raft.md)

## Development

```bash
go mod tidy
go test ./...
```

### Release builds (binaries, `.deb`, `.rpm`, `.apk`)

Recommended workflow is **[GoReleaser](https://goreleaser.com)** with **[nFPM](https://nfpm.goreleaser.com)** (configured in `.goreleaser.yaml`): cross-compile **orch** / **orch-server**, tar/zip archives, checksums, and Linux packages (`deb` / `rpm` / `apk`) in one pipeline.

```bash
task goreleaser-check              # validate config
task release-snapshot              # outputs under dist/ (no tag required)
task release-gate:static           # beta gate without Docker smoke tests
task release-gate                  # full beta gate including local Docker smoke tests
# publish beta: git tag -a v0.1.0-beta.1 -m "v0.1.0-beta.1" && git push origin v0.1.0-beta.1
```

Pushing a `v*` tag runs `.github/workflows/release.yml` and publishes archives,
checksums, and Linux packages. See [Beta Release](docs/release-beta.md) for the
supported beta scope, release gate, and known limitations.

Other common approaches:

| Approach | Notes |
|----------|--------|
| **GoReleaser** (this repo) | Single YAML; archives + nfpm packages + optional Homebrew/Chocolatey/GitHub Releases |
| **[nfpm](https://github.com/goreleaser/nfpm) alone** | Only packaging step: you build binaries (`go build`), nfpm produces deb/rpm/apk from `nfpm.yaml` |
| **[fpm](https://github.com/jordansissel/fpm)** | Flexible Ruby-based packager; many formats, extra tooling |
| **rpmbuild / debhelper** | Classic distro-native specs (`*.spec`, `debian/`) — best when targeting specific distributions |
| **OBS / COPR / PPA** | Build RPM/DEB on shared infra for multiple base OS versions |

Full matrix builds must succeed (`go test ./...` and cross-compiles); narrow `builds` targets in `.goreleaser.yaml` if you need to iterate before fixing every GOOS/GOARCH combination.

### Dev Container (VS Code / Cursor / Codespaces)

Open the repo in a container using `.devcontainer/`: Go 1.27 (bookworm), `task`, Delve, and the **Docker CLI** are preinstalled; port `17443` is forwarded for `orch-server` HTTP. After the container builds, `postCreateCommand` runs `go mod download` and `docker version`. For mdBook locally, install a [release binary](https://github.com/rust-lang/mdBook/releases) or use mdBook on the host.

**Docker vs nested runtimes**

- **Default (Docker-from-Docker):** the devcontainer [docker-outside-of-docker](https://github.com/devcontainers/features/tree/main/src/docker-outside-of-docker) feature mounts the **host** Docker socket. Commands like `docker run` / `docker compose` use **your machine’s Docker engine** (e.g. Docker Desktop on Windows/macOS). That is enough to exercise the **docker** runtime provider without running a second daemon inside the devcontainer.
- **Docker-in-Docker:** only if you need an **isolated** daemon (no host socket), swap the feature for `ghcr.io/devcontainers/features/docker-in-docker` and add `"runArgs": ["--privileged"]` (heavier; can be awkward with Docker Desktop). See the [feature docs](https://github.com/devcontainers/features/tree/main/src/docker-in-docker).
- **containerd provider:** orch uses containerd through its **CRI plugin** and creates CRI pod sandboxes, so containerd must have CRI enabled and a working CNI setup. It is not the same path as “Docker CLI + host engine”. Nested **containerd + nerdctl** inside a devcontainer is possible but heavy (extra daemons, Linux-only nuances). Typical approaches: run containerd-backed tests on a **Linux host/VM/CI** with `/run/containerd/containerd.sock` (or your distro path) reachable, or develop the docker path in devcontainer and run containerd integration elsewhere.

## Ingress Configuration

Environment variables use the **`ORCH`** prefix (see `internal/config`, loaded via configx).

- `ORCH_INGRESS_ENABLED` (default: `true`)
- Default listeners: `:80` and `:443`. Set `ORCH_INGRESS_ADDR` to use a single port instead (for example `:8088` on machines where binding low ports requires elevated privileges).

## Runtime Provider Discovery

orch-server detects host runtime providers at startup and exposes the snapshot
through `orch get runtimes` / `GET /api/v1/system/runtimes`. Provider policy can
be set in config with `runtime.providers.<name>.policy`:

```yaml
runtime:
  providers:
    docker:
      policy: required # auto | required | disabled
```

## License

MIT
