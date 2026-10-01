package raftsvc

import (
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	dragonboat "github.com/jeremyhahn/dragonboat/v4"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (s *Service) bootstrapReplicaTargets(localReplicaID uint64, localAddr string) (map[uint64]dragonboat.Target, error) {
	localID := strings.TrimSpace(s.localID.String())
	if localID == "" {
		return nil, oopsx.B("raft").Errorf("raft local id is required")
	}
	peers, err := s.bootstrapPeers(localID, localAddr)
	if err != nil {
		return nil, err
	}
	targets, err := s.replicaTargetsFromPeers(peers)
	if err != nil {
		return nil, err
	}
	ensureLocalReplicaTarget(targets, localReplicaID, localAddr)
	s.rememberMember(localID, localReplicaID, localAddr)
	return targets, nil
}

func (s *Service) bootstrapPeers(localID, localAddr string) (map[string]string, error) {
	peers := map[string]string{}
	for rawID, rawAddr := range s.cfg.Raft.Peers {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		addr, err := validateRaftPeerAddress("raft.peers."+id, rawAddr)
		if err != nil {
			return nil, oopsx.B("raft").Wrapf(err, "validate raft peer")
		}
		peers[id] = addr
	}
	s.warnIfLocalPeerAddressDiffers(peers, localID, localAddr)
	peers[localID] = localAddr
	return peers, nil
}

func (s *Service) warnIfLocalPeerAddressDiffers(peers map[string]string, localID, localAddr string) {
	configured, ok := peers[localID]
	if !ok || configured == localAddr {
		return
	}
	s.logger.Warn("raft peer address for local node differs from transport advertise address; using transport address",
		"node_id", localID,
		"configured", configured,
		"transport", localAddr,
	)
}

func (s *Service) replicaTargetsFromPeers(peers map[string]string) (map[uint64]dragonboat.Target, error) {
	targets := map[uint64]dragonboat.Target{}
	seenIDs := map[uint64]string{}
	for _, id := range sortedPeerIDs(peers) {
		replicaID, err := replicaIDForNodeID(id)
		if err != nil {
			return nil, err
		}
		if err := checkReplicaIDCollision(seenIDs, replicaID, id); err != nil {
			return nil, err
		}
		seenIDs[replicaID] = id
		targets[replicaID] = peers[id]
		s.rememberMember(id, replicaID, peers[id])
	}
	return targets, nil
}

func sortedPeerIDs(peers map[string]string) []string {
	ids := list.NewList(mapping.NewMapFrom(peers).Keys()...)
	ids.Sort(strings.Compare)
	return ids.Values()
}

func checkReplicaIDCollision(seenIDs map[uint64]string, replicaID uint64, id string) error {
	prev, exists := seenIDs[replicaID]
	if exists && prev != id {
		return oopsx.B("raft").Errorf("raft node ids %q and %q resolve to the same dragonboat replica id %d", prev, id, replicaID)
	}
	return nil
}

func ensureLocalReplicaTarget(targets map[uint64]dragonboat.Target, localReplicaID uint64, localAddr string) {
	if _, ok := targets[localReplicaID]; !ok {
		targets[localReplicaID] = localAddr
	}
}
