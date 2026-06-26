package orch

import (
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/plano/schema"
)

func workloadFormSpecs() []schema.FormSpec {
	return []schema.FormSpec{
		workloadFormSpec(),
		shorthandWorkloadForm("service", "Service workload. Short for workload kind=service."),
		shorthandWorkloadForm("stateful", "Stateful workload. Short for workload kind=stateful and scheduling.stateful=true."),
		shorthandWorkloadForm("worker", "Worker workload. Short for workload kind=worker."),
		runFormSpec(),
		endpointFormSpec(),
		mountFormSpec(),
		envFormSpec(),
		resourcesFormSpec(),
		healthFormSpec(),
		lifecycleFormSpec(),
		readinessFormSpec(),
		livenessFormSpec(),
		startupFormSpec(),
		{
			Name:        "runtime_options",
			LabelKind:   schema.LabelNone,
			BodyMode:    schema.BodyFormOnly,
			NestedForms: schema.NestedForms("docker", "containerd", "podman", "firecracker", "process", "systemd", "windows_service"),
		},
		schedulingFormSpec(),
	}
}

func workloadFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:         "workload",
		LabelKind:    schema.LabelSymbol,
		LabelRefKind: "workload",
		Declares:     "workload",
		BodyMode:     schema.BodyScript,
		Docs:         "Named workload (label is the workload name).",
		Fields: schema.Fields(
			schema.FieldSpec{Name: "kind", Type: schema.TypeString, Required: true},
			schema.FieldSpec{Name: "runtime", Type: schema.TypeString, Required: true},
			schema.FieldSpec{Name: "replicas", Type: schema.TypeInt, Default: 0, HasDefault: true},
			dependsOnFieldSpec(),
			schema.FieldSpec{Name: "restart_policy", Type: schema.TypeString},
			schema.FieldSpec{Name: "max_restarts", Type: schema.TypeInt},
			schema.FieldSpec{Name: "restart_delay", Type: schema.TypeString},
		),
		NestedForms: workloadNestedForms(),
	}
}

func shorthandWorkloadForm(name, docs string) schema.FormSpec {
	return schema.FormSpec{
		Name:         name,
		LabelKind:    schema.LabelSymbol,
		LabelRefKind: "workload",
		Declares:     "workload",
		BodyMode:     schema.BodyScript,
		Docs:         docs,
		Fields:       shorthandWorkloadFields(),
		NestedForms:  workloadNestedForms(),
	}
}

func shorthandWorkloadFields() *mapping.OrderedMap[string, schema.FieldSpec] {
	return schema.Fields(
		schema.FieldSpec{Name: "runtime", Type: schema.TypeString},
		schema.FieldSpec{Name: "image", Type: schema.TypeString},
		schema.FieldSpec{Name: "path", Type: schema.TypeString},
		schema.FieldSpec{Name: "url", Type: schema.TypeString},
		schema.FieldSpec{Name: "command", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
		schema.FieldSpec{Name: "args", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
		schema.FieldSpec{Name: "cwd", Type: schema.TypeString},
		schema.FieldSpec{Name: "user", Type: schema.TypeString},
		schema.FieldSpec{Name: "env", Type: schema.MapType{Elem: schema.TypeString}},
		schema.FieldSpec{Name: "resources", Type: schema.TypeString, Docs: `Compact "cpu/memory", e.g. "500m/512Mi".`},
		schema.FieldSpec{Name: "cpu_millis", Type: schema.TypeInt},
		schema.FieldSpec{Name: "memory_bytes", Type: schema.TypeInt},
		schema.FieldSpec{Name: "replicas", Type: schema.TypeInt, Default: 0, HasDefault: true},
		dependsOnFieldSpec(),
		schema.FieldSpec{Name: "restart_policy", Type: schema.TypeString},
		schema.FieldSpec{Name: "max_restarts", Type: schema.TypeInt},
		schema.FieldSpec{Name: "restart_delay", Type: schema.TypeString},
		schema.FieldSpec{Name: "network", Type: schema.TypeString, Docs: "Docker network_mode shorthand."},
		schema.FieldSpec{Name: "network_mode", Type: schema.TypeString},
		schema.FieldSpec{Name: "privileged", Type: schema.TypeBool, Default: false, HasDefault: true},
		schema.FieldSpec{Name: "labels", Type: schema.MapType{Elem: schema.TypeString}},
		schema.FieldSpec{Name: "allow_leader", Type: schema.TypeBool, Default: false, HasDefault: true},
		schema.FieldSpec{Name: "preferred_nodes", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
	)
}

func dependsOnFieldSpec() schema.FieldSpec {
	return schema.FieldSpec{
		Name:       "depends_on",
		Type:       schema.ListType{Elem: schema.RefType{Kind: "workload"}},
		Default:    []any{},
		HasDefault: true,
		Docs:       "Depends-on edges to other workloads.",
	}
}

func runFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "run",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "image", Type: schema.TypeString},
			schema.FieldSpec{Name: "path", Type: schema.TypeString},
			schema.FieldSpec{Name: "url", Type: schema.TypeString},
			schema.FieldSpec{Name: "command", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
			schema.FieldSpec{Name: "args", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
			schema.FieldSpec{Name: "cwd", Type: schema.TypeString},
			schema.FieldSpec{Name: "user", Type: schema.TypeString},
		),
	}
}

func endpointFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:         "endpoint",
		LabelKind:    schema.LabelSymbol,
		LabelRefKind: "endpoint",
		Declares:     "endpoint",
		BodyMode:     schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "port", Type: schema.TypeInt, Required: true},
			schema.FieldSpec{Name: "protocol", Type: schema.TypeString, Required: true},
		),
	}
}

func mountFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "mount",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "volume", Type: schema.TypeString, Required: true},
			schema.FieldSpec{Name: "target", Type: schema.TypeString, Required: true},
			schema.FieldSpec{Name: "read_only", Type: schema.TypeBool, Default: false, HasDefault: true},
		),
	}
}

func envFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "env",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "name", Type: schema.TypeString, Required: true},
			schema.FieldSpec{Name: "value", Type: schema.TypeString, Required: true},
		),
	}
}

func resourcesFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "resources",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "cpu_millis", Type: schema.TypeInt},
			schema.FieldSpec{Name: "memory_bytes", Type: schema.TypeInt},
		),
	}
}

func healthFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:        "health",
		LabelKind:   schema.LabelNone,
		BodyMode:    schema.BodyMixed,
		Fields:      probeFieldSpecs(),
		NestedForms: schema.NestedForms("readiness", "liveness", "startup"),
	}
}

func readinessFormSpec() schema.FormSpec {
	return probeFormSpec("readiness")
}

func livenessFormSpec() schema.FormSpec {
	return probeFormSpec("liveness")
}

func startupFormSpec() schema.FormSpec {
	return probeFormSpec("startup")
}

func probeFormSpec(name string) schema.FormSpec {
	return schema.FormSpec{
		Name:      name,
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields:    probeFieldSpecs(),
	}
}

func probeFieldSpecs() *mapping.OrderedMap[string, schema.FieldSpec] {
	return schema.Fields(
		schema.FieldSpec{Name: "http", Type: schema.TypeString, Docs: "HTTP readiness path, for example /ready."},
		schema.FieldSpec{Name: "path", Type: schema.TypeString, Docs: "Alias for http path."},
		schema.FieldSpec{Name: "tcp", Type: schema.TypeBool, Default: false, HasDefault: true},
		schema.FieldSpec{Name: "endpoint", Type: schema.TypeString, Docs: "Local workload endpoint name."},
		schema.FieldSpec{Name: "port", Type: schema.TypeInt},
		schema.FieldSpec{Name: "interval", Type: schema.TypeString},
		schema.FieldSpec{Name: "timeout", Type: schema.TypeString},
		schema.FieldSpec{Name: "retries", Type: schema.TypeInt},
		schema.FieldSpec{Name: "start_period", Type: schema.TypeString},
	)
}
func lifecycleFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "lifecycle",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "restart_policy", Type: schema.TypeString},
			schema.FieldSpec{Name: "max_restarts", Type: schema.TypeInt},
			schema.FieldSpec{Name: "restart_delay", Type: schema.TypeString},
		),
	}
}

func schedulingFormSpec() schema.FormSpec {
	return schema.FormSpec{
		Name:      "scheduling",
		LabelKind: schema.LabelNone,
		BodyMode:  schema.BodyFieldOnly,
		Fields: schema.Fields(
			schema.FieldSpec{Name: "stateful", Type: schema.TypeBool, Default: false, HasDefault: true},
			schema.FieldSpec{Name: "allow_leader", Type: schema.TypeBool, Default: false, HasDefault: true},
			schema.FieldSpec{Name: "preferred_nodes", Type: schema.ListType{Elem: schema.TypeString}, Default: []any{}, HasDefault: true},
		),
	}
}

func workloadNestedForms() *set.Set[string] {
	return schema.NestedForms("run", "runtime_options", "docker", "containerd", "podman", "firecracker", "process", "systemd", "windows_service", "endpoint", "mount", "env", "resources", "health", "lifecycle", "scheduling")
}
