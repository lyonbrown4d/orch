# Docker Raft Ingress Smoke Test

This smoke test verifies the container-oriented end-to-end path:

```text
3 orch-server containers -> Dragonboat Raft membership -> CLI deploy -> worker dispatch -> Docker runtime -> built-in ingress -> whoami container
```

## Prerequisites

- Go toolchain available on `PATH`
- PowerShell (`pwsh`)
- Docker CLI available on `PATH`
- A running Docker engine
- Available host API ports `18101`, `18102`, `18103`
- Available host ingress ports `18080`, `18081`, `18082`

## Run

```powershell
task smoke:docker-raft-ingress
```

Equivalent direct command:

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-ingress-smoke.ps1
```

The script builds a host `orch` CLI, cross-builds a Linux `orch-server`, builds a
small Debian-based control-plane image, creates a Docker network, and starts
three `orch-server` containers named `orch-raft-ingress-node-a`,
`orch-raft-ingress-node-b`, and `orch-raft-ingress-node-c`.

After the Raft cluster is ready, the script chooses the current leader and then
selects a non-leader target node. It generates a whoami deploy manifest with
`runtimeOptions.docker.networkMode` set to the smoke network, applies it through
the leader, waits for the assignment to run on the target node, and then sends an
HTTP request to the target node's mapped ingress port.

By default the script deletes the app, removes the whoami container, removes the
three control-plane containers, and removes the smoke Docker network.

## Keep The Environment Running

Use this when you want to inspect the cluster after the smoke succeeds:

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/docker-raft-ingress-smoke.ps1 -KeepCluster -KeepWorkload
```

Then inspect:

```powershell
.orch-docker-raft-ingress/bin/orch --server http://127.0.0.1:18101 raft status
.orch-docker-raft-ingress/bin/orch --server http://127.0.0.1:18101 get apps
.orch-docker-raft-ingress/bin/orch --server http://127.0.0.1:18101 get assignments
curl http://127.0.0.1:18080/
curl http://127.0.0.1:18081/
curl http://127.0.0.1:18082/
```

Cleanup:

```powershell
docker rm -f orch-default-whoami orch-raft-ingress-node-a orch-raft-ingress-node-b orch-raft-ingress-node-c
docker network rm orch-raft-ingress-smoke
```

## Notes

The smoke deploys the workload to one node, then verifies every node's ingress
listener can reach it. Non-owner nodes compile the route to the assigned node's
ingress listener, while the owner node routes directly to the workload DNS A
record.
