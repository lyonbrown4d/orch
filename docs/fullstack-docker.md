# Full-Stack Docker Example

This page shows how a complete application can be expressed with the short
native `.orch` DSL and deployed through the current `docker` provider.

The example manifest is:

```text
examples/fullstack-docker.orch
```

It models four workloads:

| Workload | Kind | Runtime | Role |
|----------|------|---------|------|
| `frontend` | `service` | `docker` | Public web UI |
| `backend` | `service` | `docker` | HTTP API |
| `postgres` | `stateful` | `docker` | Database |
| `redis` | `stateful` | `docker` | Cache / queue |

## Validate The Manifest

```bash
go run ./cmd/orch-cli validate --file examples/fullstack-docker.orch
go run ./cmd/orch-cli parse --file examples/fullstack-docker.orch --json
```

The DSL lowers to the canonical `internal/deploy/v1alpha1.App` model. The
server receives the same model whether the source is `.orch` or YAML.

## Deploy Flow

Create the Docker network used by the workloads:

```bash
docker network create orch-demo
```

Start the server. If you want ingress without binding privileged ports, use a
high port:

```bash
go run ./cmd/orch-server --ingress-listen :18080
```

Wait until the control plane has elected a Raft leader and can accept writes:

```bash
go run ./cmd/orch-cli ready --wait --timeout 30s
```

To let Docker workloads resolve orch service names through the internal DNS,
run the DNS listener on port 53 at an address reachable from the Docker network
and pass that same IP as the workload nameserver. For the default Linux bridge,
that is often the bridge gateway:

```bash
go run ./cmd/orch-server \
  --ingress-listen :18080 \
  --dns-listen 0.0.0.0:53 \
  --dns-workload-nameserver 172.17.0.1 \
  --dns-workload-upstream 1.1.1.1 \
  --dns-workload-advertise-address 172.17.0.1
```

With those flags, Docker containers get `dns.workload.nameserver` and search
domains like `demo.svc.orch.local`, `svc.orch.local`, and `orch.local` through
Docker's native DNS settings. No `/etc/resolv.conf` bind mount is added.
Workloads still see a single resolver: orch DNS answers names in the orch zone
and forwards non-orch names to `dns.workload.upstream`.

Deploy and wait:

```bash
go run ./cmd/orch-cli apply --file examples/fullstack-docker.orch --watch
go run ./cmd/orch-cli wait app fullstack -n demo --for running --timeout 2m
go run ./cmd/orch-cli get workloads
go run ./cmd/orch-cli get assignments
go run ./cmd/orch-cli describe workload backend --app fullstack -n demo
go run ./cmd/orch-cli logs backend --app fullstack -n demo --tail 100
go run ./cmd/orch-cli events
```

Replace these placeholder images before running a real deploy:

```text
ghcr.io/acme/fullstack-backend:latest
ghcr.io/acme/fullstack-frontend:latest
```

## Compact DSL Shape

The example uses the short authoring form. App-level Docker defaults are
inherited by shorthand workloads, `service` and `stateful` imply workload kind,
`env = { ... }` replaces repeated env blocks, and `http(...)` / `tcp(...)`
declare endpoints:

```plano
app {
  name = "fullstack"
  namespace = "demo"

  docker {
    network = "orch-demo"
  }

  volume postgresData {
    persistent = true
  }

  volume redisData {
    persistent = true
  }

  stateful postgres {
    image = "postgres:16-alpine"
    env = {
      POSTGRES_DB = "app",
      POSTGRES_USER = "orch",
      POSTGRES_PASSWORD = "orch-dev-password",
    }

    mount {
      volume = "postgresData"
      target = "/var/lib/postgresql/data"
    }

    tcp(5432)
    resources = "500m/512Mi"
  }

  service backend {
    image = "ghcr.io/acme/fullstack-backend:latest"
    depends_on = [postgres, redis]
    env = {
      DATABASE_URL = "postgres://orch:orch-dev-password@orch-demo-postgres:5432/app?sslmode=disable",
      REDIS_ADDR = "orch-demo-redis:6379",
      HTTP_ADDR = ":8080",
    }

    http(8080)
    resources = "500m/512Mi"
  }
}
```

The full manifest also defines `redis`, `frontend`, and ingress. The lowerer
turns the short form into the same canonical deploy model as the verbose form:

```text
service backend {
  image = "..."
  depends_on = [postgres, redis]
  env = { HTTP_ADDR = ":8080" }
  http(8080)
  resources = "500m/512Mi"
}
```

Ingress can use the compact `path` form. If the workload has exactly one HTTP
endpoint, the endpoint name is inferred:

```plano
ingress public {
  path "/api" {
    workload = backend
  }

  path "/" {
    workload = frontend
  }
}
```

Use `endpoint = "name"` inside `path` when the workload has multiple HTTP
endpoints or when routing to a non-default endpoint name.

The intended rule for new `.orch` files is to keep the authoring surface
application-first:

- Put cross-workload topology at the workload level with `depends_on`.
- Put runtime-wide defaults once at the app level, for example `docker.network`.
- Use compact helpers like `http(8080)`, `tcp(5432)`, `resources = "500m/512Mi"`,
  and map-style `env = { ... }`.
- Drop to the verbose canonical `workload { run { ... } }` form only when a
  runtime-specific option has no compact helper yet.

## Current Docker Provider Boundaries

The manifest intentionally stays inside the current runtime surface:

- Supported by the `docker` provider now: image pull, command, args, env, cwd,
  resource limits, Docker network mode, labels, `privileged`, Docker-native DNS
  nameserver/search injection, and workload `mounts` backed by local Docker
  named volumes.
- Workload DNS is platform-managed. Configure `dns.workload.upstream` when
  workloads also need non-orch DNS names, instead of adding per-workload
  resolver settings.
- `depends_on` is a scheduling/deploy graph signal today; it is not yet a
  readiness gate.
- Workload `endpoint` entries feed DNS/ingress intent; they do not publish host
  ports directly.
- Workload `mounts` are local runtime mounts. They persist on the node/Docker
  daemon that runs the workload, but orch does not yet provide cross-node shared
  volumes, attach/detach orchestration, or data migration.
- The verbose `workload { run { ... } endpoint { ... } }` form still works as
  an escape hatch when the short form is not expressive enough.

For a smoke test that runs without placeholder app images, use
`examples/local-docker-smoke.yaml` and `task smoke:local-docker`.
