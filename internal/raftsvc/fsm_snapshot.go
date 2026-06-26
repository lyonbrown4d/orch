package raftsvc

import (
	"encoding/json"
	"io"

	sm "github.com/lni/dragonboat/v4/statemachine"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (f *schedulingFSM) SaveSnapshot(w io.Writer, _ sm.ISnapshotFileCollection, _ <-chan struct{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := json.Marshal(f.state)
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "snapshot marshal")
	}
	if _, err := w.Write(b); err != nil {
		return oopsx.B("raft").Wrapf(err, "snapshot write")
	}
	return nil
}

func (f *schedulingFSM) RecoverFromSnapshot(r io.Reader, _ []sm.SnapshotFile, _ <-chan struct{}) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return oopsx.B("raft").Wrapf(err, "fsm restore read snapshot")
	}
	var st fsmSnapshotState
	if len(data) > 0 {
		if err := json.Unmarshal(data, &st); err != nil {
			return oopsx.B("raft").Wrapf(err, "fsm restore unmarshal")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = st
	return nil
}

func (f *schedulingFSM) Close() error {
	return nil
}
