package firecracker

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/dnssvc"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const (
	defaultBinaryPath = "firecracker"
	defaultBootArgs   = "console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda"
	defaultVCPUCount  = 1
	defaultMemSizeMiB = 128
)

type Provider struct {
	logger  *slog.Logger
	dns     *dnssvc.Service
	root    string
	running runningState
}

// VMConfig is the Firecracker machine configuration derived from an orch workload.
type VMConfig struct {
	ID             string
	BinaryPath     string
	APISocket      string
	KernelImage    string
	RootfsPath     string
	RootfsReadOnly bool
	BootArgs       string
	VCPUCount      int
	MemSizeMiB     int
	Network        *NetworkConfig
	StdoutPath     string
	StderrPath     string
}

// NetworkConfig is the Firecracker network interface configuration.
type NetworkConfig struct {
	InterfaceID       string `json:"interfaceID"`
	TapDeviceName     string `json:"tapDeviceName"`
	GuestMAC          string `json:"guestMAC,omitempty"`
	AllowMMDSRequests bool   `json:"allowMMDSRequests,omitempty"`
}

func NewProvider(logger *slog.Logger, dns *dnssvc.Service) *Provider {
	return NewProviderWithRoot(logger, dns, filepath.Join(config.DefaultDataRoot(), "runtime", "firecracker"))
}

// NewProviderWithRoot creates a Firecracker provider using an explicit runtime root.
func NewProviderWithRoot(logger *slog.Logger, dns *dnssvc.Service, root string) *Provider {
	return &Provider{
		logger:  logger,
		dns:     dns,
		root:    root,
		running: newRunningState(),
	}
}

func (p *Provider) Kind() deployv1.RuntimeKind {
	return deployv1.RuntimeFirecracker
}

func (p *Provider) Logs(_ context.Context, meta deployv1.Metadata, workloadName string, opts runtimeinfo.LogOptions) (runtimeinfo.LogResult, error) {
	base := p.nameBase(meta, workloadName)
	logDir := filepath.Join(p.rootOrDefault(), "logs")
	stdout, err := runtimeinfo.ReadTailFile(filepath.Join(logDir, base+".stdout.log"), opts.Tail)
	if err != nil {
		return runtimeinfo.LogResult{}, oopsx.B("runtime", "firecracker").Wrapf(err, "read stdout log")
	}
	stderr, err := runtimeinfo.ReadTailFile(filepath.Join(logDir, base+".stderr.log"), opts.Tail)
	if err != nil {
		return runtimeinfo.LogResult{}, oopsx.B("runtime", "firecracker").Wrapf(err, "read stderr log")
	}
	content := stdout
	if stderr != "" {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += stderr
	}
	return runtimeinfo.LogResult{
		Name:    strings.TrimSpace(workloadName),
		Runtime: deployv1.RuntimeFirecracker,
		Source:  logDir,
		Content: content,
	}, nil
}

// BuildConfig converts an orch workload into a Firecracker VM configuration.
func (p *Provider) BuildConfig(meta deployv1.Metadata, w deployv1.Workload) (VMConfig, error) {
	if err := runconfig.RejectUnsupportedMounts(deployv1.RuntimeFirecracker, w); err != nil {
		return VMConfig{}, oopsx.B("runtime", "firecracker").Wrapf(err, "validate workload mounts")
	}
	if w.Run.Options.Firecracker == nil {
		return VMConfig{}, oopsx.B("runtime", "firecracker").Errorf("workload %q: run.runtimeOptions.firecracker is required", w.Name)
	}
	cfg := p.baseVMConfig(meta, w)
	p.applyVMConfigDefaults(&cfg)
	if err := validateVMConfig(w.Name, cfg); err != nil {
		return VMConfig{}, err
	}
	return cfg, nil
}

func (p *Provider) baseVMConfig(meta deployv1.Metadata, w deployv1.Workload) VMConfig {
	opts := w.Run.Options.Firecracker
	id := p.nameBase(meta, w.Name)
	return VMConfig{
		ID:             id,
		BinaryPath:     strings.TrimSpace(opts.BinaryPath),
		APISocket:      strings.TrimSpace(opts.SocketPath),
		KernelImage:    strings.TrimSpace(opts.KernelImagePath),
		RootfsPath:     strings.TrimSpace(opts.RootfsPath),
		RootfsReadOnly: opts.RootfsReadOnly,
		BootArgs:       strings.TrimSpace(opts.BootArgs),
		VCPUCount:      opts.VCPUCount,
		MemSizeMiB:     opts.MemSizeMiB,
		Network:        firecrackerNetworkConfig(opts),
		StdoutPath:     filepath.Join(p.rootOrDefault(), "logs", id+".stdout.log"),
		StderrPath:     filepath.Join(p.rootOrDefault(), "logs", id+".stderr.log"),
	}
}

func (p *Provider) applyVMConfigDefaults(cfg *VMConfig) {
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = defaultBinaryPath
	}
	if cfg.APISocket == "" {
		cfg.APISocket = filepath.Join(p.rootOrDefault(), "sockets", cfg.ID+".sock")
	}
	if cfg.BootArgs == "" {
		cfg.BootArgs = defaultBootArgsForRootfs(cfg.RootfsReadOnly)
	}
	if cfg.VCPUCount <= 0 {
		cfg.VCPUCount = defaultVCPUCount
	}
	if cfg.MemSizeMiB <= 0 {
		cfg.MemSizeMiB = defaultMemSizeMiB
	}
}

func validateVMConfig(workloadName string, cfg VMConfig) error {
	if cfg.KernelImage == "" {
		return oopsx.B("runtime", "firecracker").Errorf("workload %q: kernel_image_path is required", workloadName)
	}
	if cfg.RootfsPath == "" {
		return oopsx.B("runtime", "firecracker").Errorf("workload %q: rootfs_path is required", workloadName)
	}
	if cfg.Network != nil && cfg.Network.TapDeviceName == "" {
		return oopsx.B("runtime", "firecracker").Errorf("workload %q: tap_device_name is required when firecracker network is configured", workloadName)
	}
	return nil
}

func firecrackerNetworkConfig(opts *deployv1.FirecrackerOptions) *NetworkConfig {
	if opts == nil {
		return nil
	}
	tap := strings.TrimSpace(opts.TapDeviceName)
	if tap == "" && strings.TrimSpace(opts.NetworkInterfaceID) == "" && strings.TrimSpace(opts.GuestMAC) == "" && !opts.AllowMMDSRequests {
		return nil
	}
	id := strings.TrimSpace(opts.NetworkInterfaceID)
	if id == "" {
		id = "eth0"
	}
	return &NetworkConfig{
		InterfaceID:       id,
		TapDeviceName:     tap,
		GuestMAC:          strings.TrimSpace(opts.GuestMAC),
		AllowMMDSRequests: opts.AllowMMDSRequests,
	}
}

func defaultBootArgsForRootfs(readOnly bool) string {
	if readOnly {
		return defaultBootArgs + " ro"
	}
	return defaultBootArgs + " rw"
}

func (p *Provider) nameBase(meta deployv1.Metadata, workloadName string) string {
	return fmt.Sprintf("%s-%s-%s",
		workloadmeta.SanitizeName(workloadmeta.NamespaceOrDefault(meta.Namespace)),
		workloadmeta.SanitizeName(meta.Name),
		workloadmeta.SanitizeName(workloadName),
	)
}

func (p *Provider) rootOrDefault() string {
	if strings.TrimSpace(p.root) != "" {
		return filepath.Clean(p.root)
	}
	return filepath.Join(config.DefaultDataRoot(), "runtime", "firecracker")
}

// ArtifactSummary returns the Firecracker artifact label for status/state output.
func ArtifactSummary(run deployv1.RunSpec) string {
	if summary := runconfig.ArtifactSummary(run); summary != "" {
		return summary
	}
	if run.Options.Firecracker != nil {
		return strings.TrimSpace(run.Options.Firecracker.RootfsPath)
	}
	return ""
}
