package raftsvc

import (
	"encoding/json"
	"strings"
	"sync"

	sm "github.com/lni/dragonboat/v4/statemachine"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/nodecapacity"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

const (
	cmdUpsertNodeCapacity       = "upsert_node_capacity"
	cmdUpsertDeployApp          = "upsert_deploy_app"
	cmdDeleteDeployApp          = "delete_deploy_app"
	cmdUpsertWorkloadAssignment = "upsert_workload_assignment"
	cmdUpsertVolumeBinding      = "upsert_volume_binding"
)

// schedulingFSM holds replicated control-plane state (node capacity snapshots, etc.).
type schedulingFSM struct {
	mu           sync.Mutex
	state        fsmSnapshotState
	notifyDeploy func()
}

type fsmSnapshotState struct {
	AppliedCommands    uint64                             `json:"appliedCommands"`
	NodeCapacity       map[string]nodecapacity.Snapshot   `json:"nodeCapacity,omitempty"`
	DeployApps         map[string]deployv1.App            `json:"deployApps,omitempty"`
	DeployAppRevisions map[string][]DeployAppRevision     `json:"deployAppRevisions,omitempty"`
	Assignments        map[string]workloadmeta.Assignment `json:"assignments,omitempty"`
	VolumeBindings     map[string]volumemeta.Binding      `json:"volumeBindings,omitempty"`
}

func (f *schedulingFSM) setNotifyDeploy(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notifyDeploy = fn
}

func (f *schedulingFSM) Update(entry sm.Entry) (sm.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.AppliedCommands++
	if len(entry.Cmd) == 0 {
		return sm.Result{Value: f.state.AppliedCommands}, nil
	}
	f.applyPayloadLocked(entry.Cmd)
	return sm.Result{Value: f.state.AppliedCommands}, nil
}

func (f *schedulingFSM) Lookup(any) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}

// applyCommandPayload applies a replicated (or local single-node) command without going through the Raft log reader.
func (f *schedulingFSM) applyCommandPayload(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.AppliedCommands++
	if len(data) == 0 {
		return
	}
	f.applyPayloadLocked(data)
}

func (f *schedulingFSM) applyPayloadLocked(data []byte) {
	commandType, ok := decodeCommandType(data)
	if !ok {
		return
	}
	switch commandType {
	case cmdUpsertNodeCapacity:
		f.applyNodeCapacity(data)
	case cmdUpsertDeployApp:
		f.applyDeployApp(data)
	case cmdDeleteDeployApp:
		f.applyDeleteDeployApp(data)
	case cmdUpsertWorkloadAssignment:
		f.applyWorkloadAssignment(data)
	case cmdUpsertVolumeBinding:
		f.applyVolumeBinding(data)
	}
}

func decodeCommandType(data []byte) (string, bool) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return "", false
	}
	return head.Type, true
}

func (f *schedulingFSM) applyNodeCapacity(data []byte) {
	var env struct {
		Type string                `json:"type"`
		Node nodecapacity.Snapshot `json:"node"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	id := strings.TrimSpace(env.Node.NodeID)
	if id == "" {
		return
	}
	if f.state.NodeCapacity == nil {
		f.state.NodeCapacity = make(map[string]nodecapacity.Snapshot)
	}
	f.state.NodeCapacity[id] = env.Node
}

func (f *schedulingFSM) applyDeployApp(data []byte) {
	var env struct {
		Type string       `json:"type"`
		App  deployv1.App `json:"app"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	if strings.TrimSpace(env.App.Metadata.Name) == "" {
		return
	}
	if f.state.DeployApps == nil {
		f.state.DeployApps = make(map[string]deployv1.App)
	}
	key := deployAppMapKey(env.App.Metadata)
	if current, ok := f.state.DeployApps[key]; ok && deployv1.AppGeneration(current) != deployv1.AppGeneration(env.App) {
		f.appendDeployAppRevisionLocked(key, current)
	}
	f.state.DeployApps[key] = env.App
	f.notifyDeployChanged()
}

func (f *schedulingFSM) applyDeleteDeployApp(data []byte) {
	var env struct {
		Type     string            `json:"type"`
		Metadata deployv1.Metadata `json:"metadata"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	if strings.TrimSpace(env.Metadata.Name) == "" {
		return
	}
	key := deployAppMapKey(env.Metadata)
	if f.state.DeployApps != nil {
		delete(f.state.DeployApps, key)
	}
	if f.state.DeployAppRevisions != nil {
		delete(f.state.DeployAppRevisions, key)
	}
	f.notifyDeployChanged()
}

func (f *schedulingFSM) notifyDeployChanged() {
	if f.notifyDeploy != nil {
		f.notifyDeploy()
	}
}

func (f *schedulingFSM) applyWorkloadAssignment(data []byte) {
	var env struct {
		Type       string                  `json:"type"`
		Assignment workloadmeta.Assignment `json:"assignment"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	assignment, ok := normalizeAssignment(env.Assignment)
	if !ok {
		return
	}
	if f.state.Assignments == nil {
		f.state.Assignments = make(map[string]workloadmeta.Assignment)
	}
	f.state.Assignments[assignment.Key] = assignment
}

func normalizeAssignment(assignment workloadmeta.Assignment) (workloadmeta.Assignment, bool) {
	assignment.Key = strings.TrimSpace(assignment.Key)
	assignment.Metadata.Name = strings.TrimSpace(assignment.Metadata.Name)
	assignment.Metadata.Namespace = strings.TrimSpace(assignment.Metadata.Namespace)
	assignment.Workload = strings.TrimSpace(assignment.Workload)
	assignment.Node = strings.TrimSpace(assignment.Node)
	assignment.Address = strings.TrimSpace(assignment.Address)
	assignment.Status = strings.TrimSpace(assignment.Status)
	if assignment.Metadata.Name == "" || assignment.Workload == "" {
		return workloadmeta.Assignment{}, false
	}
	if assignment.Key == "" {
		assignment.Key = workloadmeta.AssignmentKey(assignment.Metadata, assignment.Workload)
	}
	return assignment, assignment.Key != ""
}

func (f *schedulingFSM) applyVolumeBinding(data []byte) {
	var env struct {
		Type    string             `json:"type"`
		Binding volumemeta.Binding `json:"binding"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	binding, ok := normalizeVolumeBinding(env.Binding)
	if !ok {
		return
	}
	if f.state.VolumeBindings == nil {
		f.state.VolumeBindings = make(map[string]volumemeta.Binding)
	}
	f.state.VolumeBindings[binding.Key] = binding
}

func normalizeVolumeBinding(binding volumemeta.Binding) (volumemeta.Binding, bool) {
	binding.Key = strings.TrimSpace(binding.Key)
	binding.Metadata.Name = strings.TrimSpace(binding.Metadata.Name)
	binding.Metadata.Namespace = strings.TrimSpace(binding.Metadata.Namespace)
	binding.Volume = strings.TrimSpace(binding.Volume)
	binding.Workload = strings.TrimSpace(binding.Workload)
	binding.Target = strings.TrimSpace(binding.Target)
	binding.Node = strings.TrimSpace(binding.Node)
	binding.Source = strings.TrimSpace(binding.Source)
	binding.Status = strings.TrimSpace(binding.Status)
	if binding.Metadata.Name == "" || binding.Workload == "" || binding.Volume == "" {
		return volumemeta.Binding{}, false
	}
	if binding.Key == "" {
		binding.Key = volumemeta.BindingKey(binding.Metadata, binding.Workload, binding.Volume)
	}
	return binding, binding.Key != ""
}
