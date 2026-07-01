# Docker Raft App Smoke Tests

These smoke tests verify heavier application deployments on the Docker runtime:

```text
3 orch-server containers -> Dragonboat Raft membership -> orch-cli stack apply --file examples/integration/<app>.orch --watch -> worker dispatch -> Docker runtime -> orch DNS -> built-in ingress -> application containers
```

The workload definitions are committed as native `.orch` DSL files under
`examples/integration/`. The script does not generate deploy YAML; it starts a 3-node cluster,
deploys with:

```text
orch-cli stack apply --file examples/integration/<app>.orch --watch
```

then verifies `stack status`, waits for readiness-gated status via `orch wait app <name> --for running`, checks assignment/volume lifecycle bindings, and validates ingress routing.

## Runtime Isolation Modes

The default mode is `-RuntimeIsolation shared`. Each orch-server container mounts
the host Docker socket, so all nodes dispatch workloads into the same local Docker
daemon. This is fast for deploy, ingress, and local volume-binding checks, but it
is not a faithful workload-movement model: a stopped orch-server node does not stop a
separate Docker host, and all nodes share container names.

Use `-RuntimeIsolation dind` for a minimum real multi-node runtime shape. In this
mode each node is a privileged `docker:dind` container that starts its own
`dockerd` and then runs `orch-server` inside that same container. Workloads are
created inside the assigned node's nested Docker daemon, not in the host daemon.
The script creates a same-named Docker network inside each nested daemon so the
existing `.orch` manifests can be reused. Before deploy, it parses workload image
references from the manifest, saves them from the host Docker cache, and loads them
into each nested daemon to avoid repeated Docker Hub pulls during the test.

DinD mode requires a Docker host that allows privileged containers and may need
to pull `docker:27-dind` plus workload images from Docker Hub. Host `docker ps`
will show the three orch/DinD node containers; inspect nested workloads with:

```powershell
docker exec orch-placement-node-b docker ps
```

Use `-KeepCluster -KeepWorkload` together when inspecting dind workloads after a
run, because those workloads live inside the per-node daemons.

## Scenarios

### Placement / Failover

```powershell
task smoke:docker-raft-placement
```

Deploy file:

```text
examples/integration/placement.orch
```

Deploys `traefik/whoami:v1.11` as a lightweight HTTP service pinned to
`node-b`. The workload includes an HTTP readiness probe, so the app reaches
`running` only after whoami accepts traffic.

After the deploy succeeds, the smoke exercises stack lifecycle:

- `stack status placement-smoke`
- `stack stop placement-smoke`
- `stack start placement-smoke`
- `stack restart placement-smoke`

And then user-facing control-plane placement ops:

- `migrate app placement-smoke --to <node> --workload whoami`
- `rebalance app placement-smoke --workload whoami`
- stop one non-leader orch-server container to simulate a worker/control-plane
  node outage
- `failover app placement-smoke --to <survivor> --workload whoami`

In `dind` mode, each node owns a separate Docker daemon. In this mode,
each movement step waits for expected assignment and mounted volume binding, then checks
whoami through live ingress ports.

### Rollout Auto-Rollback

```powershell
task smoke:docker-raft-rollout
```

Deploy files:

```text
examples/integration/rollout.orch
examples/integration/rollout-broken.orch
```

Deploys the same `traefik/whoami:v1.11` workload as a stable baseline, then
deploys a second manifest with an intentionally broken readiness path and
`rollout.rollback_on_failure = true`. The smoke expects `orch stack apply --watch`
to fail for the broken revision, waits for app status to return to the previous
desired generation with rollback revisions recorded, and verifies ingress works
again through all three nodes.

### Nextcloud

```powershell
task smoke:docker-raft-nextcloud
```

Deploy file:

```text
examples/integration/nextcloud.orch
```

Deploys official Docker Hub images:

- `nextcloud:29-apache`
- `postgres:16-alpine`
- `redis:7-alpine`

The test pins all workloads to `node-b`, configures workload DNS through orch,
waits for all assignments to become `running`, then requests all three ingress
ports. This verifies Nextcloud can start with Postgres and Redis dependencies
and that non-owner ingress nodes forward to the owner node.

### SeaweedFS

```powershell
task smoke:docker-raft-seaweed
```

Deploy file:

```text
examples/integration/seaweed.orch
```

Deploys `chrislusf/seaweedfs:3.85` as a production-shaped smoke topology:

- three masters: `seaweedmastera`, `seaweedmasterb`, `seaweedmasterc`
- three volume servers: `seaweedvolumea`, `seaweedvolumeb`, `seaweedvolumec`
- three filers: `seaweedfilera`, `seaweedfilerb`, `seaweedfilerc`
- gateway/admin workloads: `seaweeds3`, `seaweedwebdav`, `seaweedadmin`, `seaweedworker`

The workload placements are spread across `node-a`, `node-b`, and `node-c`.

### Runnable Chain

Each scenario follows the same user-facing path:

1. Start a three-node orch-server Raft cluster in Docker.
2. Deploy the committed `.orch` file with `orch stack apply --file ... --watch` and verify `stack status`.
3. Wait for readiness-gated `running` state with `orch wait app <name> --for running`.
4. Assert expected workload assignments and runtime volume bindings.
5. Request the application through ingress (if the scenario exposes it).
6. Exercise stack lifecycle ops: `stack status`, `stack stop`, `stack start`, `stack restart`.
7. For `task smoke:docker-raft-placement-dind`, run `migrate`, `rebalance`, and `failover` with node failure simulation.
8. For `task smoke:docker-raft-rollout`, apply the broken rollout manifest, expect watch failure, wait for auto rollback, and re-check ingress.

## Full Stack Suite

```powershell
task smoke:docker-raft-stack-suite
task smoke:docker-raft-stack-suite-full
task smoke:docker-raft-stack-suite -CLI_ARGS "-SkipStackLifecycle"
task smoke:docker-raft-stack-suite -CLI_ARGS "-TimeoutSeconds 450"
```

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario placement
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario rollout
```

## Direct Commands

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario placement
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario placement -RuntimeIsolation dind
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario rollout
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario nextcloud
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario seaweed
```

Use `-Manifest` to point at another `.orch` file. If you also change
`-NetworkName`, pass a matching manifest because the committed `.orch` files pin
`docker.network` to the default smoke network.

Use `-KeepCluster -KeepWorkload` to inspect a successful run before cleanup.

## Current Boundaries

These tests validate deployment, connectivity, and local runtime volume binding lifecycle.
Docker runtime `mounts` now map to local Docker named volumes, and cleanup waits for
bindings to become `released`. In `dind` mode those named volumes are local to the
assigned node's nested daemon, which is useful for lifecycle isolation checks but
still does not validate cross-node shared storage, attach/detach orchestration,
or data migration.
