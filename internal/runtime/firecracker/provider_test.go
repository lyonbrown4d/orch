package firecracker_test

import (
	"path/filepath"
	"strings"
	"testing"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/firecracker"
)

func TestBuildConfigDefaults(t *testing.T) {
	t.Parallel()

	provider := firecracker.NewProviderWithRoot(nil, nil, t.TempDir())
	meta := deployv1.Metadata{Name: "demo", Namespace: "prod"}
	workload := deployv1.Workload{
		Name: "vm",
		Run: deployv1.RunSpec{
			Options: deployv1.RunOptions{
				Firecracker: &deployv1.FirecrackerOptions{
					KernelImagePath: "/var/lib/orch/vmlinux",
					RootfsPath:      "/var/lib/orch/rootfs.ext4",
				},
			},
		},
	}

	cfg, err := provider.BuildConfig(meta, workload)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ID != "prod-demo-vm" {
		t.Fatalf("id = %q", cfg.ID)
	}
	if cfg.BinaryPath != "firecracker" {
		t.Fatalf("binary = %q", cfg.BinaryPath)
	}
	if cfg.VCPUCount != 1 || cfg.MemSizeMiB != 128 {
		t.Fatalf("machine config = %d/%d", cfg.VCPUCount, cfg.MemSizeMiB)
	}
	if !strings.Contains(cfg.BootArgs, "root=/dev/vda") {
		t.Fatalf("boot args = %q", cfg.BootArgs)
	}
	if !strings.HasSuffix(cfg.BootArgs, " rw") {
		t.Fatalf("boot args = %q, want rw rootfs", cfg.BootArgs)
	}
	if !strings.HasSuffix(cfg.APISocket, filepath.Join("sockets", "prod-demo-vm.sock")) {
		t.Fatalf("api socket = %q", cfg.APISocket)
	}
}

func TestBuildConfigOptions(t *testing.T) {
	t.Parallel()

	provider := firecracker.NewProviderWithRoot(nil, nil, t.TempDir())
	workload := deployv1.Workload{
		Name: "vm",
		Run: deployv1.RunSpec{
			Options: deployv1.RunOptions{
				Firecracker: &deployv1.FirecrackerOptions{
					KernelImagePath: "/kernel",
					RootfsPath:      "/rootfs",
					BootArgs:        "console=ttyS0 root=/dev/vda ro",
					BinaryPath:      "/usr/local/bin/firecracker",
					SocketPath:      filepath.Join(t.TempDir(), "fc.sock"),
					RootfsReadOnly:  true,
					TapDeviceName:   "tap-orch0",
					GuestMAC:        "AA:FC:00:00:00:01",
					VCPUCount:       2,
					MemSizeMiB:      256,
				},
			},
		},
	}

	cfg, err := provider.BuildConfig(deployv1.Metadata{Name: "demo"}, workload)
	if err != nil {
		t.Fatal(err)
	}
	assertFirecrackerMachineConfig(t, cfg)
	assertFirecrackerRootfsConfig(t, cfg)
	assertFirecrackerNetworkConfig(t, cfg.Network)
}

func assertFirecrackerMachineConfig(t *testing.T, cfg firecracker.VMConfig) {
	t.Helper()
	if cfg.BinaryPath != "/usr/local/bin/firecracker" || cfg.VCPUCount != 2 || cfg.MemSizeMiB != 256 {
		t.Fatalf("config = %+v", cfg)
	}
}

func assertFirecrackerRootfsConfig(t *testing.T, cfg firecracker.VMConfig) {
	t.Helper()
	if !cfg.RootfsReadOnly || cfg.BootArgs != "console=ttyS0 root=/dev/vda ro" {
		t.Fatalf("rootfs/boot config = %+v", cfg)
	}
}

func assertFirecrackerNetworkConfig(t *testing.T, cfg *firecracker.NetworkConfig) {
	t.Helper()
	if cfg == nil || cfg.InterfaceID != "eth0" || cfg.TapDeviceName != "tap-orch0" || cfg.GuestMAC != "AA:FC:00:00:00:01" {
		t.Fatalf("network config = %+v", cfg)
	}
}

func TestBuildConfigReadOnlyDefaultBootArgs(t *testing.T) {
	t.Parallel()

	provider := firecracker.NewProviderWithRoot(nil, nil, t.TempDir())
	workload := deployv1.Workload{
		Name: "vm",
		Run: deployv1.RunSpec{
			Options: deployv1.RunOptions{
				Firecracker: &deployv1.FirecrackerOptions{
					KernelImagePath: "/kernel",
					RootfsPath:      "/rootfs",
					RootfsReadOnly:  true,
				},
			},
		},
	}

	cfg, err := provider.BuildConfig(deployv1.Metadata{Name: "demo"}, workload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.BootArgs, " ro") {
		t.Fatalf("boot args = %q, want ro rootfs", cfg.BootArgs)
	}
}

func TestBuildConfigNetworkRequiresTapDevice(t *testing.T) {
	t.Parallel()

	provider := firecracker.NewProviderWithRoot(nil, nil, t.TempDir())
	workload := deployv1.Workload{
		Name: "vm",
		Run: deployv1.RunSpec{
			Options: deployv1.RunOptions{
				Firecracker: &deployv1.FirecrackerOptions{
					KernelImagePath:    "/kernel",
					RootfsPath:         "/rootfs",
					NetworkInterfaceID: "eth0",
				},
			},
		},
	}

	_, err := provider.BuildConfig(deployv1.Metadata{Name: "demo"}, workload)
	if err == nil || !strings.Contains(err.Error(), "tap_device_name is required") {
		t.Fatalf("buildConfig error = %v, want tap device error", err)
	}
}

func TestFirecrackerArtifactSummaryFallsBackToRootfs(t *testing.T) {
	t.Parallel()

	run := deployv1.RunSpec{
		Options: deployv1.RunOptions{
			Firecracker: &deployv1.FirecrackerOptions{RootfsPath: "/rootfs.ext4"},
		},
	}
	if got := firecracker.ArtifactSummary(run); got != "/rootfs.ext4" {
		t.Fatalf("summary = %q", got)
	}
}

func TestBuildConfigRejectsMounts(t *testing.T) {
	t.Parallel()

	provider := firecracker.NewProviderWithRoot(nil, nil, t.TempDir())
	_, err := provider.BuildConfig(deployv1.Metadata{Name: "demo", Namespace: "prod"}, deployv1.Workload{
		Name: "vm",
		Run: deployv1.RunSpec{Options: deployv1.RunOptions{Firecracker: &deployv1.FirecrackerOptions{
			KernelImagePath: "/var/lib/orch/vmlinux",
			RootfsPath:      "/var/lib/orch/rootfs.ext4",
		}}},
		Mounts: []deployv1.Mount{{Volume: deployv1.VolumeRef{Name: "data"}, Target: "/data"}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support workload mounts") {
		t.Fatalf("err = %v", err)
	}
}
