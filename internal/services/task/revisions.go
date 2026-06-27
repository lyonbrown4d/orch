package task

import (
	"strings"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/raftsvc"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

type AppRevisionView struct {
	Metadata   deployv1.Metadata
	Generation string
	Workloads  int
	App        deployv1.App
}

func (s *Service) ListAppRevisions(meta deployv1.Metadata) (*list.List[AppRevisionView], bool) {
	meta = normalizeOperationMetadata(meta)
	if s == nil || s.raft == nil || strings.TrimSpace(meta.Name) == "" {
		return list.NewList[AppRevisionView](), false
	}
	if _, ok := s.raft.GetDesiredDeployApp(meta); !ok {
		return list.NewList[AppRevisionView](), false
	}
	return list.MapList(s.raft.ListDeployAppRevisions(meta), func(_ int, revision raftsvc.DeployAppRevision) AppRevisionView {
		app := revision.App
		app.Metadata.Namespace = workloadmeta.NamespaceOrDefault(app.Metadata.Namespace)
		return AppRevisionView{
			Metadata:   app.Metadata,
			Generation: revision.Generation,
			Workloads:  len(app.Workloads),
			App:        app,
		}
	}), true
}
