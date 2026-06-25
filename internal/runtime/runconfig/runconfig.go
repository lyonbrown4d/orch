package runconfig

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/samber/lo"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const (
	nanoCPUsPerMilli = int64(1_000_000)
	cfsPeriodMicros  = uint64(100_000)
)

// Mount is a runtime-ready view of a workload mount.
type Mount struct {
	Volume     string
	VolumeName string
	SourcePath string
	Target     string
	ReadOnly   bool
}

// Env returns Docker/OCI-style environment entries.
func Env(vars *list.List[deployv1.EnvVar]) *list.List[string] {
	return list.FilterMapList(vars, func(_ int, v deployv1.EnvVar) (string, bool) {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			return "", false
		}
		return name + "=" + v.Value, true
	})
}

// CommandArgs returns an explicit OCI process argv when run.exec.command is set.
func CommandArgs(run deployv1.RunSpec) *list.List[string] {
	if len(run.Exec.Command) == 0 {
		return list.NewList[string]()
	}
	out := list.NewListWithCapacity[string](len(run.Exec.Command) + len(run.Exec.Args))
	out.Add(run.Exec.Command...)
	out.Add(run.Exec.Args...)
	return out
}

// ProcessCommand returns the executable path and argv for local process-style runtimes.
func ProcessCommand(run deployv1.RunSpec) (string, *list.List[string], bool) {
	if len(run.Exec.Command) > 0 {
		exe := strings.TrimSpace(run.Exec.Command[0])
		if exe == "" {
			return "", list.NewList[string](), false
		}
		args := list.NewListWithCapacity[string](len(run.Exec.Command) - 1 + len(run.Exec.Args))
		args.Add(run.Exec.Command[1:]...)
		args.Add(run.Exec.Args...)
		return exe, args, true
	}
	exe := strings.TrimSpace(run.Artifact.Path)
	if exe == "" {
		return "", list.NewList[string](), false
	}
	return exe, list.NewList(run.Exec.Args...), true
}

// ArtifactSummary returns a compact human-facing identifier for a workload artifact.
func ArtifactSummary(run deployv1.RunSpec) string {
	command := ""
	if len(run.Exec.Command) > 0 {
		command = strings.TrimSpace(run.Exec.Command[0])
	}
	return lo.CoalesceOrEmpty(
		strings.TrimSpace(run.Artifact.Image),
		strings.TrimSpace(run.Artifact.Path),
		strings.TrimSpace(run.Artifact.URL),
		command,
	)
}

// NanoCPUs converts millicores to Docker NanoCPUs.
func NanoCPUs(cpuMillis int64) int64 {
	if cpuMillis <= 0 {
		return 0
	}
	if cpuMillis > math.MaxInt64/nanoCPUsPerMilli {
		return math.MaxInt64
	}
	return cpuMillis * nanoCPUsPerMilli
}

// CFSQuota converts millicores to a Linux CFS quota using a stable 100ms period.
func CFSQuota(cpuMillis int64) (quota int64, period uint64) {
	if cpuMillis <= 0 {
		return 0, cfsPeriodMicros
	}
	quota = int64(cfsPeriodMicros) * cpuMillis / 1000
	if quota <= 0 {
		quota = 1
	}
	return quota, cfsPeriodMicros
}

// LocalMounts converts deploy mounts into local runtime mount descriptors.
func LocalMounts(root string, meta deployv1.Metadata, workload deployv1.Workload) (*list.List[Mount], error) {
	mounts := list.NewListWithCapacity[Mount](len(workload.Mounts))
	var convertErr error
	workload.MountList().Range(func(i int, mount deployv1.Mount) bool {
		converted, err := LocalMount(root, meta, mount)
		if err != nil {
			convertErr = oopsx.B("runtime", "mount").Wrapf(err, "workload %s mounts[%d]", workload.Name, i)
			return false
		}
		mounts.Add(converted)
		return true
	})
	if convertErr != nil {
		return nil, convertErr
	}
	return mounts, nil
}

// LocalMount converts one deploy mount into a runtime-ready local mount.
func LocalMount(root string, meta deployv1.Metadata, mount deployv1.Mount) (Mount, error) {
	volume := strings.TrimSpace(mount.Volume.Name)
	if volume == "" {
		return Mount{}, oopsx.B("runtime", "mount").Errorf("volume name is required")
	}
	target, err := cleanMountTarget(mount.Target)
	if err != nil {
		return Mount{}, err
	}
	out := Mount{
		Volume:     volume,
		VolumeName: VolumeName(meta, volume),
		Target:     target,
		ReadOnly:   mount.ReadOnly,
	}
	if strings.TrimSpace(root) != "" {
		out.SourcePath = LocalVolumePath(root, meta, volume)
	}
	return out, nil
}

// VolumeName returns a stable runtime-local volume name.
func VolumeName(meta deployv1.Metadata, volumeName string) string {
	ns := workloadmeta.SanitizeName(workloadmeta.NamespaceOrDefault(meta.Namespace))
	app := workloadmeta.SanitizeName(meta.Name)
	volume := workloadmeta.SanitizeName(volumeName)
	return fmt.Sprintf("orch-%s-%s-%s", ns, app, volume)
}

// LocalVolumePath returns the host path backing a local runtime volume.
func LocalVolumePath(root string, meta deployv1.Metadata, volumeName string) string {
	return filepath.Join(
		filepath.Clean(root),
		"volumes",
		workloadmeta.SanitizeName(workloadmeta.NamespaceOrDefault(meta.Namespace)),
		workloadmeta.SanitizeName(meta.Name),
		workloadmeta.SanitizeName(volumeName),
	)
}

// EnsureLocalMountSources creates host directories for mounts with SourcePath set.
func EnsureLocalMountSources(mounts *list.List[Mount]) error {
	if mounts == nil {
		return nil
	}
	var ensureErr error
	mounts.Range(func(_ int, mount Mount) bool {
		if strings.TrimSpace(mount.SourcePath) == "" {
			return true
		}
		if err := os.MkdirAll(mount.SourcePath, 0o755); err != nil {
			ensureErr = oopsx.B("runtime", "mount").Wrapf(err, "create volume source %s", mount.SourcePath)
			return false
		}
		return true
	})
	return ensureErr
}

// RejectUnsupportedMounts returns an explicit error for runtimes that cannot
// currently materialize workload mounts.
func RejectUnsupportedMounts(runtime deployv1.RuntimeKind, workload deployv1.Workload) error {
	if len(workload.Mounts) == 0 {
		return nil
	}
	return oopsx.B("runtime", "mount").Errorf("runtime %s does not support workload mounts yet", runtime)
}

func cleanMountTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", oopsx.B("runtime", "mount").Errorf("target is required")
	}
	if strings.HasPrefix(target, "/") {
		return path.Clean(target), nil
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return "", oopsx.B("runtime", "mount").Errorf("target must be absolute: %q", target)
}
