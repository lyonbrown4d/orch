# Docker Raft App Smoke Tests

These smoke tests verify heavier application deployments on the Docker runtime:

```text
3 orch-server containers -> Dragonboat Raft membership -> orch-cli apply --file examples/integration/<app>.orch -> worker dispatch -> Docker runtime -> orch DNS -> built-in ingress -> application containers
```

The workload definitions are committed as native `.orch` DSL files under
`examples/integration/`. The script does not generate deploy YAML; it starts the
test cluster, calls the CLI with the workload file, waits for status, and then
runs HTTP checks through ingress, and checks local runtime volume bindings when the scenario declares mounts.

## Runtime Isolation Modes

The default mode is `-RuntimeIsolation shared`. Each orch-server container mounts
the host Docker socket, so all nodes dispatch workloads into the same local
Docker daemon. This is fast for deploy, ingress, and volume-binding lifecycle
checks, but it is not a faithful workload movement model: a stopped orch-server
node does not stop a separate Docker host, and all nodes share container names.

Use `-RuntimeIsolation dind` for a minimum real multi-node runtime shape. In this
mode each node is a privileged `docker:dind` container that starts its own
`dockerd` and then runs `orch-server` inside that same container. Workloads are
created inside the assigned node's nested Docker daemon, not in the host daemon.
The script creates a same-named Docker network inside each nested daemon so the
existing `.orch` manifests can be reused. Before deploy, it parses workload image
references from the manifest, saves them from the host Docker cache, and loads
them into each nested daemon to avoid repeated Docker Hub pulls during the test.

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
`node-b`. After the initial deploy succeeds, the smoke exercises the user-facing
operation chain:

- `migrate app placement-smoke --to <node> --workload whoami`
- `rebalance app placement-smoke --workload whoami`
- stop one non-leader orch-server container to simulate a worker/control-plane
  node outage
- `failover app placement-smoke --to <survivor> --workload whoami`

In `shared` mode, the placement smoke runs only the deploy/status/volume/ingress/delete chain. The user-facing movement chain is intentionally reserved for `-RuntimeIsolation dind`, where each node owns a separate Docker daemon. In DinD mode each step waits for the assignment and mounted volume binding to become `running`/`bound` on the expected node, then checks whoami through the live ingress ports. The stopped node also loses its nested Docker daemon, so the placement DinD smoke is the lightest starting point for real multi-node runtime validation.

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

The workload placements are spread across `node-a`, `node-b`, and `node-c`. The
test writes a small file through the filer HTTP API and reads it back from all
three ingress ports. This covers the master/volume/filer path, replicated
workload DNS records, and cross-node ingress forwarding.

## Direct Commands

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario placement
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario placement -RuntimeIsolation dind
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario nextcloud
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario seaweed
```

Use `-Manifest` to point at another `.orch` file. If you also change
`-NetworkName`, pass a matching manifest because the committed `.orch` files pin
`docker.network` to the default smoke network.

Use `-KeepCluster -KeepWorkload` to inspect a successful run before cleanup.

## Current Boundaries

These tests validate deployment, connectivity, and local runtime volume binding lifecycle. Docker runtime `mounts` now map
to local Docker named volumes, and cleanup waits for bindings to become `released`. In `dind` mode those named volumes are local to the
assigned node's nested daemon, which is useful for lifecycle isolation checks but
still does not validate cross-node shared storage, attach/detach orchestration,
or data migration.

Placement is intentionally lightweight and focuses on control-plane operations:
migrate, rebalance, and failover after a simulated non-leader node outage.
Nextcloud is intentionally pinned to one node so the smoke focuses on a common
single-owner web application shape. SeaweedFS is spread across all three orch
nodes and relies on replicated workload assignment addresses to let a single orch
DNS nameserver resolve workloads owned by other nodes.

## Planned Stateful Cluster Smokes

Kafka cluster smoke:

- Add `examples/integration/kafka.orch` after selecting the image/runtime shape.
- Prefer a KRaft-based image so the test does not need ZooKeeper.
- Deploy three brokers on the Docker runtime.
- Verify controller quorum is healthy.
- Create a topic with replication factor greater than one.
- Produce and consume a message through an internal client workload.
- Later add broker restart/failover checks.

Postgres HA smoke:

- Add `examples/integration/postgres-ha.orch` after selecting the HA stack.
- Prefer a maintained Patroni + etcd image set or pg_auto_failover image set.
- Deploy the coordinator layer and at least two Postgres nodes.
- Verify primary discovery and replica streaming.
- Write a row through the primary endpoint and read it back.
- Later add primary kill, promotion, and client reconnect checks.

These two should be separate smoke tasks from Nextcloud and SeaweedFS because
failure diagnostics and runtime cost are materially higher.
