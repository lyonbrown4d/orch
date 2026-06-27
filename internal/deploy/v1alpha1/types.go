// Package v1alpha1 defines the canonical orch deploy document for YAML/JSON.
// Fields remain plain slices and maps at the YAML manifest boundary. collectionx
// containers provide JSON/Binary/Gob serialization and are used through
// collections.go and JSON-only views where that does not weaken YAML decoding.
package v1alpha1

// App is the YAML-friendly canonical deploy model for the first Go rewrite
// iteration. It intentionally mirrors the canonical model described in
// docs/src/dsl.md and docs/src/dsl.zh.md.
type App struct {
	APIVersion string   `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string   `json:"kind,omitempty"       yaml:"kind,omitempty"`
	Metadata   Metadata `json:"metadata"             yaml:"metadata"`

	Workloads []Workload `json:"workloads,omitempty" yaml:"workloads,omitempty"`
	Configs   []Config   `json:"configs,omitempty"   yaml:"configs,omitempty"`
	Secrets   []Secret   `json:"secrets,omitempty"   yaml:"secrets,omitempty"`
	Volumes   []Volume   `json:"volumes,omitempty"   yaml:"volumes,omitempty"`
	Ingresses []Ingress  `json:"ingresses,omitempty" yaml:"ingresses,omitempty"`
}

type Metadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Namespace   string            `json:"namespace,omitempty"   yaml:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type Workload struct {
	Name string       `json:"name" yaml:"name"`
	Kind WorkloadKind `json:"kind" yaml:"kind"` // service|worker|job|cron|stateful
	Run  RunSpec      `json:"run"  yaml:"run"`  // runtime-neutral artifact + exec + env/cwd/runtimeOptions
	// Runtime selects the backend adapter. This stays separate from Run.RuntimeOptions
	// because the canonical intent model needs a stable first-class field.
	Runtime RuntimeKind `json:"runtime" yaml:"runtime"` // docker|containerd|podman|firecracker|process|systemd|windows-service

	Replicas  int           `json:"replicas,omitempty"  yaml:"replicas,omitempty"`
	DependsOn []WorkloadRef `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`

	Endpoints []Endpoint `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
	Mounts    []Mount    `json:"mounts,omitempty"    yaml:"mounts,omitempty"`
	Resources *Resources `json:"resources,omitempty" yaml:"resources,omitempty"`
	Health    *Health    `json:"health,omitempty"    yaml:"health,omitempty"`
	Lifecycle *Lifecycle `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`

	Scheduling *Scheduling `json:"scheduling,omitempty" yaml:"scheduling,omitempty"`
	Rollout    *Rollout    `json:"rollout,omitempty"    yaml:"rollout,omitempty"`
}

type WorkloadKind string

const (
	WorkloadKindService  WorkloadKind = "service"
	WorkloadKindWorker   WorkloadKind = "worker"
	WorkloadKindJob      WorkloadKind = "job"
	WorkloadKindCron     WorkloadKind = "cron"
	WorkloadKindStateful WorkloadKind = "stateful"
)

type RuntimeKind string

const (
	RuntimeDocker         RuntimeKind = "docker"
	RuntimeContainerd     RuntimeKind = "containerd"
	RuntimePodman         RuntimeKind = "podman"
	RuntimeFirecracker    RuntimeKind = "firecracker"
	RuntimeProcess        RuntimeKind = "process"
	RuntimeSystemd        RuntimeKind = "systemd"
	RuntimeWindowsService RuntimeKind = "windows-service"
)

type RunSpec struct {
	Artifact ArtifactSpec `json:"artifact,omitzero" yaml:"artifact,omitempty"`
	Exec     ExecSpec     `json:"exec,omitzero"     yaml:"exec,omitempty"`
	Env      []EnvVar     `json:"env,omitempty"     yaml:"env,omitempty"`
	Cwd      string       `json:"cwd,omitempty"     yaml:"cwd,omitempty"`
	User     string       `json:"user,omitempty"    yaml:"user,omitempty"`
	Options  RunOptions   `json:"runtimeOptions"    yaml:"runtimeOptions"`
}

type ArtifactSpec struct {
	Image string `json:"image,omitempty" yaml:"image,omitempty"` // OCI/container image.
	Path  string `json:"path,omitempty"  yaml:"path,omitempty"`  // Local executable/package/rootfs path.
	URL   string `json:"url,omitempty"   yaml:"url,omitempty"`   // Future remote artifact source.
}

type ExecSpec struct {
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string `json:"args,omitempty"    yaml:"args,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"  yaml:"name"`
	Value string `json:"value" yaml:"value"`
}

// RunOptions captures backend-specific knobs. Cross-runtime execution intent
// belongs on RunSpec; adapter-native details stay in these optional branches.
type RunOptions struct {
	Docker         *DockerOptions         `json:"docker,omitempty"         yaml:"docker,omitempty"`
	Containerd     *ContainerdOptions     `json:"containerd,omitempty"     yaml:"containerd,omitempty"`
	Firecracker    *FirecrackerOptions    `json:"firecracker,omitempty"    yaml:"firecracker,omitempty"`
	Process        *ProcessOptions        `json:"process,omitempty"        yaml:"process,omitempty"`
	Systemd        *SystemdOptions        `json:"systemd,omitempty"        yaml:"systemd,omitempty"`
	WindowsService *WindowsServiceOptions `json:"windowsService,omitempty" yaml:"windowsService,omitempty"`
}

type DockerOptions struct {
	NetworkMode string            `json:"networkMode,omitempty" yaml:"networkMode,omitempty"`
	Privileged  bool              `json:"privileged,omitempty"  yaml:"privileged,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
}

type ContainerdOptions struct {
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	// Snapshotter or runtime handler can be added later when we wire containerd.
}

type FirecrackerOptions struct {
	KernelImagePath    string `json:"kernelImagePath,omitempty"    yaml:"kernelImagePath,omitempty"`
	RootfsPath         string `json:"rootfsPath,omitempty"         yaml:"rootfsPath,omitempty"`
	BootArgs           string `json:"bootArgs,omitempty"           yaml:"bootArgs,omitempty"`
	BinaryPath         string `json:"binaryPath,omitempty"         yaml:"binaryPath,omitempty"`
	SocketPath         string `json:"socketPath,omitempty"         yaml:"socketPath,omitempty"`
	RootfsReadOnly     bool   `json:"rootfsReadOnly,omitempty"     yaml:"rootfsReadOnly,omitempty"`
	NetworkInterfaceID string `json:"networkInterfaceID,omitempty" yaml:"networkInterfaceID,omitempty"`
	TapDeviceName      string `json:"tapDeviceName,omitempty"      yaml:"tapDeviceName,omitempty"`
	GuestMAC           string `json:"guestMAC,omitempty"           yaml:"guestMAC,omitempty"`
	AllowMMDSRequests  bool   `json:"allowMMDSRequests,omitempty"  yaml:"allowMMDSRequests,omitempty"`
	VCPUCount          int    `json:"vcpuCount,omitempty"          yaml:"vcpuCount,omitempty"`
	MemSizeMiB         int    `json:"memSizeMiB,omitempty"         yaml:"memSizeMiB,omitempty"`
}

type ProcessOptions struct {
	GracefulStopTimeout string `json:"gracefulStopTimeout,omitempty" yaml:"gracefulStopTimeout,omitempty"`
	StdoutPath          string `json:"stdoutPath,omitempty"          yaml:"stdoutPath,omitempty"`
	StderrPath          string `json:"stderrPath,omitempty"          yaml:"stderrPath,omitempty"`
}

type SystemdOptions struct {
	UnitName   string `json:"unitName,omitempty"   yaml:"unitName,omitempty"`
	User       string `json:"user,omitempty"       yaml:"user,omitempty"`
	Group      string `json:"group,omitempty"      yaml:"group,omitempty"`
	Restart    string `json:"restart,omitempty"    yaml:"restart,omitempty"`
	RestartSec string `json:"restartSec,omitempty" yaml:"restartSec,omitempty"`
	WantedBy   string `json:"wantedBy,omitempty"   yaml:"wantedBy,omitempty"`
}

type WindowsServiceOptions struct {
	ServiceName string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	StartType   string `json:"startType,omitempty"   yaml:"startType,omitempty"`
}

type Endpoint struct {
	Name     string        `json:"name"               yaml:"name"`
	Port     int           `json:"port"               yaml:"port"`
	Protocol EndpointProto `json:"protocol"           yaml:"protocol"` // tcp|udp|http
	HostPort int           `json:"hostPort,omitempty" yaml:"hostPort,omitempty"`
	HostIP   string        `json:"hostIP,omitempty"   yaml:"hostIP,omitempty"`
}
type EndpointProto string

const (
	ProtoTCP  EndpointProto = "tcp"
	ProtoUDP  EndpointProto = "udp"
	ProtoHTTP EndpointProto = "http"
)

type Mount struct {
	Volume   VolumeRef `json:"volume"             yaml:"volume"`
	Target   string    `json:"target"             yaml:"target"`
	ReadOnly bool      `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
}

type Resources struct {
	CPUMillis   int64 `json:"cpuMillis,omitempty"   yaml:"cpuMillis,omitempty"`
	MemoryBytes int64 `json:"memoryBytes,omitempty" yaml:"memoryBytes,omitempty"`
}

type Health struct {
	Readiness *Probe `json:"readiness,omitempty" yaml:"readiness,omitempty"`
	Liveness  *Probe `json:"liveness,omitempty"  yaml:"liveness,omitempty"`
	Startup   *Probe `json:"startup,omitempty"   yaml:"startup,omitempty"`
}

type Lifecycle struct {
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty" yaml:"restartPolicy,omitempty"`
	MaxRestarts   int           `json:"maxRestarts,omitempty"   yaml:"maxRestarts,omitempty"`
	RestartDelay  string        `json:"restartDelay,omitempty"  yaml:"restartDelay,omitempty"`
}

type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "always"
	RestartPolicyOnFailure RestartPolicy = "on-failure"
	RestartPolicyNever     RestartPolicy = "never"
)

type Probe struct {
	HTTP        *HTTPProbe `json:"http,omitempty"        yaml:"http,omitempty"`
	TCP         *TCPProbe  `json:"tcp,omitempty"         yaml:"tcp,omitempty"`
	Interval    string     `json:"interval,omitempty"    yaml:"interval,omitempty"`
	Timeout     string     `json:"timeout,omitempty"     yaml:"timeout,omitempty"`
	Retries     int        `json:"retries,omitempty"     yaml:"retries,omitempty"`
	StartPeriod string     `json:"startPeriod,omitempty" yaml:"startPeriod,omitempty"`
}

type HTTPProbe struct {
	Path     string      `json:"path,omitempty"    yaml:"path,omitempty"`
	Endpoint EndpointRef `json:"endpoint,omitzero" yaml:"endpoint,omitempty"`
	Port     int         `json:"port,omitempty"    yaml:"port,omitempty"`
}

type TCPProbe struct {
	Endpoint EndpointRef `json:"endpoint,omitzero" yaml:"endpoint,omitempty"`
	Port     int         `json:"port,omitempty"    yaml:"port,omitempty"`
}

type Scheduling struct {
	Stateful       bool     `json:"stateful,omitempty"       yaml:"stateful,omitempty"`
	AllowLeader    bool     `json:"allowLeader,omitempty"    yaml:"allowLeader,omitempty"`
	PreferredNodes []string `json:"preferredNodes,omitempty" yaml:"preferredNodes,omitempty"`
}

type Rollout struct {
	Strategy       string `json:"strategy,omitempty"       yaml:"strategy,omitempty"`
	MaxUnavailable int    `json:"maxUnavailable,omitempty" yaml:"maxUnavailable,omitempty"`
	MaxSurge       int    `json:"maxSurge,omitempty"       yaml:"maxSurge,omitempty"`
}

const (
	RolloutStrategyRecreate        = "recreate"
	RolloutStrategyStopBeforeStart = "stop-before-start"
	RolloutStrategyRolling         = "rolling"
)

type Config struct {
	Name string            `json:"name"           yaml:"name"`
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
}

type Secret struct {
	Name string            `json:"name"           yaml:"name"`
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
}

type Volume struct {
	Name       string `json:"name"                 yaml:"name"`
	Persistent bool   `json:"persistent,omitempty" yaml:"persistent,omitempty"`
	// SizeBytes is optional; keep numeric for canonical normalization.
	SizeBytes int64 `json:"sizeBytes,omitempty" yaml:"sizeBytes,omitempty"`
}

type Ingress struct {
	Name   string         `json:"name"             yaml:"name"`
	Host   string         `json:"host,omitempty"   yaml:"host,omitempty"`
	Routes []IngressRoute `json:"routes,omitempty" yaml:"routes,omitempty"`
}

type IngressRoute struct {
	Path    string      `json:"path"    yaml:"path"`
	Backend EndpointRef `json:"backend" yaml:"backend"`
}

// ---- Typed references (YAML-friendly) ----

// WorkloadRef refers to a workload by name. YAML form:
// - "redis" (string)  OR  { name: "redis" }
type WorkloadRef struct {
	Name      string              `json:"name"                yaml:"name"`
	Condition DependencyCondition `json:"condition,omitempty" yaml:"condition,omitempty"`
}

type DependencyCondition string

const (
	DependencyConditionStarted   DependencyCondition = "started"
	DependencyConditionReady     DependencyCondition = "ready"
	DependencyConditionCompleted DependencyCondition = "completed"
)

// VolumeRef refers to a volume by name. YAML form:
// - "redisData"  OR  { name: "redisData" }
type VolumeRef struct {
	Name string `json:"name" yaml:"name"`
}

// EndpointRef refers to a workload endpoint. YAML form:
// - "gateway:http" OR { workload: "gateway", endpoint: "http" }
type EndpointRef struct {
	Workload string `json:"workload" yaml:"workload"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}
