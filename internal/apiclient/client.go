package apiclient

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/clientx"
	clientxhttp "github.com/arcgolabs/clientx/http"
	"github.com/samber/lo"

	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const defaultBaseURL = "http://127.0.0.1:17443"

// DefaultBaseURL returns ORCH_SERVER if set, else local dev default matching orch-server HTTP.Addr (:17443).
func DefaultBaseURL() string {
	return strings.TrimRight(lo.CoalesceOrEmpty(strings.TrimSpace(os.Getenv("ORCH_SERVER")), defaultBaseURL), "/")
}

// Client is the orch control-plane HTTP facade (github.com/arcgolabs/clientx/http → resty for transport).
// Request/response JSON uses clientx [clientcodec.JSON] (implements [clientcodec.Codec]); TCP/UDP use the same codecs via DialCodec. clientx/http leaves bodies to callers/resty.
type Client struct {
	hc clientxhttp.Client
}

// New builds a client with base URL and optional bearer token (Authorization header).
func New(baseURL, bearerToken string) (*Client, error) {
	var opts []clientxhttp.Option
	tok := strings.TrimSpace(bearerToken)
	if tok != "" {
		opts = append(opts, clientxhttp.WithHeader("Authorization", "Bearer "+tok))
	}
	hc, err := clientxhttp.New(clientxhttp.Config{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Timeout: 60 * time.Second,
		Retry: clientx.RetryConfig{
			Enabled:    true,
			MaxRetries: 2,
			WaitMin:    200 * time.Millisecond,
			WaitMax:    2 * time.Second,
		},
	}, opts...)
	if err != nil {
		return nil, oopsx.B("cli", "apiclient").Wrapf(err, "create clientx http client")
	}
	return &Client{hc: hc}, nil
}

// Close releases idle connections held by the underlying transport.
func (c *Client) Close() error {
	if c == nil || c.hc == nil {
		return nil
	}
	if err := c.hc.Close(); err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "close http client")
	}
	return nil
}

// Health calls GET /api/health.
func (c *Client) Health(ctx context.Context) (*api.HealthOutput, error) {
	var out api.HealthOutput
	if err := c.get(ctx, api.PathHealth, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// Ready calls GET /api/ready.
func (c *Client) Ready(ctx context.Context) (*api.ReadyOutput, error) {
	var out api.ReadyOutput
	if err := c.get(ctx, api.PathReady, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// Hostinfo calls GET /api/v1/hostinfo.
func (c *Client) Hostinfo(ctx context.Context) (*api.HostinfoOutput, error) {
	var out api.HostinfoOutput
	if err := c.get(ctx, api.PathV1Hostinfo, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// Diagnostics calls GET /api/v1/diagnostics.
func (c *Client) Diagnostics(ctx context.Context) (*api.DiagnosticsOutput, error) {
	var out api.DiagnosticsOutput
	if err := c.get(ctx, api.PathV1Diagnostics, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListRuntimeProviders calls GET /api/v1/system/runtimes.
func (c *Client) ListRuntimeProviders(ctx context.Context) (*api.ListRuntimeProvidersOutput, error) {
	var out api.ListRuntimeProvidersOutput
	if err := c.get(ctx, api.PathV1RuntimeProviders, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListWorkloads calls GET /api/v1/workloads.
func (c *Client) ListWorkloads(ctx context.Context) (*api.ListWorkloadsOutput, error) {
	var out api.ListWorkloadsOutput
	if err := c.get(ctx, api.PathV1Workloads, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAssignments calls GET /api/v1/assignments.
func (c *Client) ListAssignments(ctx context.Context) (*api.ListAssignmentsOutput, error) {
	var out api.ListAssignmentsOutput
	if err := c.get(ctx, api.PathV1Assignments, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVolumes calls GET /api/v1/volumes.
func (c *Client) ListVolumes(ctx context.Context) (*api.ListVolumesOutput, error) {
	var out api.ListVolumesOutput
	if err := c.get(ctx, api.PathV1Volumes, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListApps(ctx context.Context) (*api.ListAppsOutput, error) {
	var out api.ListAppsOutput
	if err := c.get(ctx, api.PathV1Apps, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetApp(ctx context.Context, namespace, name string) (*api.GetAppOutput, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, oopsx.B("cli", "apiclient").Errorf("empty app name")
	}
	namespace = workloadmeta.NamespaceOrDefault(namespace)
	path := api.PathV1Apps + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	var out api.GetAppOutput
	if err := c.get(ctx, path, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) WorkloadRuntimeStatus(ctx context.Context, namespace, app, workload string) (*api.WorkloadRuntimeStatusOutput, error) {
	namespace, app, workload, err := normalizeWorkloadPath(namespace, app, workload)
	if err != nil {
		return nil, err
	}
	path := api.PathV1Workloads + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(app) + "/" + url.PathEscape(workload) + "/status"
	var out api.WorkloadRuntimeStatusOutput
	if err := c.get(ctx, path, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) WorkloadLogs(ctx context.Context, namespace, app, workload string, tail int) (*api.WorkloadLogsOutput, error) {
	namespace, app, workload, err := normalizeWorkloadPath(namespace, app, workload)
	if err != nil {
		return nil, err
	}
	path := api.PathV1Workloads + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(app) + "/" + url.PathEscape(workload) + "/logs"
	if tail > 0 {
		path += "?tail=" + url.QueryEscape(strconv.Itoa(tail))
	}
	var out api.WorkloadLogsOutput
	if err := c.get(ctx, path, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RaftStatus(ctx context.Context) (*api.RaftStatusOutput, error) {
	var out api.RaftStatusOutput
	if err := c.get(ctx, api.PathV1RaftStatus, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func normalizeWorkloadPath(namespace, app, workload string) (string, string, string, error) {
	app = strings.TrimSpace(app)
	workload = strings.TrimSpace(workload)
	if app == "" {
		return "", "", "", oopsx.B("cli", "apiclient").Errorf("empty app name")
	}
	if workload == "" {
		return "", "", "", oopsx.B("cli", "apiclient").Errorf("empty workload name")
	}
	namespace = workloadmeta.NamespaceOrDefault(namespace)
	return namespace, app, workload, nil
}

func (c *Client) ListRaftMembers(ctx context.Context) (*api.ListRaftMembersOutput, error) {
	var out api.ListRaftMembersOutput
	if err := c.get(ctx, api.PathV1RaftMembers, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AddRaftVoter(ctx context.Context, id, address string) (*api.AddRaftMemberOutput, error) {
	id = strings.TrimSpace(id)
	address = strings.TrimSpace(address)
	if id == "" || address == "" {
		return nil, oopsx.B("cli", "apiclient").Errorf("raft member id and address are required")
	}
	var in api.AddRaftMemberInput
	in.Body.ID = id
	in.Body.Address = address
	var out api.AddRaftMemberOutput
	if err := c.post(ctx, api.PathV1RaftMembers, &in.Body, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RemoveRaftMember(ctx context.Context, id string) (*api.RemoveRaftMemberOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, oopsx.B("cli", "apiclient").Errorf("raft member id is required")
	}
	path := api.PathV1RaftMembers + "/" + url.PathEscape(id)
	var out api.RemoveRaftMemberOutput
	if err := c.delete(ctx, path, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrchVPNBootstrap calls GET /api/v1/orch-vpn/bootstrap.
func (c *Client) OrchVPNBootstrap(ctx context.Context) (*api.OrchVPNBootstrapOutput, error) {
	var out api.OrchVPNBootstrapOutput
	if err := c.get(ctx, api.PathV1OrchVPNBootstrap, &out.Body); err != nil {
		return nil, err
	}
	return &out, nil
}
