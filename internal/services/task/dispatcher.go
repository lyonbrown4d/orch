package task

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/arcgolabs/clientx"
	clientcodec "github.com/arcgolabs/clientx/codec"
	clientxhttp "github.com/arcgolabs/clientx/http"

	"github.com/lyonbrown4d/orch/internal/config"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/workerapi"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type WorkerDispatcher interface {
	DispatchWorkload(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (DispatchResult, error)
	StopWorkload(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (DispatchResult, error)
	WorkloadStatus(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (orchruntime.Status, error)
	WorkloadLogs(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload, opts orchruntime.LogOptions) (orchruntime.LogResult, error)
}

type DispatchResult struct {
	Accepted bool
	Node     string
	Address  string
	Status   string
	Workload string
}

type HTTPWorkerDispatcher struct {
	cfg config.Config
}

func NewHTTPWorkerDispatcher(cfg config.Config) *HTTPWorkerDispatcher {
	return &HTTPWorkerDispatcher{cfg: cfg}
}

func (d *HTTPWorkerDispatcher) DispatchWorkload(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (DispatchResult, error) {
	in := workerapi.DeployWorkloadInput{
		Body: workerapi.DeployWorkloadBody{
			Metadata: meta,
			Workload: workload,
			Node:     nodeID,
		},
	}
	return d.executeWorker(ctx, nodeID, workload.Name, "dispatch", workerapi.PathV1WorkerDeploy, in.Body, decodeWorkerDeploy)
}

func (d *HTTPWorkerDispatcher) StopWorkload(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (DispatchResult, error) {
	in := workerapi.StopWorkloadInput{
		Body: workerapi.StopWorkloadBody{
			Metadata: meta,
			Workload: workload,
			Node:     nodeID,
		},
	}
	return d.executeWorker(ctx, nodeID, workload.Name, "stop", workerapi.PathV1WorkerStop, in.Body, decodeWorkerStop)
}

func (d *HTTPWorkerDispatcher) WorkloadStatus(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload) (orchruntime.Status, error) {
	in := workerapi.WorkloadStatusInput{
		Body: workerapi.WorkloadStatusBody{
			Metadata: meta,
			Workload: workload,
			Node:     nodeID,
		},
	}
	raw, err := d.executeWorkerRaw(ctx, nodeID, workload.Name, "status", workerapi.PathV1WorkerStatus, in.Body)
	if err != nil {
		return orchruntime.Status{}, err
	}
	return decodeWorkerStatus(raw, workload)
}

func (d *HTTPWorkerDispatcher) WorkloadLogs(ctx context.Context, nodeID string, meta deployv1.Metadata, workload deployv1.Workload, opts orchruntime.LogOptions) (orchruntime.LogResult, error) {
	in := workerapi.WorkloadLogsInput{
		Body: workerapi.WorkloadLogsBody{
			Metadata: meta,
			Workload: workload,
			Node:     nodeID,
			Tail:     opts.Tail,
		},
	}
	raw, err := d.executeWorkerRaw(ctx, nodeID, workload.Name, "logs", workerapi.PathV1WorkerLogs, in.Body)
	if err != nil {
		return orchruntime.LogResult{}, err
	}
	return decodeWorkerLogs(raw, workload)
}

func decodeWorkerDeploy(data []byte, nodeID, fallbackWorkload string) (DispatchResult, error) {
	var out workerapi.DeployWorkloadOutput
	if err := clientcodec.JSON.Unmarshal(data, &out.Body); err != nil {
		return DispatchResult{}, oopsx.B("task", "worker").Wrapf(err, "decode worker deploy response")
	}
	status := strings.TrimSpace(out.Body.Status)
	if status == "" && out.Body.Accepted {
		status = "running"
	}
	node := strings.TrimSpace(out.Body.Node)
	if node == "" {
		node = nodeID
	}
	workloadName := strings.TrimSpace(out.Body.Workload)
	if workloadName == "" {
		workloadName = fallbackWorkload
	}
	return DispatchResult{
		Accepted: out.Body.Accepted,
		Node:     node,
		Address:  strings.TrimSpace(out.Body.Address),
		Status:   status,
		Workload: workloadName,
	}, nil
}

func decodeWorkerStop(data []byte, nodeID, fallbackWorkload string) (DispatchResult, error) {
	var out workerapi.StopWorkloadOutput
	if err := clientcodec.JSON.Unmarshal(data, &out.Body); err != nil {
		return DispatchResult{}, oopsx.B("task", "worker").Wrapf(err, "decode worker stop response")
	}
	status := strings.TrimSpace(out.Body.Status)
	if status == "" && out.Body.Accepted {
		status = "stopped"
	}
	node := strings.TrimSpace(out.Body.Node)
	if node == "" {
		node = nodeID
	}
	workloadName := strings.TrimSpace(out.Body.Workload)
	if workloadName == "" {
		workloadName = fallbackWorkload
	}
	return DispatchResult{
		Accepted: out.Body.Accepted,
		Node:     node,
		Status:   status,
		Workload: workloadName,
	}, nil
}

func decodeWorkerStatus(data []byte, workload deployv1.Workload) (orchruntime.Status, error) {
	var out workerapi.WorkloadStatusOutput
	if err := clientcodec.JSON.Unmarshal(data, &out.Body); err != nil {
		return orchruntime.Status{}, oopsx.B("task", "worker").Wrapf(err, "decode worker status response")
	}
	if out.Body.Name == "" {
		out.Body.Name = strings.TrimSpace(workload.Name)
	}
	if out.Body.Runtime == "" {
		out.Body.Runtime = workload.Runtime
	}
	return out.Body, nil
}

func decodeWorkerLogs(data []byte, workload deployv1.Workload) (orchruntime.LogResult, error) {
	var out workerapi.WorkloadLogsOutput
	if err := clientcodec.JSON.Unmarshal(data, &out.Body); err != nil {
		return orchruntime.LogResult{}, oopsx.B("task", "worker").Wrapf(err, "decode worker logs response")
	}
	if out.Body.Name == "" {
		out.Body.Name = strings.TrimSpace(workload.Name)
	}
	if out.Body.Runtime == "" {
		out.Body.Runtime = workload.Runtime
	}
	return out.Body, nil
}

func (d *HTTPWorkerDispatcher) executeWorker(
	ctx context.Context,
	nodeID string,
	workloadName string,
	action string,
	path string,
	body any,
	decode func([]byte, string, string) (DispatchResult, error),
) (DispatchResult, error) {
	raw, err := d.executeWorkerRaw(ctx, nodeID, workloadName, action, path, body)
	if err != nil {
		return DispatchResult{}, err
	}
	return decode(raw, nodeID, workloadName)
}

func (d *HTTPWorkerDispatcher) executeWorkerRaw(
	ctx context.Context,
	nodeID string,
	workloadName string,
	action string,
	path string,
	body any,
) ([]byte, error) {
	baseURL, ok := d.cfg.Cluster.NodeURL(nodeID)
	if !ok {
		return nil, oopsx.B("task", "worker").Errorf("no worker API URL configured for node %q (set cluster.nodes.%s)", nodeID, nodeID)
	}

	opts := []clientxhttp.Option{}
	if tok := strings.TrimSpace(d.cfg.Cluster.WorkerToken); tok != "" {
		opts = append(opts, clientxhttp.WithHeader("Authorization", "Bearer "+tok))
	}
	hc, err := clientxhttp.New(clientxhttp.Config{
		BaseURL: baseURL,
		Timeout: 60 * time.Second,
		Retry: clientx.RetryConfig{
			Enabled:    true,
			MaxRetries: 2,
			WaitMin:    200 * time.Millisecond,
			WaitMax:    2 * time.Second,
		},
	}, opts...)
	if err != nil {
		return nil, oopsx.B("task", "worker").Wrapf(err, "create worker client for node %q", nodeID)
	}
	defer closeWorkerClient(hc)

	raw, err := clientcodec.JSON.Marshal(body)
	if err != nil {
		return nil, oopsx.B("task", "worker").Wrapf(err, "encode worker %s", action)
	}
	req := hc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(raw)
	resp, err := hc.Execute(ctx, req, "POST", path)
	if err != nil {
		return nil, oopsx.B("task", "worker").Wrapf(err, "%s workload %q on node %q", action, workloadName, nodeID)
	}
	if !resp.IsStatusSuccess() {
		msg := strings.TrimSpace(string(resp.Bytes()))
		return nil, oopsx.B("task", "worker").Errorf("%s workload %q on node %q: %s: %s", action, workloadName, nodeID, resp.Status(), msg)
	}
	return resp.Bytes(), nil
}

func closeWorkerClient(hc clientxhttp.Client) {
	if err := hc.Close(); err != nil {
		slog.Default().Debug("worker client close", "error", err)
	}
}
