# orch Workload DSL v1

> This repository is **orch**. Older prose may say “Warden” for the same control-plane lineage; configuration uses the **`ORCH`** environment prefix.

Status: Design baseline plus tracked implementation subset for the first
canonical workload DSL.

Chinese version: `dsl.zh.md`

## Current Go `.orch` Authoring Surface

The Go implementation in this repository currently uses the Plano-backed
`.orch` compiler under `internal/deploy/orch`. New examples should prefer the
short form:

```plano
app {
  name = "mall"
  namespace = "demo"

  docker {
    network = "orch-demo"
  }

  stateful postgres {
    image = "postgres:16-alpine"
    env = {
      POSTGRES_DB = "app",
    }

    tcp(5432)
    resources = "500m/512Mi"
  }

  service api {
    image = "ghcr.io/acme/api:latest"
    depends_on = [postgres]
    http(8080)
  }

  worker localJob {
    runtime = "process"
    command = ["/opt/app/job"]
    args = ["--once"]
  }

  ingress public {
    path "/" {
      workload = api
    }
  }
}
```

The short `.orch` authoring fields lower into the runtime-neutral canonical
shape:

```yaml
run:
  artifact:
    image: ghcr.io/acme/api:latest
  exec:
    command: ["/app/server"]
    args: ["--listen", ":8080"]
```

For non-container runtimes, use `run.artifact.path` or `run.exec.command`
rather than a container image. The verbose `workload { run { ... } endpoint {
... } }` shape remains supported as an escape hatch. The sections below are
the longer-term DSL direction and historical design context; the full-stack
example documents the current recommended Go `.orch` style.

This document defines the intended v1 direction for the Warden workload DSL.
It replaces the earlier open-ended design draft as the primary reference for
future parser, planner, and apply work.

## Design Goals

Warden is a deploy system centered on long-lived workloads. The DSL therefore
models workload intent first, not backend-native objects.

The v1 DSL should provide:

- A canonical, Warden-native deployment model.
- Strong typing for cross-object references.
- A Gradle/KTS-inspired authoring style without becoming a scripting language.
- A compile-time pipeline suitable for parse, bind, validate, plan, diff, and
  apply.
- Compatibility importers for external formats such as Kubernetes YAML and
  Docker Compose.

The v1 DSL should not attempt to be:

- A generic infrastructure provisioning language.
- A free-form programming language.
- A thin text templating layer over Kubernetes manifests.

## Core Architecture

Warden should have one canonical model for deployment intent.

All authoring and import paths lower into the same canonical model:

```text
Warden DSL -----------+
                      |
Kubernetes importer --+--> Canonical Model --> Plan IR --> Apply / Runtime Lowering
                      |
Compose importer -----+
```

This is the key boundary:

- Warden DSL is the primary authoring surface.
- Kubernetes YAML and Docker Compose are compatibility inputs.
- Plan, diff, apply, and runtime lowering operate only on the canonical model.

## Canonical Object Model

The first-class object is `workload`.

Top-level objects in v1:

- `app`
- `workload`
- `config`
- `secret`
- `volume`
- `ingress`

Recommended canonical shape:

```text
App
- metadata
  - name
  - namespace
  - labels
  - annotations
- workloads[]
- configs[]
- secrets[]
- volumes[]
- ingresses[]
```

Each workload carries Warden-native deploy intent:

```text
Workload
- name
- kind
  - service | worker | job | cron | stateful
- runtime
  - docker | podman | containerd | firecracker | process | systemd | windows-service
- run
  - artifact
    - image
    - path
    - url
  - exec
    - command[]
    - args[]
  - env[]
  - cwd
  - user
  - runtime_options
- replicas
- depends_on[]
- endpoints[]
  - name
  - port
  - protocol
- mounts[]
  - volume_ref
  - target
  - read_only
- resources
  - cpu
  - memory
- health
  - readiness
  - liveness
  - startup
- lifecycle
  - restart_policy
  - max_restarts
  - restart_delay
- scheduling
  - stateful
  - allow_leader
  - preferred_nodes[]
- rollout
  - strategy
  - max_unavailable
  - max_surge
  - rollback_on_failure
  - progress_deadline
```

In YAML, the current canonical fields are `rollout.rollbackOnFailure` and
`rollout.progressDeadline`. `progressDeadline` uses Go duration syntax such as
`30s` or `2m`; when `rollbackOnFailure` is true, a failed rollout restores the
previous desired app revision if one exists.

### Model Boundaries

The canonical model should express platform-level deployment semantics, not
every backend-native flag directly.

Backend-specific details should be isolated under runtime-specific options, for
example:

```text
runtime_options.firecracker
runtime_options.containerd
runtime_options.docker
runtime_options.podman
runtime_options.process
runtime_options.systemd
runtime_options.windowsService
```

This keeps the main DSL stable even when individual runtime adapters evolve.

Current provider coverage:

- `docker`, `podman`, `containerd`, and `process` are deployable runtime
  providers.
- `firecracker` starts Linux microVMs by launching the local `firecracker`
  binary and configuring its API socket with kernel/rootfs/machine settings.
  Optional TAP networking is supported when the host tap/bridge already exists.
- `systemd` deploys Linux system units from `run.exec` / `run.artifact.path`.
- `windows-service` registers Windows services from `run.exec` / `run.artifact.path`;
  the target executable must currently be service-aware.
- Firecracker jailer support and automatic tap/bridge management are not wired yet.

## Syntax Style

The v1 surface syntax should follow a Gradle/KTS-like builder style:

- Call-style configuration such as `runtime(containerd)` and `replicas(3)`.
- Scoped blocks such as `resources { ... }` and `env { ... }`.
- Named object declarations such as `workload("gateway") { ... }`.
- Typed accessors such as `workloads.redis`.

The DSL should not use assignment-style fields as the primary style in v1.

Preferred:

```text
runtime(containerd)
replicas(3)
port(8080)
```

Not preferred for v1:

```text
runtime = containerd
replicas = 3
port = 8080
```

This keeps the first implementation smaller and the binder rules clearer.

## Typed References and Scope

Strongly typed cross-object references are a core requirement.

Built-in reference namespaces in v1:

- `workloads.<name>`
- `configs.<name>`
- `secrets.<name>`
- `volumes.<name>`
- `ingresses.<name>`

Important reference types:

- `WorkloadRef`
- `EndpointRef`
- `ConfigRef`
- `SecretRef`
- `VolumeRef`
- `IngressRef`

Examples:

```text
workloads.redis
workloads.gateway.endpoint("http")
volumes.redisData
secrets.dbPassword
```

Invocation constraints should be type-driven:

- `dependsOn(...)` accepts only `WorkloadRef`.
- `backend(...)` accepts only `EndpointRef`.
- `mount(...)` accepts only `VolumeRef`.
- `env.set(...)` may accept `String`, `ConfigRef`, `SecretRef`, or `EndpointRef`.

Stringly-typed cross-object references should be rejected in the canonical DSL.

Preferred:

```text
dependsOn(workloads.redis)
backend(workloads.gateway.endpoint("http"))
mount(volumes.redisData, "/data")
```

Not preferred:

```text
dependsOn("redis")
backend("gateway:http")
mount("redis-data", "/data")
```

### Scope Rules

- A top-level declaration is visible through its namespace accessor.
- `endpoint("http")` inside a workload creates a local endpoint object.
- A local endpoint may be referenced inside the same workload by
  `endpoint("http")`.
- Cross-workload endpoint references must be explicit:
  `workloads.gateway.endpoint("http")`.

## Ingress and DSL Boundary

Ingress should stay aligned with the same typed-reference model as the rest of
the DSL.

The important boundary is:

```text
workload endpoint declaration
  -> canonical ingress object with EndpointRef backend
  -> ingress runtime route snapshot
  -> resolved healthy backend addresses at runtime
```

This means:

- DSL authoring expresses which workload endpoint should be published.
- The canonical ingress object stores that intent as `EndpointRef`.
- The ingress runtime later resolves that `EndpointRef` into concrete backend
  candidates from endpoint state.
- Raw socket addresses and node-local backend strings are runtime data, not DSL
  data.

### Canonical Ingress Backend Type

In the canonical model, `route.backend` should be typed as `EndpointRef`, not
`String`.

Recommended canonical shape:

```text
Ingress
- name
- host
- routes[]
  - path
  - backend: EndpointRef
```

Preferred DSL form:

```text
ingress("public") {
  route("/") {
    backend(workloads.api.endpoint("http"))
  }
}
```

Not acceptable as canonical DSL:

```text
ingress("public") {
  route("/") {
    backend("10.0.0.5:8080")
  }
}
```

If an address string appears in the current runtime path, it should be treated
as transitional runtime data only, not as the long-term canonical interface.

### Workload and Ingress Relationship

`workload` owns endpoint declaration.

`ingress` owns external publishing intent.

That division should stay stable:

- `workload` declares `endpoint("http") { ... }`
- `ingress` references `workloads.api.endpoint("http")`
- runtime routing resolves that endpoint reference later

This avoids mixing author intent with ephemeral runtime placement and addresses.

### Future Workload-Local Sugar

Authoring sugar may later allow a workload-local publish block, for example:

```kotlin
workload("api") {
  endpoint("http") {
    port(8080)
    protocol(http)
  }

  publish("public") {
    host("api.example.com")
    path("/")
    endpoint("http")
  }
}
```

But that should lower to the same canonical object:

```kotlin
ingress("public") {
  host("api.example.com")
  route("/") {
    backend(workloads.api.endpoint("http"))
  }
}
```

So the sugar is optional authoring syntax only. The canonical control-plane
object remains top-level `ingress`.

## Imports

The v1 DSL should support imports, but only in a controlled form.

Supported form:

```text
import("./modules/redis.wd")
```

v1 import rules:

- The argument must be a static string literal.
- Paths are local filesystem paths relative to the importing file.
- Imports are resolved at compile time.
- Circular imports must be detected and rejected.
- Remote URLs are not supported.
- Globs are not supported.
- Environment-variable path construction is not supported.

### Module Shape

To keep composition simple, imported files in v1 should be fragments rather than
full applications.

Main file:

```kotlin
app("mall") {
  import("./modules/redis.wd")
  import("./modules/gateway.wd")
}
```

Imported fragment:

```kotlin
volume("redis-data") {
  persistent(true)
  size(20.gibi)
}

workload("redis") {
  kind(stateful)
  runtime(containerd)
  image("redis:7")
}
```

An imported fragment should not declare another `app(...)`.

## Expressions

The DSL should support simple expression evaluation, but only as a
compile-time-evaluable subset.

v1 expression support:

- String literals
- Integer literals
- Boolean literals
- Unit literals such as `500.milliCpu`, `512.mebi`, `30.seconds`
- `let` constants
- Typed refs
- String interpolation
- Simple conditional expressions
- Basic arithmetic on numeric values
- Basic comparison and boolean operators

Examples:

```text
let env = "prod"
let version = "1.2.3"
let replicas = if env == "prod" then 3 else 1
let port = 8000 + 80
```

Recommended supported operators in v1:

- `+`
- `-`
- `*`
- `/`
- `==`
- `!=`
- `>`
- `>=`
- `<`
- `<=`
- `&&`
- `||`
- `!`

Explicit non-goals for v1:

- User-defined functions
- Loops
- Mutable variables
- General recursion
- Runtime-evaluated expressions
- Embedding a full scripting engine in the parser layer

All expressions must be statically evaluable before planning and applying.

## Minimal v1 Surface

The initial top-level grammar should stay intentionally small:

- `app("name") { ... }`
- `workload("name") { ... }`
- `config("name") { ... }`
- `secret("name") { ... }`
- `volume("name") { ... }`
- `ingress("name") { ... }`
- `let name = expr`
- `import("relative/path.wd")`

The first implementation should not require support for:

- `profile(...)`
- `capability(...)`
- `policy(...)`
- `hook(...)`
- User-defined functions

These may be added later if the canonical model demonstrates a real need.

## Example

```kotlin
app("mall") {
  let env = "prod"
  let version = "1.2.3"
  let gatewayReplicas = if env == "prod" then 3 else 1

  import("./modules/redis.wd")

  workload("gateway") {
    kind(service)
    runtime(containerd)
    image("ghcr.io/acme/gateway:${version}")
    replicas(gatewayReplicas)

    dependsOn(workloads.redis)

    env {
      set("REDIS_ADDR", workloads.redis.endpoint("redis"))
    }

    endpoint("http") {
      port(8080)
      protocol(http)
    }

    resources {
      cpu(500.milliCpu)
      memory(512.mebi)
    }

    health {
      readiness {
        http("/health", endpoint("http"))
      }
    }
  }

  ingress("public") {
    host("mall.example.com")
    route("/") {
      backend(workloads.gateway.endpoint("http"))
    }
  }
}
```

## Compatibility Layers

Compatibility support should exist, but outside the main DSL grammar.

### Kubernetes

The Kubernetes importer should map common workload-oriented objects into the
canonical model, for example:

- `Deployment` -> `workload(kind = service)`
- `StatefulSet` -> `workload(kind = stateful)`
- `DaemonSet` -> `workload(kind = worker or daemon-like extension later)`
- `Job` -> `workload(kind = job)`
- `CronJob` -> `workload(kind = cron)`
- `Service` and `Ingress` -> endpoint and ingress structures
- `ConfigMap` -> `config`
- `Secret` -> `secret`
- `PersistentVolumeClaim` -> `volume`

### Docker Compose

The Compose importer should map:

- `services.*` -> `workload`
- `environment` -> `env`
- `ports` -> endpoints and exposure
- `depends_on` -> `dependsOn`
- `volumes` -> mounts and volumes
- `networks` -> network attachment when that model exists
- `healthcheck` -> health
- `deploy.resources` -> resources

### Importer Output Requirements

Importers should never silently pretend to be lossless.

Each importer should be able to report:

- Fully mapped fields
- Lossy mappings
- Ignored fields
- Unsupported fields

This diagnostic layer is required to make compatibility trustworthy.

## Implementation Guidance

The implementation should continue to follow a staged compiler pipeline:

```text
source -> parse -> bind -> type check -> canonical model -> plan -> apply
```

Recommended responsibilities:

- Parser: syntax only
- Binder: symbol resolution and imports
- Type checker: invocation signatures and ref typing
- Canonical lowerer: Warden-native deployment model
- Planner/apply layer: plan and execution based only on the canonical model

## Status Against Current Repo

The current repository still contains transitional YAML manifest support and a
limited invocation-style parser. Those paths should be treated as compatibility
or migration layers while the canonical workload DSL is implemented.

On the older manifest-based `/dsl/apply` path, the implementation now also
compiles explicit ingress route specs and reconciles route/DNS records after
workload deploy so that ingress control-plane intent is no longer only a side
effect of `task.deploy`.

## Current Implemented Subset

Snapshot date: March 26, 2026

The current repository does not yet implement the full v1 surface described
above. What does exist now is a real, compiler-style pipeline for a growing
canonical subset:

```text
source file
  -> parser
  -> import expansion
  -> HIR
  -> binder
  -> IR
  -> canonical model
  -> canonical apply object
  -> planner output
```

The main crates on that path are:

- `warden-dsl-ast`
- `warden-dsl-parser`
- `warden-dsl-hir`
- `warden-dsl-binder`
- `warden-dsl-ir`
- `warden-dsl-canonical`
- `warden-dsl-planner`

### What Is Implemented Today

Top-level declarations currently recognized by the canonical path:

- `app("name") { ... }`
- `workload("name") { ... }`
- `volume("name") { ... }`
- `config("name") { ... }`
- `secret("name") { ... }`
- `ingress("name") { ... }`
- `let name = expr`
- `import("relative/path.wd")`

Current import support:

- Static string literal imports only
- Local relative filesystem paths only
- Imported files are fragments, not nested `app(...)`
- Recursive import expansion
- Import-cycle detection

Current expression support on the implemented path:

- String literals
- Integer literals
- Unit-like member numbers such as `500.milliCpu`, `512.mebi`
- Simple `if env == "prod" then 3 else 1`
- Path refs such as `workloads.redis`
- Invocation-style refs such as `workloads.redis.endpoint("redis")`
- String interpolation for image strings in canonical lowering

Current workload fields supported end-to-end:

- `kind(...)`
- `runtime(...)`
- `image(...)`
- `replicas(...)`
- `dependsOn(...)`
- `endpoint("name") { port(...) protocol(...) host_port(...) host_ip(...) }`
- `mount(volumes.xxx, "/path")`
- `env { set("KEY", value) }`
- `resources { cpu(...) memory(...) }`
- `health { readiness/liveness/startup { http/path/tcp endpoint timing fields } }`

Current typed refs supported end-to-end:

- `WorkloadRef`
  Example: `dependsOn(workloads.redis)`
- `EndpointRef`
  Example: `backend(workloads.gateway.endpoint("http"))`
- `VolumeRef`
  Example: `mount(volumes.redisData, "/data")`
- `ConfigRef`
  Example: `set("APP_CONFIG", configs.appConfig)`
- `SecretRef`
  Example: `set("DB_PASSWORD", secrets.dbPassword)`

Current `env.set(...)` value forms supported end-to-end:

- String literal
- `ConfigRef`
- `SecretRef`
- `EndpointRef`

Current ingress forms supported:

- Preferred: `backend(workloads.gateway.endpoint("http"))`
- Transitional compatibility: `backend(workloads.gateway)` plus `port("http")`
- Ingress backend refs are now carried as typed endpoint references through
  binder, IR, and canonical lowering rather than only being reconstructed at
  the final lowering step

Endpoint publishing for Docker-compatible runtimes:

- `port` is always the workload/container port.
- Optional `host_port` publishes that endpoint on the host, for example
  `endpoint mysql { port = 3306 protocol = "tcp" host_port = 3306 }`.
- Shorthand endpoint calls also accept publish args:
  `tcp(5432, "postgres", 5432)` and `http(80, "console", 9001)`.
Current canonical normalization already done by the implementation:

- Workload kind normalization to `service | worker | job | cron | stateful`
- Runtime normalization to `docker | containerd | firecracker | process |
  systemd | windows-service`
- Endpoint protocol normalization to `tcp | udp | http`
- CPU normalization to `cpu_millis`
- Memory normalization to `memory_bytes`

Current planner/apply handoff support:

- `dsl planner` output now includes a canonical apply object
- ingress routes are compiled there as explicit route specs rather than being
  observable only as legacy deploy side effects

### Current Limitations

The implementation is still intentionally narrower than the target design.

Known gaps and limits as of March 26, 2026:

- The parser is still a restricted invocation-style language, not a Kotlin
  interpreter or a general scripting runtime.
- Expression support is still minimal; arithmetic, boolean operators, and more
  general compile-time evaluation are not fully implemented.
- The binder and canonical lowerer still treat many fields as a small,
  statically-known subset rather than a full signature/type system.
- `config("name")` and `secret("name")` currently model identity and refs, but
  not full key/value payload semantics yet.
- `volume("name")` currently models identity and `mount(...)` refs, but not the
  full persistent/ephemeral/size policy surface yet.
- `resources` currently only supports CPU and memory.
- `health` supports HTTP and TCP probes in canonical manifests and short `.orch` field blocks. The runtime currently enforces `readiness` as the deploy dependency gate; background liveness/startup controllers are not wired yet.
- `lifecycle.restart_policy/max_restarts/restart_delay` is available in the canonical spec and `.orch` lowering for restart/failure-recovery controllers; runtime enforcement is not wired yet.
- The current lowering path still mostly behaves as if each workload has one
  primary endpoint. Multi-endpoint modeling is the next important structural
  step.
- `workload` is already the primary top-level authoring object, but the legacy
  `services { val x = create(...) { ... } }` shape is still supported as a
  compatibility path.
- Planner output exposes both transitional layers (`hir`, `ir`) and the newer
  canonical model because the migration is still in progress.
- `dsl plan/render/apply/delete` CLI flow is still partly tied to the older
  manifest path; `dsl planner` is the main entry for inspecting the canonical
  compiler pipeline.

### Recommended Authoring Style Right Now

If writing new DSL files against the current implementation, prefer this style:

```kotlin
app("mall") {
  import("./modules/redis.wd")

  config("appConfig") {}
  secret("dbPassword") {}
  volume("redisData") {}

  workload("gateway") {
    kind(worker)
    runtime(containerd)
    image("ghcr.io/acme/gateway:1.2.3")
    replicas(3)

    dependsOn(workloads.redis)
    mount(volumes.redisData, "/data")

    env {
      set("APP_CONFIG", configs.appConfig)
      set("DB_PASSWORD", secrets.dbPassword)
      set("REDIS_ADDR", workloads.redis.endpoint("redis"))
    }

    endpoint("http") {
      port(8080)
      protocol(http)
    }

    resources {
      cpu(500.milliCpu)
      memory(512.mebi)
    }

    health {
      readiness { http("/ready", endpoint("http")) }
    }
  }

  ingress("public") {
    route("/") {
      backend(workloads.gateway.endpoint("http"))
    }
  }
}
```

Until the implementation fully catches up, this document should be read in two
layers:

- The sections above define the intended v1 direction.
- The sections in `Current Implemented Subset` define what is actually safe to
  rely on today.
