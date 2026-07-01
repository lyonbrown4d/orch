# orch Workload DSL v1（中文）

> 本仓库为 **orch**。文中出现的 “Warden” 多为历史产品线命名；运行时配置使用 **`ORCH`** 环境变量前缀。

状态：第一版 canonical workload DSL 的设计基线与当前实现子集跟踪。

对应英文版：`dsl.md`

## 当前 Go `.orch` 作者界面

当前仓库里的 Go 实现使用 `internal/deploy/orch` 下基于 Plano 的 `.orch`
compiler。新示例优先使用短写法：

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

短 `.orch` 作者字段会 lower 到 runtime-neutral canonical 结构：

```yaml
run:
  artifact:
    image: ghcr.io/acme/api:latest
  exec:
    command: ["/app/server"]
    args: ["--listen", ":8080"]
```

非容器 runtime 使用 `run.artifact.path` 或 `run.exec.command`，而不是把
本地可执行文件塞进 image。更完整的 `workload { run { ... } endpoint { ... } }`
写法仍然保留，作为短写法不够表达时的 escape hatch。下面的大部分内容是更长期的
DSL 方向和历史设计上下文；当前推荐的 Go `.orch` 写法以 full-stack 示例文档为准。

本文档用于说明 Warden Workload DSL 的 v1 方向，同时记录当前仓库里
已经真实实现的子集与限制，避免“设计稿”和“可用能力”混在一起。

## 设计目标

Warden 是一个围绕长生命周期 workload 的部署系统，因此 DSL 的一等抽象
应当首先表达 workload 意图，而不是直接暴露 backend 原生对象。

v1 DSL 应当提供：

- 一套 Warden-native 的 canonical deployment model
- 强类型的跨对象引用
- 借鉴 Gradle/KTS 体验，但不是脚本语言
- 适合 `parse -> bind -> validate -> plan -> apply` 的编译期流水线
- 面向 Kubernetes YAML / Docker Compose 的兼容 importer

v1 DSL 不应当尝试变成：

- 通用基础设施编排语言
- 任意脚本语言
- 套在 Kubernetes manifest 外面的一层纯文本模板

## 核心架构

Warden 应当只有一套 canonical deployment model。

所有作者输入和兼容输入最终都 lower 到同一套 canonical model：

```text
Warden DSL -----------+
                      |
Kubernetes importer --+--> Canonical Model --> Plan IR --> Apply / Runtime Lowering
                      |
Compose importer -----+
```

关键边界是：

- Warden DSL 是主 authoring surface
- Kubernetes YAML 和 Docker Compose 是 compatibility inputs
- `plan/diff/apply/runtime lowering` 只面对 canonical model

## Canonical 对象模型

一等对象是 `workload`。

v1 顶层对象：

- `app`
- `workload`
- `config`
- `secret`
- `volume`
- `ingress`

推荐 canonical 结构：

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

每个 workload 表达的是 Warden-native deploy intent：

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

当前 YAML canonical 字段是 `rollout.rollbackOnFailure` 和
`rollout.progressDeadline`。`progressDeadline` 使用 Go duration 写法，例如
`30s` 或 `2m`；当 `rollbackOnFailure` 为 true 时，rollout 失败会在存在上一版
revision 时恢复上一版 desired app。

### 模型边界

canonical model 应当表达平台层的部署语义，而不是直接暴露每个 backend
的全部原生参数。

backend-specific 细节应当隔离在 runtime-specific options 下，例如：

```text
runtime_options.firecracker
runtime_options.containerd
runtime_options.docker
runtime_options.podman
runtime_options.process
runtime_options.systemd
runtime_options.windowsService
```

这样即使 runtime adapter 迭代，主 DSL 也能保持稳定。

当前 provider 覆盖：

- `docker`、`podman`、`containerd`、`process` 已经是可 deploy 的 runtime provider。
- `firecracker` 会启动本机 `firecracker` 二进制，并通过 API socket 配置
  kernel/rootfs/machine settings 来运行 Linux microVM；当宿主机已经准备好
  tap/bridge 时，也可以配置 TAP 网络。
- `systemd` 会基于 `run.exec` / `run.artifact.path` 生成并启动 Linux system unit。
- `windows-service` 会基于 `run.exec` / `run.artifact.path` 注册 Windows Service；
  目前目标可执行文件需要自身支持 Windows Service 模式。
- Firecracker 的 jailer 和自动创建 tap/bridge 还没有接入。

## 语法风格

v1 采用类 Gradle/KTS 的 builder 风格：

- 调用式配置，例如 `runtime(containerd)`、`replicas(3)`
- block 式分组，例如 `resources { ... }`、`env { ... }`
- 命名对象声明，例如 `workload("gateway") { ... }`
- typed accessor，例如 `workloads.redis`

v1 不把赋值式字段写法作为主风格。

推荐：

```text
runtime(containerd)
replicas(3)
port(8080)
```

不推荐：

```text
runtime = containerd
replicas = 3
port = 8080
```

这样第一版实现更小，binder 规则也更清晰。

## 强类型引用与作用域

强类型跨对象引用是核心要求。

v1 内建引用 namespace：

- `workloads.<name>`
- `configs.<name>`
- `secrets.<name>`
- `volumes.<name>`
- `ingresses.<name>`

重要引用类型：

- `WorkloadRef`
- `EndpointRef`
- `ConfigRef`
- `SecretRef`
- `VolumeRef`
- `IngressRef`

示例：

```text
workloads.redis
workloads.gateway.endpoint("http")
volumes.redisData
secrets.dbPassword
```

调用约束应当由类型驱动：

- `dependsOn(...)` 只接受 `WorkloadRef`
- `backend(...)` 只接受 `EndpointRef`
- `mount(...)` 只接受 `VolumeRef`
- `env.set(...)` 可接受 `String | ConfigRef | SecretRef | EndpointRef`

canonical DSL 不应接受 stringly-typed cross-object refs。

推荐：

```text
dependsOn(workloads.redis)
backend(workloads.gateway.endpoint("http"))
mount(volumes.redisData, "/data")
```

不推荐：

```text
dependsOn("redis")
backend("gateway:http")
mount("redis-data", "/data")
```

### 作用域规则

- 顶层声明通过各自 namespace accessor 可见
- `endpoint("http")` 在 workload 内创建一个局部 endpoint 对象
- 同一 workload 内可通过 `endpoint("http")` 引用本地 endpoint
- 跨 workload endpoint 引用必须显式写成
  `workloads.gateway.endpoint("http")`

## Ingress 与 DSL 的边界

Ingress 应当和 DSL 里的强类型引用模型保持一致。

关键边界是：

```text
workload endpoint declaration
  -> canonical ingress object with EndpointRef backend
  -> ingress runtime route snapshot
  -> resolved healthy backend addresses at runtime
```

也就是说：

- DSL authoring 表达的是“哪个 workload endpoint 需要被发布”
- canonical ingress object 用 `EndpointRef` 保存这份意图
- ingress runtime 再根据 endpoint state 把 `EndpointRef` 解析成具体 backend candidates
- 原始 socket 地址和节点上的 backend 字符串属于 runtime data，不属于 DSL data

### Canonical Ingress Backend 类型

在 canonical model 中，`route.backend` 应当是 `EndpointRef`，而不是 `String`。

推荐 canonical 结构：

```text
Ingress
- name
- host
- routes[]
  - path
  - backend: EndpointRef
```

推荐 DSL 写法：

```text
ingress("public") {
  route("/") {
    backend(workloads.api.endpoint("http"))
  }
}
```

不应作为 canonical DSL 接受的写法：

```text
ingress("public") {
  route("/") {
    backend("10.0.0.5:8080")
  }
}
```

如果当前 runtime 路径里还出现地址字符串，那也只能被视为 transitional runtime data，
而不应成为长期 canonical interface。

### Workload 与 Ingress 的关系

`workload` 负责声明 endpoint。

`ingress` 负责表达对外发布意图。

这层分工应保持稳定：

- `workload` 声明 `endpoint("http") { ... }`
- `ingress` 引用 `workloads.api.endpoint("http")`
- runtime routing 再去解析这个 endpoint ref

这样 author intent 就不会和临时的 runtime placement / address 混在一起。

### 未来的 Workload-Local Sugar

未来 authoring 层可以允许 workload-local 的 publish sugar，例如：

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

但它应当 lower 到同一个 canonical 对象：

```kotlin
ingress("public") {
  host("api.example.com")
  route("/") {
    backend(workloads.api.endpoint("http"))
  }
}
```

所以 sugar 只是可选的 authoring 语法，canonical control-plane object 仍然是
顶层 `ingress`。

## Import

v1 支持受控 import。

支持形态：

```text
import("./modules/redis.wd")
```

v1 import 规则：

- 参数必须是静态字符串字面量
- 路径必须是相对当前文件的本地路径
- import 在编译期展开
- 必须检测并拒绝循环导入
- 不支持远程 URL
- 不支持 glob
- 不支持环境变量拼接路径

### 模块形态

为了让组合更简单，v1 中被导入文件应当是 fragment，而不是完整应用。

主文件：

```kotlin
app("mall") {
  import("./modules/redis.wd")
  import("./modules/gateway.wd")
}
```

导入 fragment：

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

被导入 fragment 不应再声明 `app(...)`。

## 表达式

DSL 可以支持简单表达式，但只限编译期可求值的子集。

v1 目标表达式能力：

- 字符串字面量
- 整数字面量
- 布尔字面量
- 单位字面量，例如 `500.milliCpu`、`512.mebi`、`30.seconds`
- `let` 常量
- typed refs
- 字符串插值
- 简单条件表达式
- 基础数值运算
- 基础比较与布尔运算

示例：

```text
let env = "prod"
let version = "1.2.3"
let replicas = if env == "prod" then 3 else 1
let port = 8000 + 80
```

推荐支持的操作符：

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

v1 明确不做：

- 用户自定义函数
- 循环
- 可变变量
- 一般递归
- 运行时求值表达式
- 在 parser 层嵌入完整脚本引擎

所有表达式都必须在 planning / applying 之前静态求值。

## 最小 v1 Surface

第一版顶层语法保持刻意收敛：

- `app("name") { ... }`
- `workload("name") { ... }`
- `config("name") { ... }`
- `secret("name") { ... }`
- `volume("name") { ... }`
- `ingress("name") { ... }`
- `let name = expr`
- `import("relative/path.wd")`

第一版不要求一开始就支持：

- `profile(...)`
- `capability(...)`
- `policy(...)`
- `hook(...)`
- 用户自定义函数

这些只有在 canonical model 证明确实需要时再加。

## 设计示例

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

## 兼容层

兼容支持应存在，但不直接混入主 DSL grammar。

### Kubernetes

Kubernetes importer 预期映射：

- `Deployment` -> `workload(kind = service)`
- `StatefulSet` -> `workload(kind = stateful)`
- `DaemonSet` -> `workload(kind = worker 或未来 daemon-like 扩展)`
- `Job` -> `workload(kind = job)`
- `CronJob` -> `workload(kind = cron)`
- `Service` / `Ingress` -> endpoint 与 ingress 结构
- `ConfigMap` -> `config`
- `Secret` -> `secret`
- `PersistentVolumeClaim` -> `volume`

### Docker Compose

Compose importer 预期映射：

- `services.*` -> `workload`
- `environment` -> `env`
- `ports` -> endpoint / exposure
- `depends_on` -> `dependsOn`
- `volumes` -> mount / volume
- `networks` -> 未来 network attachment
- `healthcheck` -> health
- `deploy.resources` -> resources

### Importer 输出要求

importer 不能假装自己是无损的。

每个 importer 至少应能报告：

- 完整映射的字段
- 有损映射的字段
- 被忽略的字段
- 不支持的字段

这是 compatibility 层可信的前提。

## 实现指导

实现仍应沿着 staged compiler pipeline 推进：

```text
source -> parse -> bind -> type check -> canonical model -> plan -> apply
```

推荐职责分层：

- Parser：只负责语法
- Binder：负责 import 与符号绑定
- Type checker：负责调用签名与 ref typing
- Canonical lowerer：负责 Warden-native deployment model
- Planner/apply：只消费 canonical model

## 与当前仓库的关系

仓库里仍然保留 transitional YAML manifest 支持和较老的 invocation-style
兼容路径，这些现在应被视为 compatibility / migration layer。

在较老的 manifest-based `/dsl/apply` 路径上，当前实现也已经开始编译显式
ingress route spec，并在 workload deploy 之后显式 reconcile route / DNS
records，因此 ingress control-plane intent 不再只是 `task.deploy` 的副作用。

## 当前已实现子集

快照日期：2026-03-26

当前仓库尚未实现上面描述的全部 v1 surface，但已经有一条真实可用的
compiler-style canonical pipeline：

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

主 crate 包括：

- `warden-dsl-ast`
- `warden-dsl-parser`
- `warden-dsl-hir`
- `warden-dsl-binder`
- `warden-dsl-ir`
- `warden-dsl-canonical`
- `warden-dsl-planner`

### 当前已实现内容

当前 canonical path 已识别的顶层声明：

- `app("name") { ... }`
- `workload("name") { ... }`
- `volume("name") { ... }`
- `config("name") { ... }`
- `secret("name") { ... }`
- `ingress("name") { ... }`
- `let name = expr`
- `import("relative/path.wd")`

当前 import 支持：

- 仅支持静态字符串字面量
- 仅支持本地相对路径
- 导入文件必须是 fragment，不能嵌套 `app(...)`
- 支持递归 import 展开
- 支持 import cycle 检测

当前实现路径上的表达式支持：

- 字符串字面量
- 整数字面量
- member-number 单位值，例如 `500.milliCpu`、`512.mebi`
- 简单 `if env == "prod" then 3 else 1`
- 路径引用，例如 `workloads.redis`
- invocation-style ref，例如 `workloads.redis.endpoint("redis")`
- image 字符串插值

当前 workload 已端到端支持的字段：

- `kind(...)`
- `runtime(...)`
- `image(...)`
- `replicas(...)`
- `dependsOn(...)`
- `endpoint("name") { port(...) protocol(...) }`
- `mount(volumes.xxx, "/path")`
- `env { set("KEY", value) }`
- `resources { cpu(...) memory(...) }`
- `health { readiness/liveness/startup { http/path/tcp endpoint timing fields } }`

当前已端到端支持的 typed ref：

- `WorkloadRef`
  例：`dependsOn(workloads.redis)`
- `EndpointRef`
  例：`backend(workloads.gateway.endpoint("http"))`
- `VolumeRef`
  例：`mount(volumes.redisData, "/data")`
- `ConfigRef`
  例：`set("APP_CONFIG", configs.appConfig)`
- `SecretRef`
  例：`set("DB_PASSWORD", secrets.dbPassword)`

当前 `env.set(...)` value 已支持：

- 字符串字面量
- `ConfigRef`
- `SecretRef`
- `EndpointRef`

当前 ingress 支持的两种形式：

- 推荐：`backend(workloads.gateway.endpoint("http"))`
- 兼容：`backend(workloads.gateway)` 加 `port("http")`
- ingress backend ref 现在已经在 binder、IR、canonical lowering 三层中都
  作为 typed endpoint reference 传递，而不是只在最终 lowering 时临时重建

当前 canonical normalization 已覆盖：

- workload kind 归一化为 `service | worker | job | cron | stateful`
- runtime 归一化为 `docker | containerd | firecracker | process | systemd |
  windows-service`
- endpoint protocol 归一化为 `tcp | udp | http`
- CPU 归一化为 `cpu_millis`
- 内存归一化为 `memory_bytes`

当前 planner/apply handoff 已支持：

- `dsl planner` 输出里已经包含 canonical apply object
- ingress route 会在那里被编译成显式 route spec，而不再只能从 legacy deploy
  side effect 间接观察到

### 当前限制

当前实现仍然明显比目标设计窄。

截至 2026-03-26 的主要限制：

- parser 仍是受限 invocation-style 语言，不是 Kotlin 解释器，也不是通用脚本语言
- 表达式支持仍较小，算术、布尔运算和更一般的编译期求值尚未完整实现
- binder 和 canonical lowerer 目前只覆盖一个静态已知子集，还不是完整签名/类型系统
- `config("name")` 与 `secret("name")` 当前主要表达 identity 与 refs，尚未支持完整 key/value payload
- `volume("name")` 当前主要表达 identity 与 `mount(...)` refs，尚未支持完整 persistent/ephemeral/size 策略
- `resources` 当前只支持 CPU 和 memory
- `health` 当前在 canonical manifest 与短 `.orch` 字段块中支持 HTTP/TCP probe；运行时当前执行 `readiness` 作为 deploy 依赖 gate，后台 liveness/startup controller 尚未接入
- 当前 lowering 路径仍基本按“每个 workload 一个主 endpoint”思路工作，多 endpoint 是下一步重要结构升级
- `workload` 已是主 authoring object，但 legacy `services { val x = create(...) { ... } }` 仍保留为兼容路径
- planner 输出同时暴露 `hir`、`ir` 和 canonical，是因为迁移仍在进行中
- `dsl plan/render/apply/delete` CLI 仍部分绑定旧 manifest 路径；`dsl planner` 才是 canonical compiler pipeline 的主要观察入口

### 当前推荐写法

如果现在要写新的 DSL 文件，推荐按下面这类写法：

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

在实现完全追上之前，本文档应分两层理解：

- 上半部分定义的是 v1 目标方向
- `当前已实现子集` 章节说明的是当前代码里真正可以依赖的能力
