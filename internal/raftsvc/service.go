package raftsvc

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dragonboat "github.com/jeremyhahn/dragonboat/v4"
	dbconfig "github.com/jeremyhahn/dragonboat/v4/config"
	dbtan "github.com/jeremyhahn/dragonboat/v4/plugin/tan"
	sm "github.com/jeremyhahn/dragonboat/v4/statemachine"

	"github.com/lyonbrown4d/orch/internal/config"
	"github.com/lyonbrown4d/orch/internal/logging"
	"github.com/lyonbrown4d/orch/internal/nodeid"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

const (
	controlShardID uint64 = 1
)

// Service owns the Dragonboat NodeHost used for replicated control-plane state.
type Service struct {
	logger  *slog.Logger
	cfg     config.Config
	localID nodeid.Local

	nh             *dragonboat.NodeHost
	fsm            *schedulingFSM
	deploySignalCh chan struct{}

	localReplicaID uint64
	localAddress   string

	mu struct {
		sync.RWMutex
		replicaToNode map[uint64]string
		addressToNode map[string]string
	}

	started atomic.Bool
}

// New constructs the service (Dragonboat starts in Start).
func New(cfg config.Config, logger *slog.Logger, local nodeid.Local) *Service {
	logging.InstallDragonboatLogger(logger)
	ch := make(chan struct{}, 1)
	fsm := &schedulingFSM{}
	s := &Service{
		logger:         logger,
		cfg:            cfg,
		localID:        local,
		fsm:            fsm,
		deploySignalCh: ch,
	}
	s.mu.replicaToNode = map[uint64]string{}
	s.mu.addressToNode = map[string]string{}
	fsm.setNotifyDeploy(func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	})
	return s
}

func (s *Service) dragonboatDataDir() string {
	if dir := strings.TrimSpace(s.cfg.Raft.Data.Dir); dir != "" {
		return dir
	}
	return filepath.Join(config.DefaultDataRoot(), "dragonboat")
}

func (s *Service) dragonboatAddresses(ctx context.Context) (listenAddr, raftAddr string, err error) {
	bindAddr := strings.TrimSpace(s.cfg.Raft.Bind)
	if bindAddr == "" {
		return "", "", oopsx.B("raft").Errorf("raft.bind is required")
	}
	bindAddr, err = validateRaftPeerAddress("raft.bind", bindAddr)
	if err != nil {
		return "", "", oopsx.B("raft").Wrapf(err, "validate raft bind")
	}
	listenAddr, err = resolveEphemeralTCPAddr(ctx, bindAddr)
	if err != nil {
		return "", "", oopsx.B("raft").Wrapf(err, "reserve raft bind address")
	}
	advertise := strings.TrimSpace(s.cfg.Raft.Advertise)
	if advertise == "" {
		raftAddr, err = validateConcreteRaftAddress("raft.bind", listenAddr)
	} else {
		raftAddr, err = validateConcreteRaftAddress("raft.advertise", advertise)
	}
	if err != nil {
		return "", "", oopsx.B("raft").Wrapf(err, "validate raft address")
	}
	return listenAddr, raftAddr, nil
}

func (s *Service) dragonboatReplicaConfig(replicaID uint64) dbconfig.Config {
	return dbconfig.Config{
		ReplicaID:          replicaID,
		ShardID:            controlShardID,
		CheckQuorum:        true,
		PreVote:            true,
		HeartbeatRTT:       1,
		ElectionRTT:        10,
		SnapshotEntries:    10_000,
		CompactionOverhead: 1_000,
		WaitReady:          true,
	}
}

func (s *Service) startReplica(nh *dragonboat.NodeHost, replicaID uint64, raftAddr string) error {
	hasNodeInfo := nh.HasNodeInfo(controlShardID, replicaID)
	initialMembers := map[uint64]dragonboat.Target{}
	join := false
	if !hasNodeInfo {
		if s.cfg.Raft.Bootstrap {
			var err error
			initialMembers, err = s.bootstrapReplicaTargets(replicaID, raftAddr)
			if err != nil {
				return err
			}
		} else {
			join = true
		}
	}
	if err := nh.StartReplica(initialMembers, join, func(shardID, rid uint64) sm.IStateMachine {
		return s.fsm
	}, s.dragonboatReplicaConfig(replicaID)); err != nil {
		return oopsx.B("raft").Wrapf(err, "start dragonboat replica")
	}
	return nil
}

// Start opens a Dragonboat NodeHost and starts the control-plane Raft shard replica.
func (s *Service) Start(ctx context.Context) error {
	if s.started.Load() {
		return nil
	}

	replicaID, err := replicaIDForNodeID(s.localID.String())
	if err != nil {
		return err
	}
	listenAddr, raftAddr, err := s.dragonboatAddresses(ctx)
	if err != nil {
		return err
	}

	dataDir := s.dragonboatDataDir()
	if mkdirErr := os.MkdirAll(dataDir, 0o750); mkdirErr != nil {
		return oopsx.B("raft").Wrapf(mkdirErr, "dragonboat mkdir %q", dataDir)
	}

	nh, err := dragonboat.NewNodeHost(dbconfig.NodeHostConfig{
		WALDir:         filepath.Join(dataDir, "wal"),
		NodeHostDir:    filepath.Join(dataDir, "nodehost"),
		RTTMillisecond: 100,
		RaftAddress:    raftAddr,
		ListenAddress:  listenAddr,
		Expert: dbconfig.ExpertConfig{
			LogDBFactory: dbtan.Factory,
			FS:           dragonboatFS(),
		},
	})
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "dragonboat.NewNodeHost")
	}

	s.localReplicaID = replicaID
	s.localAddress = raftAddr
	s.rememberMember(s.localID.String(), replicaID, raftAddr)

	if err := s.startReplica(nh, replicaID, raftAddr); err != nil {
		nh.Close()
		return oopsx.B("raft").Wrapf(err, "dragonboat StartReplica")
	}

	s.nh = nh
	s.started.Store(true)

	s.logger.Info("raft started",
		"engine", "dragonboat",
		"node_id", s.localID.String(),
		"replica_id", replicaID,
		"bind", listenAddr,
		"advertise", raftAddr,
		"configured_peers", len(s.cfg.Raft.Peers),
		"data_dir", dataDir,
	)
	return nil
}

func (s *Service) isLocalLeader() bool {
	if s == nil || s.nh == nil || s.localReplicaID == 0 {
		return false
	}
	leaderID, _, ready, err := s.nh.GetLeaderID(controlShardID)
	return err == nil && ready && leaderID == s.localReplicaID
}

func (s *Service) LocalAddress() string {
	if s == nil {
		return ""
	}
	return s.localAddress
}

// WaitLocalLeader blocks until this node is the local Raft leader.
// If Dragonboat is not started, it returns immediately so FSM-only tests keep using the local apply path.
func (s *Service) WaitLocalLeader(ctx context.Context) error {
	if s == nil {
		return oopsx.B("raft").Errorf("nil service")
	}
	if s.nh == nil {
		return nil
	}
	if s.isLocalLeader() {
		return nil
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return oopsx.B("raft").Wrapf(ctx.Err(), "wait local leader")
		case <-ticker.C:
			if s.isLocalLeader() {
				return nil
			}
		}
	}
}

func (s *Service) applyCommand(ctx context.Context, data []byte, timeout time.Duration, notLeaderMessage string) error {
	if s == nil {
		return oopsx.B("raft").Errorf("nil service")
	}
	if s.nh == nil {
		// Tests can exercise the FSM without opening a Dragonboat NodeHost.
		// Production startup always calls Start before serving control-plane APIs.
		s.fsm.applyCommandPayload(data)
		return nil
	}
	if !s.isLocalLeader() {
		return s.notLeaderError(notLeaderMessage)
	}
	ctx, cancel := withDefaultDeadline(ctx, timeout)
	defer cancel()
	_, err := s.nh.SyncPropose(ctx, s.nh.GetNoOPSession(controlShardID), data)
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "dragonboat propose")
	}
	return nil
}

func (s *Service) notLeaderError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "raft local node is not leader"
	}
	if s == nil || s.nh == nil {
		return oopsx.B("raft").Errorf("%s: raft is not ready", message)
	}
	leaderID, _, ready, err := s.nh.GetLeaderID(controlShardID)
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "%s: read raft leader", message)
	}
	if !ready || leaderID == 0 {
		return oopsx.B("raft").Errorf("%s: raft leader is not ready", message)
	}
	return oopsx.B("raft").Errorf("%s: local node is follower, leader=%s", message, s.nodeIDForMember(leaderID, ""))
}

func withDefaultDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// Stop shuts Dragonboat down.
func (s *Service) Stop(_ context.Context) error {
	if s.nh != nil {
		s.nh.Close()
		s.nh = nil
	}
	if s.started.Load() {
		s.started.Store(false)
		s.logger.Info("raft stopped")
	}
	return nil
}
