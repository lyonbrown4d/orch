package api

import (
	"context"
	"net/url"

	"github.com/arcgolabs/httpx"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/services/task"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type deployActionConfig struct {
	basePath    string
	suffix      string
	description string
	operationID string
	summary     string
	detail      string
	action      string
	status      string
	submit      func(context.Context, *task.Service, deployv1.Metadata) error
}

type deployActionEndpoint[I, O any] struct {
	tasks            *task.Service
	leader           *LeaderForwarder
	openAPIAuthApply bool
	cfg              deployActionConfig
	meta             func(*I) deployv1.Metadata
	output           func(DeployActionBody) *O
}

// StartDeployEndpoint serves POST /api/v1/deploy/{namespace}/{name}/start.
type StartDeployEndpoint = deployActionEndpoint[StartDeployInput, StartDeployOutput]

// StopDeployEndpoint serves POST /api/v1/deploy/{namespace}/{name}/stop.
type StopDeployEndpoint = deployActionEndpoint[StopDeployInput, StopDeployOutput]

// RestartDeployEndpoint serves POST /api/v1/deploy/{namespace}/{name}/restart.
type RestartDeployEndpoint = deployActionEndpoint[RestartDeployInput, RestartDeployOutput]

// RollbackDeployEndpoint serves POST /api/v1/deploy/{namespace}/{name}/rollback.
type RollbackDeployEndpoint = deployActionEndpoint[RollbackDeployInput, RollbackDeployOutput]

func NewStartDeployEndpoint(tasks *task.Service, leader *LeaderForwarder, openAPIAuthApply bool) *StartDeployEndpoint {
	return newDeployActionEndpoint(tasks, leader, openAPIAuthApply, startDeployActionConfig(), startDeployMeta, newStartDeployOutput)
}

func NewStopDeployEndpoint(tasks *task.Service, leader *LeaderForwarder, openAPIAuthApply bool) *StopDeployEndpoint {
	return newDeployActionEndpoint(tasks, leader, openAPIAuthApply, stopDeployActionConfig(), stopDeployMeta, newStopDeployOutput)
}

func NewRestartDeployEndpoint(tasks *task.Service, leader *LeaderForwarder, openAPIAuthApply bool) *RestartDeployEndpoint {
	return newDeployActionEndpoint(tasks, leader, openAPIAuthApply, restartDeployActionConfig(), restartDeployMeta, newRestartDeployOutput)
}

func NewRollbackDeployEndpoint(tasks *task.Service, leader *LeaderForwarder, openAPIAuthApply bool) *RollbackDeployEndpoint {
	return newDeployActionEndpoint(tasks, leader, openAPIAuthApply, rollbackDeployActionConfig(), rollbackDeployMeta, newRollbackDeployOutput)
}

func newDeployActionEndpoint[I, O any](
	tasks *task.Service,
	leader *LeaderForwarder,
	openAPIAuthApply bool,
	cfg deployActionConfig,
	meta func(*I) deployv1.Metadata,
	output func(DeployActionBody) *O,
) *deployActionEndpoint[I, O] {
	return &deployActionEndpoint[I, O]{
		tasks:            tasks,
		leader:           leader,
		openAPIAuthApply: openAPIAuthApply,
		cfg:              cfg,
		meta:             meta,
		output:           output,
	}
}

func (e *deployActionEndpoint[I, O]) EndpointSpec() httpx.EndpointSpec {
	spec := httpx.EndpointSpec{
		Prefix:      "/v1/deploy/{namespace}/{name}/" + e.cfg.suffix,
		Description: e.cfg.description,
		Tags:        httpx.Tags("deploy"),
	}
	if e.openAPIAuthApply {
		spec.Security = httpx.SecurityRequirements(httpx.SecurityRequirement("bearerAuth"))
	}
	return spec
}

func (e *deployActionEndpoint[I, O]) Register(r httpx.Registrar) {
	httpx.MustGroupPost(r.Scope(), "", e.handle, OpenAPIMeta([]string{"deploy"}, e.cfg.operationID, e.cfg.summary, e.cfg.detail))
}

func (e *deployActionEndpoint[I, O]) handle(ctx context.Context, in *I) (*O, error) {
	body := DeployActionBody{}
	if err := e.execute(ctx, e.meta(in), &body); err != nil {
		return nil, err
	}
	return e.output(body), nil
}

func (e *deployActionEndpoint[I, O]) execute(ctx context.Context, meta deployv1.Metadata, body *DeployActionBody) error {
	path := e.forwardPath(meta)
	if forwarded, err := e.leader.ForwardPost(ctx, path, struct{}{}, body); err != nil {
		return oopsx.B("api").Wrapf(err, "forward %s app", e.cfg.action)
	} else if forwarded {
		return nil
	}
	if err := e.cfg.submit(ctx, e.tasks, meta); err != nil {
		return oopsx.B("api").Wrapf(err, "%s app", e.cfg.action)
	}
	body.Accepted = true
	body.App = meta.Name
	body.Namespace = workloadmeta.NamespaceOrDefault(meta.Namespace)
	body.Status = e.cfg.status
	return nil
}

func (e *deployActionEndpoint[I, O]) forwardPath(meta deployv1.Metadata) string {
	return e.cfg.basePath + "/" + url.PathEscape(meta.Namespace) + "/" + url.PathEscape(meta.Name) + "/" + e.cfg.suffix
}

func startDeployActionConfig() deployActionConfig {
	return deployActionConfig{
		basePath:    PathV1DeployStart,
		suffix:      "start",
		description: "Start deploy workloads from retained desired state.",
		operationID: "startDeployApp",
		summary:     "Start a deploy app",
		detail:      "Starts the app workloads from its retained desired app document.",
		action:      "start",
		status:      workloadmeta.AssignmentStatusRunning,
		submit: func(ctx context.Context, tasks *task.Service, meta deployv1.Metadata) error {
			return tasks.SubmitStart(ctx, meta)
		},
	}
}

func stopDeployActionConfig() deployActionConfig {
	return deployActionConfig{
		basePath:    PathV1DeployStop,
		suffix:      "stop",
		description: "Stop deploy workloads while keeping desired state.",
		operationID: "stopDeployApp",
		summary:     "Stop a deploy app",
		detail:      "Stops the app workloads and records stopped scheduler assignments without deleting the desired app document.",
		action:      "stop",
		status:      workloadmeta.AssignmentStatusStopped,
		submit: func(ctx context.Context, tasks *task.Service, meta deployv1.Metadata) error {
			return tasks.SubmitStop(ctx, meta)
		},
	}
}

func restartDeployActionConfig() deployActionConfig {
	return deployActionConfig{
		basePath:    PathV1DeployRestart,
		suffix:      "restart",
		description: "Restart deploy workloads from retained desired state.",
		operationID: "restartDeployApp",
		summary:     "Restart a deploy app",
		detail:      "Stops then starts the app workloads using its retained desired app document.",
		action:      "restart",
		status:      workloadmeta.AssignmentStatusRunning,
		submit: func(ctx context.Context, tasks *task.Service, meta deployv1.Metadata) error {
			return tasks.SubmitRestart(ctx, meta)
		},
	}
}

func rollbackDeployActionConfig() deployActionConfig {
	return deployActionConfig{
		basePath:    PathV1DeployRollback,
		suffix:      "rollback",
		description: "Roll back deploy desired state to the previous revision.",
		operationID: "rollbackDeployApp",
		summary:     "Roll back a deploy app",
		detail:      "Restores the previous desired app revision stored in Raft and lets reconcile apply it. Followers forward to the known leader when configured.",
		action:      "rollback",
		status:      "accepted",
		submit: func(ctx context.Context, tasks *task.Service, meta deployv1.Metadata) error {
			return tasks.SubmitRollback(ctx, meta)
		},
	}
}
func startDeployMeta(in *StartDeployInput) deployv1.Metadata {
	return deployv1.Metadata{Name: in.Name, Namespace: in.Namespace}
}

func stopDeployMeta(in *StopDeployInput) deployv1.Metadata {
	return deployv1.Metadata{Name: in.Name, Namespace: in.Namespace}
}

func restartDeployMeta(in *RestartDeployInput) deployv1.Metadata {
	return deployv1.Metadata{Name: in.Name, Namespace: in.Namespace}
}

func rollbackDeployMeta(in *RollbackDeployInput) deployv1.Metadata {
	return deployv1.Metadata{Name: in.Name, Namespace: in.Namespace}
}

func newStartDeployOutput(body DeployActionBody) *StartDeployOutput {
	return &StartDeployOutput{Body: body}
}

func newStopDeployOutput(body DeployActionBody) *StopDeployOutput {
	return &StopDeployOutput{Body: body}
}

func newRestartDeployOutput(body DeployActionBody) *RestartDeployOutput {
	return &RestartDeployOutput{Body: body}
}

func newRollbackDeployOutput(body DeployActionBody) *RollbackDeployOutput {
	return &RollbackDeployOutput{Body: body}
}
