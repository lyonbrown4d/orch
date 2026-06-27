package apiclient

import (
	"context"
	"net/url"
	"strings"

	"github.com/lyonbrown4d/orch/internal/api"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// Deploy calls POST /api/v1/deploy with a deploy DSL document.
func (c *Client) Deploy(ctx context.Context, app *deployv1.App) (*api.DeployOutput, error) {
	if app == nil {
		return nil, oopsx.B("cli", "apiclient").Errorf("nil app")
	}
	in := api.DeployInput{Body: *app}
	var out api.DeployOutput
	if err := c.post(ctx, api.PathV1Deploy, &in.Body, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeploySource calls POST /api/v1/deploy/source with manifest source; virtualPath should end with .orch or .yaml/.yml (or JSON-shaped YAML).
func (c *Client) DeploySource(ctx context.Context, virtualPath, source string) (*api.DeployOutput, error) {
	if virtualPath == "" {
		return nil, oopsx.B("cli", "apiclient").Errorf("empty virtualPath")
	}
	var in api.DeploySourceInput
	in.Body.VirtualPath = virtualPath
	in.Body.Source = source
	var out api.DeployOutput
	if err := c.post(ctx, api.PathV1DeploySource, &in.Body, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteDeploy(ctx context.Context, namespace, name string) (*api.DeleteDeployOutput, error) {
	path, err := deployActionPath(api.PathV1DeployDelete, namespace, name, "")
	if err != nil {
		return nil, err
	}
	var out api.DeleteDeployOutput
	if err := c.delete(ctx, path, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StopDeploy(ctx context.Context, namespace, name string) (*api.StopDeployOutput, error) {
	path, err := deployActionPath(api.PathV1DeployStop, namespace, name, "stop")
	if err != nil {
		return nil, err
	}
	var out api.StopDeployOutput
	if err := c.post(ctx, path, struct{}{}, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartDeploy(ctx context.Context, namespace, name string) (*api.StartDeployOutput, error) {
	path, err := deployActionPath(api.PathV1DeployStart, namespace, name, "start")
	if err != nil {
		return nil, err
	}
	var out api.StartDeployOutput
	if err := c.post(ctx, path, struct{}{}, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RestartDeploy(ctx context.Context, namespace, name string) (*api.RestartDeployOutput, error) {
	path, err := deployActionPath(api.PathV1DeployRestart, namespace, name, "restart")
	if err != nil {
		return nil, err
	}
	var out api.RestartDeployOutput
	if err := c.post(ctx, path, struct{}{}, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RollbackDeploy(ctx context.Context, namespace, name string) (*api.RollbackDeployOutput, error) {
	path, err := deployActionPath(api.PathV1DeployRollback, namespace, name, "rollback")
	if err != nil {
		return nil, err
	}
	var out api.RollbackDeployOutput
	if err := c.post(ctx, path, struct{}{}, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) MigrateDeploy(ctx context.Context, namespace, name, targetNode string, workloads []string) (*api.DeployOperationOutput, error) {
	return c.deployOperation(ctx, api.PathV1DeployMigrate, taskOperationMigrate, namespace, name, targetNode, workloads)
}

func (c *Client) FailoverDeploy(ctx context.Context, namespace, name, targetNode string, workloads []string) (*api.DeployOperationOutput, error) {
	return c.deployOperation(ctx, api.PathV1DeployFailover, taskOperationFailover, namespace, name, targetNode, workloads)
}

func (c *Client) RebalanceDeploy(ctx context.Context, namespace, name string, workloads []string) (*api.DeployOperationOutput, error) {
	return c.deployOperation(ctx, api.PathV1DeployRebalance, taskOperationRebalance, namespace, name, "", workloads)
}

const (
	taskOperationMigrate   = "migrate"
	taskOperationFailover  = "failover"
	taskOperationRebalance = "rebalance"
)

func deployActionPath(basePath, namespace, name, suffix string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", oopsx.B("cli", "apiclient").Errorf("empty app name")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "default"
	}
	path := basePath + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	if suffix != "" {
		path += "/" + suffix
	}
	return path, nil
}

func (c *Client) deployOperation(ctx context.Context, basePath, operation, namespace, name, targetNode string, workloads []string) (*api.DeployOperationOutput, error) {
	path, err := deployActionPath(basePath, namespace, name, operation)
	if err != nil {
		return nil, err
	}
	var in api.DeployOperationInput
	in.Body.TargetNode = targetNode
	in.Body.Workloads = workloads
	var out api.DeployOperationOutput
	if err := c.post(ctx, path, &in.Body, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}
