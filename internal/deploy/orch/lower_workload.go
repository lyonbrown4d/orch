package orch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/arcgolabs/mapper"
	"github.com/arcgolabs/plano/compiler"

	v1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func lowerWorkload(m *mapper.Mapper, f *compiler.HIRForm, defaults appDefaults) (v1.Workload, error) {
	workload, err := lowerWorkloadBase(f, defaults)
	if err != nil {
		return workload, err
	}
	for _, step := range workloadLoweringSteps(m, defaults) {
		if err := step(&workload, f); err != nil {
			return workload, fmt.Errorf("workload %q: %w", workload.Name, err)
		}
	}
	return workload, nil
}

type workloadLoweringStep func(*v1.Workload, *compiler.HIRForm) error

func workloadLoweringSteps(m *mapper.Mapper, defaults appDefaults) []workloadLoweringStep {
	return []workloadLoweringStep{
		fillWorkloadCore,
		func(workload *v1.Workload, form *compiler.HIRForm) error {
			return fillWorkloadRun(m, workload, form, defaults)
		},
		fillWorkloadEndpoints,
		fillWorkloadMounts,
		fillWorkloadEnv,
		func(workload *v1.Workload, form *compiler.HIRForm) error {
			return fillWorkloadResources(m, workload, form)
		},
		func(workload *v1.Workload, form *compiler.HIRForm) error {
			return fillWorkloadHealth(m, workload, form)
		},
		func(workload *v1.Workload, form *compiler.HIRForm) error {
			return fillWorkloadScheduling(m, workload, form)
		},
	}
}

func lowerWorkloadBase(f *compiler.HIRForm, defaults appDefaults) (v1.Workload, error) {
	name, err := symbolLabelName(f)
	if err != nil {
		return v1.Workload{}, fmt.Errorf("workload: %w", err)
	}
	kind, err := workloadKind(f)
	if err != nil {
		return v1.Workload{}, fmt.Errorf("workload %q: %w", name, err)
	}
	return v1.Workload{
		Name:    name,
		Kind:    kind,
		Runtime: workloadRuntime(f, defaults),
	}, nil
}

func fillWorkloadCore(workload *v1.Workload, f *compiler.HIRForm) error {
	if replicas, ok := intField(f, "replicas"); ok {
		workload.Replicas = replicas
	}
	if f.Fields == nil {
		return nil
	}
	if deps, ok := f.Fields.Get("depends_on"); ok {
		workload.DependsOn = workloadRefList(deps.Value)
	}
	return nil
}

func fillWorkloadRun(m *mapper.Mapper, workload *v1.Workload, f *compiler.HIRForm, defaults appDefaults) error {
	runs := childFormsByKind(f, "run")
	if len(runs) > 1 {
		return errors.New("at most one run block")
	}
	if len(runs) == 1 {
		fillRun(&workload.Run, &runs[0])
	}
	if err := fillRunFromFields(&workload.Run, f); err != nil {
		return err
	}
	if err := fillRuntimeOptions(m, &workload.Run.Options, f); err != nil {
		return err
	}
	if err := fillDockerOptionsFromFields(m, &workload.Run.Options, f); err != nil {
		return err
	}
	workload.Run.Options.Docker = mergeDockerOptionsForRuntime(workload.Runtime, defaults.Docker, workload.Run.Options.Docker)
	return nil
}

func fillWorkloadEndpoints(workload *v1.Workload, f *compiler.HIRForm) error {
	endpointForms := childFormsByKind(f, "endpoint")
	for i := range endpointForms {
		endpoint, err := lowerEndpoint(&endpointForms[i])
		if err != nil {
			return err
		}
		workload.Endpoints = append(workload.Endpoints, endpoint)
	}
	workload.Endpoints = append(workload.Endpoints, lowerEndpointCalls(f)...)
	return nil
}

func fillWorkloadMounts(workload *v1.Workload, f *compiler.HIRForm) error {
	mountForms := childFormsByKind(f, "mount")
	for i := range mountForms {
		mount, err := lowerMount(&mountForms[i])
		if err != nil {
			return err
		}
		workload.Mounts = append(workload.Mounts, mount)
	}
	return nil
}

func fillWorkloadEnv(workload *v1.Workload, f *compiler.HIRForm) error {
	if envMap, ok := stringMapField(f, "env"); ok {
		workload.Run.Env = append(workload.Run.Env, envVarsFromMap(envMap)...)
	}
	envForms := childFormsByKind(f, "env")
	for i := range envForms {
		env, err := lowerEnv(&envForms[i])
		if err != nil {
			return err
		}
		workload.Run.Env = append(workload.Run.Env, env)
	}
	return nil
}

func fillWorkloadResources(m *mapper.Mapper, workload *v1.Workload, f *compiler.HIRForm) error {
	resources, err := lowerWorkloadResources(m, f)
	if err != nil {
		return err
	}
	workload.Resources = resources
	return nil
}

func fillWorkloadScheduling(m *mapper.Mapper, workload *v1.Workload, f *compiler.HIRForm) error {
	sched := childFormsByKind(f, "scheduling")
	if len(sched) > 1 {
		return errors.New("at most one scheduling block")
	}
	workload.Scheduling = lowerSchedulingFromFields(m, f, workload.Kind == v1.WorkloadKindStateful)
	if len(sched) == 1 {
		workload.Scheduling = mergeScheduling(workload.Scheduling, lowerScheduling(m, &sched[0]))
	}
	return nil
}

func workloadKind(f *compiler.HIRForm) (v1.WorkloadKind, error) {
	switch f.Kind {
	case "service":
		return v1.WorkloadKindService, nil
	case "stateful":
		return v1.WorkloadKindStateful, nil
	case "worker":
		return v1.WorkloadKindWorker, nil
	}
	kindStr, ok := stringField(f, "kind")
	if !ok || strings.TrimSpace(kindStr) == "" {
		return "", errors.New("kind is required")
	}
	return v1.WorkloadKind(strings.ToLower(strings.TrimSpace(kindStr))), nil
}

func workloadRuntime(f *compiler.HIRForm, defaults appDefaults) v1.RuntimeKind {
	if rtStr, ok := stringField(f, "runtime"); ok && strings.TrimSpace(rtStr) != "" {
		return v1.RuntimeKind(strings.ToLower(strings.TrimSpace(rtStr)))
	}
	if defaults.Runtime != "" {
		return defaults.Runtime
	}
	return v1.RuntimeDocker
}
