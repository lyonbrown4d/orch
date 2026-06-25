# Docker Systemd Package Smoke

This smoke test verifies the Linux package installation path inside a privileged
Docker container that runs systemd as PID 1:

```text
GoReleaser snapshot -> .deb package -> apt install -> systemctl enable --now orch-server -> orch ready -> systemd workload deploy/delete
```

It is intentionally narrower than the Vagrant/VM smoke. It is fast enough for a
local packaging check and CI environments that allow privileged Docker, but it
is not a full host integration test for resolver behavior, kernel networking, or
real systemd-resolved state.

## Prerequisites

- Docker CLI and daemon.
- Privileged containers enabled.
- A host that can run systemd in Docker with a writable cgroup mount.
- Go toolchain, when the script needs to build a fresh GoReleaser snapshot.

The script defaults to Debian 12 and `linux/amd64` packages.

## Run

```bash
task smoke:systemd-docker
```

Equivalent direct command:

```bash
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/systemd-docker-smoke.ps1
```

Useful options:

```bash
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/systemd-docker-smoke.ps1 \
  -SkipSnapshot \
  -PackagePath dist/orch_linux_amd64.deb \
  -KeepContainer
```

## What It Checks

1. Builds the systemd smoke image from `scripts/systemd/Dockerfile.debian`.
2. Builds a GoReleaser snapshot unless `-SkipSnapshot` or `-PackagePath` is used.
3. Starts a privileged Debian container with systemd as PID 1.
4. Installs the `.deb` package through `apt-get install`.
5. Verifies the package installed:
   - `/usr/bin/orch-server`
   - `/usr/bin/orch`
   - `/usr/lib/systemd/system/orch-server.service`
   - `/etc/orch/env`
6. Starts the service with `systemctl enable --now orch-server.service`.
7. Waits for `orch ready`.
8. Deploys a small `systemd` runtime workload, inspects app/workload status and
   logs, then deletes the app.

## Limits

This smoke does not replace the VM e2e test. In particular, it does not fully
prove host DNS installer behavior, host Docker service interaction, or distro
specific boot behavior. Keep Vagrant/VM tests as the release gate for those.
