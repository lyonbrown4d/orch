package config

import (
	"path/filepath"
	"strings"

	"github.com/arcgolabs/configx"
	formathcl "github.com/arcgolabs/configx/format/hcl"
)

const (
	RuntimeProviderPolicyAuto     = "auto"
	RuntimeProviderPolicyRequired = "required"
	RuntimeProviderPolicyDisabled = "disabled"
)

type RuntimeConfig struct {
	Providers map[string]RuntimeProviderConfig `json:"providers,omitempty"`
}

type RuntimeProviderConfig struct {
	Policy string `json:"policy,omitempty"`
}

func (c RuntimeConfig) ProviderPolicy(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(c.Providers) == 0 {
		return RuntimeProviderPolicyAuto
	}
	policy := strings.ToLower(strings.TrimSpace(c.Providers[name].Policy))
	switch policy {
	case RuntimeProviderPolicyRequired, RuntimeProviderPolicyDisabled:
		return policy
	default:
		return RuntimeProviderPolicyAuto
	}
}

type SchedulerConfig struct {
	HeartbeatInterval       string `json:"heartbeat_interval,omitempty"`
	ResourceRefreshInterval string `json:"resource_refresh_interval,omitempty"` // cadence for leader to apply local host metrics into Raft
	RaftLeaderOnly          bool   `json:"raft_leader_only,omitempty"`
	MaxConcurrentJobs       uint   `json:"max_concurrent_jobs,omitempty"`
	ConcurrentJobsMode      string `json:"concurrent_jobs_mode,omitempty"`
}

type ClusterConfig struct {
	// Nodes maps node ids to worker API base URLs, for example {"node-a": "http://10.0.0.11:17443"}.
	Nodes map[string]string `json:"nodes,omitempty"`
	// WorkerToken is sent as a bearer token for worker dispatch when remote nodes require HTTP auth.
	WorkerToken string `json:"worker_token,omitempty"`
}

type GossipConfig struct {
	Enabled           bool     `json:"enabled,omitempty"`
	Bind              string   `json:"bind,omitempty"`
	Advertise         string   `json:"advertise,omitempty"`
	Seeds             []string `json:"seeds,omitempty"`
	SecretKey         string   `json:"secret_key,omitempty"`
	APIURL            string   `json:"api_url,omitempty"`
	AutoJoinRaft      bool     `json:"auto_join_raft,omitempty"`
	ReconcileInterval string   `json:"reconcile_interval,omitempty"`
}

func (c ClusterConfig) NodeURL(nodeID string) (string, bool) {
	id := strings.TrimSpace(nodeID)
	if id == "" || len(c.Nodes) == 0 {
		return "", false
	}
	for k, v := range c.Nodes {
		if strings.TrimSpace(k) != id {
			continue
		}
		url := strings.TrimRight(strings.TrimSpace(v), "/")
		if url == "" {
			return "", false
		}
		return url, true
	}
	return "", false
}

// AuthConfig matches paths auth.enabled and auth.jwt.secret.
type AuthConfig struct {
	Enabled bool `json:"enabled"`
	JWT     struct {
		Secret string `json:"secret"`
	} `json:"jwt"`
}

// RaftConfig matches raft.node.id, raft.data.dir, raft.bind, raft.peers, etc.
type RaftConfig struct {
	Node struct {
		// ID forces this Raft/server identity (omit, "", or "auto" → resolved via nodeid hardware resolver).
		ID string `json:"id"`
	} `json:"node"`
	Bind      string            `json:"bind"`
	Advertise string            `json:"advertise,omitempty"`
	Peers     map[string]string `json:"peers,omitempty"`
	Bootstrap bool              `json:"bootstrap,omitempty"`
	Data      struct {
		Dir string `json:"dir"`
	} `json:"data"`
}

// Load merges defaults, dotenv, optional files, env (ORCH_), then CLI flags when passed via [LoadFromCobra].
func Load(opts ...configx.Option) (Config, error) {
	base := make([]configx.Option, 0, 5+len(opts))
	base = append(base,
		configx.WithTypedDefaults(Default()),
		formathcl.WithHCLSupport(),
		configx.WithEnvPrefix("ORCH"),
		configx.WithPriority(
			configx.SourceDotenv,
			configx.SourceFile,
			configx.SourceEnv,
			configx.SourceArgs,
		),
		configx.WithValidateLevel(configx.ValidateLevelNone),
	)
	return configx.Load[Config](append(base, opts...)...)
}

func Default() Config {
	root := DefaultDataRoot()

	var dns DNSConfig
	dns.Enabled = true
	dns.Listen = "127.0.0.1:15353"
	dns.Data.Path = filepath.Join(root, "dnsx.db")
	dns.Zone = "orch.local"

	var obs ObservabilityConfig
	obs.Prometheus.Enabled = true
	obs.Prometheus.Path = "/metrics"

	var auth AuthConfig
	auth.Enabled = false
	auth.JWT.Secret = "dev-secret-change-me"

	var raft RaftConfig
	// raft.Node.ID empty or "auto" → resolved from hardware at runtime ([nodeid.Resolve]).
	raft.Node.ID = ""
	raft.Bind = "127.0.0.1:7444"
	raft.Advertise = ""
	raft.Peers = map[string]string{}
	raft.Bootstrap = true
	raft.Data.Dir = filepath.Join(root, "dragonboat")

	var gossip GossipConfig
	gossip.Enabled = false
	gossip.Bind = "0.0.0.0:7946"
	gossip.Advertise = ""
	gossip.Seeds = []string{}
	gossip.SecretKey = ""
	gossip.APIURL = ""
	gossip.AutoJoinRaft = true
	gossip.ReconcileInterval = "5s"

	runtimeCfg := RuntimeConfig{
		Providers: map[string]RuntimeProviderConfig{},
	}

	return Config{
		App: AppConfig{
			Name: "orch",
		},
		Env: "dev",
		Log: LogConfig{
			Level: "debug",
		},
		HTTP: HTTPConfig{
			Addr: ":17443",
		},
		Observability: obs,
		Ingress: IngressConfig{
			Enabled: true,
			Listen:  []string{":80", ":443"},
		},
		DNS: dns,
		OrchVPN: OrchVPNConfig{
			Enabled:         false,
			TunnelListenUDP: ":15888",
		},
		Runtime: runtimeCfg,
		Scheduler: SchedulerConfig{
			HeartbeatInterval:       "2m",
			ResourceRefreshInterval: "30s",
			RaftLeaderOnly:          false,
			MaxConcurrentJobs:       0,
			ConcurrentJobsMode:      "reschedule",
		},
		Cluster: ClusterConfig{
			Nodes: map[string]string{},
		},
		Gossip: gossip,
		Auth:   auth,
		Raft:   raft,
	}
}
