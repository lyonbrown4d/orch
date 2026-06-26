package raftsvc

import (
	"strings"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/volumemeta"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

func deployAppMapKey(m deployv1.Metadata) string {
	return workloadmeta.NamespaceOrDefault(m.Namespace) + "/" + strings.TrimSpace(m.Name)
}

func (f *schedulingFSM) getDeployApp(meta deployv1.Metadata) (deployv1.App, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.DeployApps == nil {
		return deployv1.App{}, false
	}
	app, ok := f.state.DeployApps[deployAppMapKey(meta)]
	return app, ok
}

func (f *schedulingFSM) listDeployApps() *list.List[deployv1.App] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.state.DeployApps) == 0 {
		return list.NewList[deployv1.App]()
	}
	out := list.NewListWithCapacity[deployv1.App](len(f.state.DeployApps))
	for key := range f.state.DeployApps {
		out.Add(f.state.DeployApps[key])
	}
	return out
}

func (f *schedulingFSM) listAssignments() *list.List[workloadmeta.Assignment] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.state.Assignments) == 0 {
		return list.NewList[workloadmeta.Assignment]()
	}
	out := list.NewListWithCapacity[workloadmeta.Assignment](len(f.state.Assignments))
	for key := range f.state.Assignments {
		out.Add(f.state.Assignments[key])
	}
	return out
}

func (f *schedulingFSM) getAssignment(key string) (workloadmeta.Assignment, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.Assignments == nil {
		return workloadmeta.Assignment{}, false
	}
	a, ok := f.state.Assignments[strings.TrimSpace(key)]
	return a, ok
}

func (f *schedulingFSM) listVolumeBindings() *list.List[volumemeta.Binding] {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.state.VolumeBindings) == 0 {
		return list.NewList[volumemeta.Binding]()
	}
	out := list.NewListWithCapacity[volumemeta.Binding](len(f.state.VolumeBindings))
	for key := range f.state.VolumeBindings {
		out.Add(f.state.VolumeBindings[key])
	}
	return out
}

func (f *schedulingFSM) getVolumeBinding(key string) (volumemeta.Binding, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.VolumeBindings == nil {
		return volumemeta.Binding{}, false
	}
	binding, ok := f.state.VolumeBindings[strings.TrimSpace(key)]
	return binding, ok
}
