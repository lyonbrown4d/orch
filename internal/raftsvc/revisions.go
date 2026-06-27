package raftsvc

import (
	"strings"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

const maxDeployAppRevisions = 10

type DeployAppRevision struct {
	Generation string       `json:"generation"`
	App        deployv1.App `json:"app"`
}

func (s *Service) ListDeployAppRevisions(meta deployv1.Metadata) *list.List[DeployAppRevision] {
	if s == nil || s.fsm == nil {
		return list.NewList[DeployAppRevision]()
	}
	return s.fsm.listDeployAppRevisions(meta)
}

func (s *Service) GetPreviousDeployApp(meta deployv1.Metadata) (DeployAppRevision, bool) {
	if s == nil || s.fsm == nil {
		return DeployAppRevision{}, false
	}
	return s.fsm.previousDeployApp(meta)
}

func (f *schedulingFSM) appendDeployAppRevisionLocked(key string, app deployv1.App) {
	generation := deployv1.AppGeneration(app)
	if strings.TrimSpace(generation) == "" {
		return
	}
	if f.state.DeployAppRevisions == nil {
		f.state.DeployAppRevisions = map[string][]DeployAppRevision{}
	}
	revisions := f.state.DeployAppRevisions[key]
	if len(revisions) > 0 && revisions[len(revisions)-1].Generation == generation {
		return
	}
	revisions = append(revisions, DeployAppRevision{Generation: generation, App: app})
	if len(revisions) > maxDeployAppRevisions {
		revisions = revisions[len(revisions)-maxDeployAppRevisions:]
	}
	f.state.DeployAppRevisions[key] = revisions
}
