# Docker Raft App Smoke Tests

These smoke tests verify heavier application deployments on the Docker runtime:

```text
3 orch-server containers -> Dragonboat Raft membership -> orch-cli apply --file examples/integration/<app>.orch -> worker dispatch -> Docker runtime -> orch DNS -> built-in ingress -> application containers
```

The workload definitions are committed as native `.orch` DSL files under
`examples/integration/`. The script does not generate deploy YAML; it starts the
test cluster, calls the CLI with the workload file, waits for status, and then
runs HTTP checks through ingress.

The scripts currently mount the host Docker socket into the control-plane
containers. This keeps the tests reproducible on a local Docker engine without
adding Docker-in-Docker startup cost. A future DinD variant can reuse the same
`.orch` files when we want a fully nested Docker daemon per test node.

## Scenarios

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
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario nextcloud
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-app-smoke.ps1 -Scenario seaweed
```

Use `-Manifest` to point at another `.orch` file. If you also change
`-NetworkName`, pass a matching manifest because the committed `.orch` files pin
`docker.network` to the default smoke network.

Use `-KeepCluster -KeepWorkload` to inspect a successful run before cleanup.

## Current Boundaries

These tests validate deployment and connectivity. They do not yet validate
persistent data recovery because the Docker provider has not wired deploy
`mounts` and `volumes` into Docker HostConfig mounts.

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