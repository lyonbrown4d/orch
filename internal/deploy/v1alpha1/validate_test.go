package v1alpha1_test

import (
	"strings"
	"testing"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func validApp() *deployv1.App {
	return &deployv1.App{
		Metadata: deployv1.Metadata{Name: "demo"},
		Workloads: []deployv1.Workload{
			{
				Name:    "api",
				Kind:    deployv1.WorkloadKindService,
				Runtime: deployv1.RuntimeDocker,
				Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "nginx"}},
			},
		},
	}
}

func TestValidateProcessRequiresCommandOrArtifactPath(t *testing.T) {
	app := validApp()
	app.Workloads[0].Runtime = deployv1.RuntimeProcess
	app.Workloads[0].Run.Artifact = deployv1.ArtifactSpec{}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `run.exec.command or run.artifact.path is required for runtime "process"`) {
		t.Fatalf("Validate() error = %v, want process command error", err)
	}

	app.Workloads[0].Run.Artifact.Path = "/opt/app/api"
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with artifact path = %v", err)
	}
}

func TestValidatePodmanRequiresImage(t *testing.T) {
	app := validApp()
	app.Workloads[0].Runtime = deployv1.RuntimePodman
	app.Workloads[0].Run.Artifact = deployv1.ArtifactSpec{}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `run.artifact.image is required for runtime "podman"`) {
		t.Fatalf("Validate() error = %v, want podman image error", err)
	}

	app.Workloads[0].Run.Artifact.Image = "nginx"
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with podman image = %v", err)
	}
}

func TestValidateFirecrackerRequiresRuntimeOptions(t *testing.T) {
	app := validApp()
	app.Workloads[0].Runtime = deployv1.RuntimeFirecracker
	app.Workloads[0].Run.Artifact = deployv1.ArtifactSpec{}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `run.runtimeOptions.firecracker is required`) {
		t.Fatalf("Validate() error = %v, want firecracker options error", err)
	}

	app.Workloads[0].Run.Options.Firecracker = &deployv1.FirecrackerOptions{
		KernelImagePath: "/var/lib/orch/vmlinux",
		RootfsPath:      "/var/lib/orch/rootfs.ext4",
	}
	if validateErr := app.Validate(); validateErr != nil {
		t.Fatalf("Validate() with firecracker options = %v", validateErr)
	}

	app.Workloads[0].Run.Options.Firecracker.MemSizeMiB = -1
	err = app.Validate()
	if err == nil || !strings.Contains(err.Error(), `run.runtimeOptions.firecracker.memSizeMiB must be >= 0`) {
		t.Fatalf("Validate() error = %v, want firecracker mem error", err)
	}
}

func TestValidateFirecrackerNetwork(t *testing.T) {
	app := validApp()
	app.Workloads[0].Runtime = deployv1.RuntimeFirecracker
	app.Workloads[0].Run.Artifact = deployv1.ArtifactSpec{}
	app.Workloads[0].Run.Options.Firecracker = &deployv1.FirecrackerOptions{
		KernelImagePath:    "/var/lib/orch/vmlinux",
		RootfsPath:         "/var/lib/orch/rootfs.ext4",
		NetworkInterfaceID: "eth0",
	}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), `tapDeviceName is required`) {
		t.Fatalf("Validate() error = %v, want tap device error", err)
	}

	app.Workloads[0].Run.Options.Firecracker.TapDeviceName = "tap-orch0"
	app.Workloads[0].Run.Options.Firecracker.GuestMAC = "not-a-mac"
	err = app.Validate()
	if err == nil || !strings.Contains(err.Error(), `guestMAC is invalid`) {
		t.Fatalf("Validate() error = %v, want mac error", err)
	}

	app.Workloads[0].Run.Options.Firecracker.GuestMAC = "AA:FC:00:00:00:01"
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with firecracker network = %v", err)
	}
}

func TestValidateRejectsEmptyEnvName(t *testing.T) {
	app := validApp()
	app.Workloads[0].Run.Env = []deployv1.EnvVar{{Name: " ", Value: "8080"}}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "run.env[0].name is required") {
		t.Fatalf("Validate() error = %v, want empty env name error", err)
	}
}

func TestValidateRejectsNegativeResources(t *testing.T) {
	tests := []struct {
		name string
		res  deployv1.Resources
		want string
	}{
		{name: "cpu", res: deployv1.Resources{CPUMillis: -1}, want: "resources.cpuMillis must be >= 0"},
		{name: "memory", res: deployv1.Resources{MemoryBytes: -1}, want: "resources.memoryBytes must be >= 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := validApp()
			app.Workloads[0].Resources = &tt.res

			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRolloutProgressDeadline(t *testing.T) {
	tests := []struct {
		name     string
		deadline string
		want     string
	}{
		{name: "invalid", deadline: "soon", want: "rollout.progressDeadline is invalid"},
		{name: "zero", deadline: "0s", want: "rollout.progressDeadline must be > 0"},
		{name: "negative", deadline: "-1s", want: "rollout.progressDeadline must be > 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := validApp()
			app.Workloads[0].Rollout = &deployv1.Rollout{ProgressDeadline: tt.deadline}

			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}

	app := validApp()
	app.Workloads[0].Rollout = &deployv1.Rollout{ProgressDeadline: "30s"}
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with progress deadline = %v", err)
	}
}

func TestValidateRejectsWorkloadDependencyCycle(t *testing.T) {
	app := validApp()
	app.Workloads = append(app.Workloads, deployv1.Workload{
		Name:    "db",
		Kind:    deployv1.WorkloadKindStateful,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "postgres"}},
	})
	app.Workloads[0].DependsOn = []deployv1.WorkloadRef{{Name: "db"}}
	app.Workloads[1].DependsOn = []deployv1.WorkloadRef{{Name: "api"}}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "workloads dependsOn contains a cycle") {
		t.Fatalf("Validate() error = %v, want dependency cycle error", err)
	}
}

func TestValidateAllowsAcyclicWorkloadDependencies(t *testing.T) {
	app := validApp()
	app.Workloads = append(app.Workloads, deployv1.Workload{
		Name:    "db",
		Kind:    deployv1.WorkloadKindStateful,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "postgres"}},
	})
	app.Workloads[0].DependsOn = []deployv1.WorkloadRef{{Name: "db"}}

	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with acyclic dependencies = %v", err)
	}
}

func TestWorkloadRefEffectiveConditionDefaultsToReady(t *testing.T) {
	ref := deployv1.WorkloadRef{Name: "db"}
	if got := ref.EffectiveCondition(); got != deployv1.DependencyConditionReady {
		t.Fatalf("EffectiveCondition() = %q, want %q", got, deployv1.DependencyConditionReady)
	}
}

func TestValidateAllowsCompletedJobDependency(t *testing.T) {
	app := validApp()
	app.Workloads = append(app.Workloads, deployv1.Workload{
		Name:    "init-db",
		Kind:    deployv1.WorkloadKindJob,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "migrate"}},
	})
	app.Workloads[0].DependsOn = []deployv1.WorkloadRef{{
		Name:      "init-db",
		Condition: deployv1.DependencyConditionCompleted,
	}}

	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with completed job dependency = %v", err)
	}
}

func TestValidateRejectsInvalidDependencyCondition(t *testing.T) {
	app := validApp()
	app.Workloads = append(app.Workloads, deployv1.Workload{
		Name:    "db",
		Kind:    deployv1.WorkloadKindStateful,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "postgres"}},
	})
	app.Workloads[0].DependsOn = []deployv1.WorkloadRef{{Name: "db", Condition: deployv1.DependencyCondition("healthy")}}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "dependsOn[0].condition is invalid") {
		t.Fatalf("Validate() error = %v, want dependency condition error", err)
	}
}

func TestValidateRejectsCompletedDependencyOnService(t *testing.T) {
	app := validApp()
	app.Workloads = append(app.Workloads, deployv1.Workload{
		Name:    "db",
		Kind:    deployv1.WorkloadKindStateful,
		Runtime: deployv1.RuntimeDocker,
		Run:     deployv1.RunSpec{Artifact: deployv1.ArtifactSpec{Image: "postgres"}},
	})
	app.Workloads[0].DependsOn = []deployv1.WorkloadRef{{
		Name:      "db",
		Condition: deployv1.DependencyConditionCompleted,
	}}

	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "condition completed requires a job or cron dependency") {
		t.Fatalf("Validate() error = %v, want completed dependency kind error", err)
	}
}

func TestValidateEndpointHostPublish(t *testing.T) {
	tests := []struct {
		name     string
		hostPort int
		hostIP   string
		want     string
	}{
		{name: "host port negative", hostPort: -1, want: "hostPort must be 1..65535"},
		{name: "host port too large", hostPort: 70000, want: "hostPort must be 1..65535"},
		{name: "host ip invalid", hostPort: 8080, hostIP: "not-an-ip", want: "hostIP is invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := validApp()
			app.Workloads[0].Endpoints = []deployv1.Endpoint{{
				Name:     "http",
				Port:     80,
				Protocol: deployv1.ProtoHTTP,
				HostPort: tt.hostPort,
				HostIP:   tt.hostIP,
			}}

			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}

	app := validApp()
	app.Workloads[0].Endpoints = []deployv1.Endpoint{{Name: "http", Port: 80, Protocol: deployv1.ProtoHTTP, HostPort: 8080, HostIP: "127.0.0.1"}}
	if err := app.Validate(); err != nil {
		t.Fatalf("Validate() with host publish = %v", err)
	}
}
