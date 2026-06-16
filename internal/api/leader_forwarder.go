package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	clientcodec "github.com/arcgolabs/clientx/codec"
	clientxhttp "github.com/arcgolabs/clientx/http"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type raftStatusProvider interface {
	Status(context.Context) (raftsvc.Status, error)
}

// LeaderForwarder forwards write requests received by a Raft follower to the
// current leader's HTTP API when cluster.nodes contains the leader node ID.
type LeaderForwarder struct {
	cfg  config.Config
	raft raftStatusProvider
}

func NewLeaderForwarder(cfg config.Config, raft raftStatusProvider) *LeaderForwarder {
	return &LeaderForwarder{
		cfg:  cfg,
		raft: raft,
	}
}

func (f *LeaderForwarder) ForwardPost(ctx context.Context, path string, body, out any) (bool, error) {
	return f.forwardJSON(ctx, http.MethodPost, path, body, out)
}

func (f *LeaderForwarder) ForwardDelete(ctx context.Context, path string, out any) (bool, error) {
	return f.forwardJSON(ctx, http.MethodDelete, path, nil, out)
}

func (f *LeaderForwarder) leaderBaseURL(ctx context.Context) (string, bool, error) {
	if f == nil || f.raft == nil {
		return "", false, nil
	}
	status, err := f.raft.Status(ctx)
	if err != nil {
		return "", false, oopsx.B("api", "raft").Wrapf(err, "get raft status")
	}
	if !status.Ready || status.IsLeader {
		return "", false, nil
	}
	leaderID := strings.TrimSpace(status.LeaderID)
	if leaderID == "" {
		return "", false, oopsx.B("api", "raft").Errorf("not leader and raft leader is not known")
	}
	baseURL, ok := f.cfg.Cluster.NodeURL(leaderID)
	if !ok {
		return "", false, oopsx.B("api", "raft").Errorf("not leader: configure cluster.nodes.%s or target the raft leader", leaderID)
	}
	return baseURL, true, nil
}

func (f *LeaderForwarder) forwardJSON(ctx context.Context, method, path string, body, out any) (bool, error) {
	baseURL, ok, err := f.leaderBaseURL(ctx)
	if err != nil || !ok {
		return ok, err
	}

	client, err := f.newLeaderClient(baseURL)
	if err != nil {
		return true, err
	}
	defer closeLeaderClient(client)

	resp, err := executeForwardedJSON(ctx, client, method, path, body)
	if err != nil {
		return true, err
	}
	if err := decodeForwardedJSON(resp, out); err != nil {
		return true, err
	}
	return true, nil
}

func (f *LeaderForwarder) newLeaderClient(baseURL string) (clientxhttp.Client, error) {
	var opts []clientxhttp.Option
	if tok := strings.TrimSpace(f.cfg.Cluster.WorkerToken); tok != "" {
		opts = append(opts, clientxhttp.WithHeader("Authorization", "Bearer "+tok))
	}
	client, err := clientxhttp.New(clientxhttp.Config{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Timeout: 60 * time.Second,
	}, opts...)
	if err != nil {
		return nil, oopsx.B("api", "raft").Wrapf(err, "create leader HTTP client")
	}
	return client, nil
}

func executeForwardedJSON(ctx context.Context, client clientxhttp.Client, method, path string, body any) (forwardedResponse, error) {
	req := client.R()
	if body != nil {
		raw, marshalErr := clientcodec.JSON.Marshal(body)
		if marshalErr != nil {
			return forwardedResponse{}, oopsx.B("api", "raft").Wrapf(marshalErr, "encode forwarded request")
		}
		req.SetHeader("Content-Type", "application/json").SetBody(raw)
	}

	resp, err := client.Execute(ctx, req, method, path)
	if err != nil {
		return forwardedResponse{}, oopsx.B("api", "raft").Wrapf(err, "forward request to raft leader")
	}
	return forwardedResponse{status: resp.Status(), body: resp.Bytes(), success: resp.IsStatusSuccess()}, nil
}

type forwardedResponse struct {
	status  string
	body    []byte
	success bool
}

func decodeForwardedJSON(resp forwardedResponse, out any) error {
	if !resp.success {
		msg := strings.TrimSpace(string(resp.body))
		return oopsx.B("api", "raft").Errorf("forwarded request failed: %s: %s", resp.status, msg)
	}
	if out == nil || len(resp.body) == 0 {
		return nil
	}
	if err := clientcodec.JSON.Unmarshal(resp.body, out); err != nil {
		return oopsx.B("api", "raft").Wrapf(err, "decode forwarded response")
	}
	return nil
}

func closeLeaderClient(client clientxhttp.Client) {
	if err := client.Close(); err != nil {
		return
	}
}
