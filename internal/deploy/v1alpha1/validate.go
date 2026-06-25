package v1alpha1

import (
	"regexp"
	"strings"

	"github.com/arcgolabs/collectionx/set"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

var (
	nameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._-]{0,127}$`)
)

func (a *App) Validate() error {
	if err := a.validateMetadata(); err != nil {
		return err
	}

	seenWorkloads := set.NewSet[string]()
	var validateErr error
	a.WorkloadList().Range(func(i int, workload Workload) bool {
		w := workload
		if err := (&w).validate(seenWorkloads); err != nil {
			validateErr = oopsx.B("deploy").Wrapf(err, "workloads[%d]", i)
			return false
		}
		return true
	})
	if validateErr != nil {
		return validateErr
	}

	if err := a.validateWorkloadCrossRefs(); err != nil {
		return err
	}
	if err := a.validateIngresses(seenWorkloads); err != nil {
		return err
	}
	return nil
}

func (a *App) validateMetadata() error {
	if strings.TrimSpace(a.Metadata.Name) == "" {
		return oopsx.B("deploy").Errorf("metadata.name is required")
	}
	if !nameRe.MatchString(a.Metadata.Name) {
		return oopsx.B("deploy").Errorf("metadata.name is invalid: %q", a.Metadata.Name)
	}
	if a.Metadata.Namespace != "" && !nameRe.MatchString(a.Metadata.Namespace) {
		return oopsx.B("deploy").Errorf("metadata.namespace is invalid: %q", a.Metadata.Namespace)
	}
	return nil
}

func (a *App) validateWorkloadCrossRefs() error {
	if err := a.validateWorkloadDepends(); err != nil {
		return err
	}
	if err := a.validateWorkloadMounts(); err != nil {
		return err
	}
	return a.validateWorkloadEndpointsAll()
}

func (a *App) validateWorkloadDepends() error {
	_, err := a.WorkloadsInDependencyOrder()
	return err
}

func (a *App) validateWorkloadMounts() error {
	var validateErr error
	a.WorkloadList().Range(func(i int, workload Workload) bool {
		workload.MountList().Range(func(j int, mount Mount) bool {
			if strings.TrimSpace(mount.Volume.Name) == "" {
				validateErr = oopsx.B("deploy").Errorf("workloads[%d].mounts[%d].volume: name is required", i, j)
				return false
			}
			if strings.TrimSpace(mount.Target) == "" {
				validateErr = oopsx.B("deploy").Errorf("workloads[%d].mounts[%d].target: target is required", i, j)
				return false
			}
			return true
		})
		return validateErr == nil
	})
	if validateErr != nil {
		return validateErr
	}
	return nil
}

func (a *App) validateWorkloadEndpointsAll() error {
	var validateErr error
	a.WorkloadList().Range(func(i int, workload Workload) bool {
		workload.EndpointList().Range(func(j int, endpoint Endpoint) bool {
			if err := endpoint.validate(); err != nil {
				validateErr = oopsx.B("deploy").Wrapf(err, "workloads[%d].endpoints[%d]", i, j)
				return false
			}
			return true
		})
		return validateErr == nil
	})
	if validateErr != nil {
		return validateErr
	}
	return nil
}

func (a *App) validateIngresses(seenWorkloads *set.Set[string]) error {
	var validateErr error
	a.IngressList().Range(func(i int, ing Ingress) bool {
		if err := a.validateIngressOne(i, ing, seenWorkloads); err != nil {
			validateErr = err
			return false
		}
		return true
	})
	return validateErr
}

func (a *App) validateIngressOne(ingIndex int, ing Ingress, seenWorkloads *set.Set[string]) error {
	if strings.TrimSpace(ing.Name) == "" {
		return oopsx.B("deploy").Errorf("ingresses[%d].name is required", ingIndex)
	}
	var validateErr error
	ing.RouteList().Range(func(j int, r IngressRoute) bool {
		if strings.TrimSpace(r.Path) == "" {
			validateErr = oopsx.B("deploy").Errorf("ingresses[%d].routes[%d].path is required", ingIndex, j)
			return false
		}
		if strings.TrimSpace(r.Backend.Workload) == "" || strings.TrimSpace(r.Backend.Endpoint) == "" {
			validateErr = oopsx.B("deploy").Errorf("ingresses[%d].routes[%d].backend must specify workload + endpoint", ingIndex, j)
			return false
		}
		if !seenWorkloads.Contains(r.Backend.Workload) {
			validateErr = oopsx.B("deploy").Errorf("ingresses[%d].routes[%d].backend: unknown workload %q", ingIndex, j, r.Backend.Workload)
			return false
		}
		return true
	})
	if validateErr != nil {
		return validateErr
	}
	return nil
}

func (w *Workload) validate(seen *set.Set[string]) error {
	validators := []func() error{
		func() error { return w.validateName(seen) },
		w.validateKind,
		w.validateRuntime,
		w.validateRunForRuntime,
		w.validateEnv,
		w.validateReplicaCount,
		w.validateResources,
	}
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workload) validateName(seen *set.Set[string]) error {
	if strings.TrimSpace(w.Name) == "" {
		return oopsx.B("deploy").Errorf("name is required")
	}
	if !nameRe.MatchString(w.Name) {
		return oopsx.B("deploy").Errorf("name is invalid: %q", w.Name)
	}
	if seen.Contains(w.Name) {
		return oopsx.B("deploy").Errorf("duplicate workload name %q", w.Name)
	}
	seen.Add(w.Name)
	return nil
}

func (w *Workload) validateKind() error {
	switch w.Kind {
	case WorkloadKindService, WorkloadKindWorker, WorkloadKindJob, WorkloadKindCron, WorkloadKindStateful:
		return nil
	default:
		return oopsx.B("deploy").Errorf("invalid kind %q", w.Kind)
	}
}

func (w *Workload) validateRuntime() error {
	switch w.Runtime {
	case RuntimeDocker, RuntimeContainerd, RuntimePodman, RuntimeFirecracker, RuntimeProcess, RuntimeSystemd, RuntimeWindowsService:
		return nil
	default:
		return oopsx.B("deploy").Errorf("invalid runtime %q", w.Runtime)
	}
}

func (w *Workload) validateEnv() error {
	var validateErr error
	w.EnvList().Range(func(i int, env EnvVar) bool {
		if strings.TrimSpace(env.Name) == "" {
			validateErr = oopsx.B("deploy").Errorf("run.env[%d].name is required", i)
			return false
		}
		return true
	})
	return validateErr
}

func (w *Workload) validateReplicaCount() error {
	if w.Replicas < 0 {
		return oopsx.B("deploy").Errorf("replicas must be >= 0")
	}
	return nil
}

func (w *Workload) validateResources() error {
	if w.Resources == nil {
		return nil
	}
	if w.Resources.CPUMillis < 0 {
		return oopsx.B("deploy").Errorf("resources.cpuMillis must be >= 0")
	}
	if w.Resources.MemoryBytes < 0 {
		return oopsx.B("deploy").Errorf("resources.memoryBytes must be >= 0")
	}
	return nil
}

func (e *Endpoint) validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return oopsx.B("deploy").Errorf("name is required")
	}
	if !nameRe.MatchString(e.Name) {
		return oopsx.B("deploy").Errorf("name is invalid: %q", e.Name)
	}
	if e.Port <= 0 || e.Port > 65535 {
		return oopsx.B("deploy").Errorf("port must be 1..65535 (got %d)", e.Port)
	}
	switch e.Protocol {
	case ProtoTCP, ProtoUDP, ProtoHTTP:
	default:
		return oopsx.B("deploy").Errorf("invalid protocol %q", e.Protocol)
	}
	return nil
}
