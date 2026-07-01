package task

import (
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/runtime/runconfig"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

const (
	AppStatusPending = "pending"
	AppStatusRunning = "running"
	AppStatusStopped = "stopped"
	AppStatusPartial = "partial"
	AppStatusFailed  = "failed"
)

type AppView struct {
	Metadata           deployv1.Metadata
	Status             string
	DesiredGeneration  string
	ObservedGeneration string
	DesiredWorkloads   int
	Running            int
	Stopped            int
	Failed             int
	Pending            int
	LastTransitionAt   time.Time
	LastError          string
	RevisionCount      int
	RollbackAvailable  bool
	PreviousGeneration string
	Workloads          *list.List[AppWorkloadView]
}

type AppWorkloadView struct {
	Name       string
	Kind       deployv1.WorkloadKind
	Runtime    deployv1.RuntimeKind
	Node       string
	Artifact   string
	Status     string
	Generation string
	Error      string
	UpdatedAt  time.Time
}

func (s *Service) DesiredWorkload(meta deployv1.Metadata, workloadName string) (deployv1.Workload, bool) {
	if s == nil || s.raft == nil {
		return deployv1.Workload{}, false
	}
	app, ok := s.raft.GetDesiredDeployApp(meta)
	if !ok {
		return deployv1.Workload{}, false
	}
	name := strings.TrimSpace(workloadName)
	return appWorkloadsForView(app).FirstWhere(func(_ int, workload deployv1.Workload) bool {
		return strings.TrimSpace(workload.Name) == name
	}).Get()
}

func (s *Service) ListApps() *list.List[AppView] {
	if s == nil || s.raft == nil {
		return list.NewList[AppView]()
	}
	apps := s.raft.ListDesiredDeployApps()
	out := list.MapList(apps, func(_ int, app deployv1.App) AppView {
		return s.buildAppView(app)
	})
	out.Sort(func(a, b AppView) int {
		if cmp := strings.Compare(workloadmeta.NamespaceOrDefault(a.Metadata.Namespace), workloadmeta.NamespaceOrDefault(b.Metadata.Namespace)); cmp != 0 {
			return cmp
		}
		return strings.Compare(strings.TrimSpace(a.Metadata.Name), strings.TrimSpace(b.Metadata.Name))
	})
	return out
}

func (s *Service) GetApp(meta deployv1.Metadata) (AppView, bool) {
	if s == nil || s.raft == nil {
		return AppView{}, false
	}
	app, ok := s.raft.GetDesiredDeployApp(meta)
	if !ok {
		return AppView{}, false
	}
	return s.buildAppView(app), true
}

func (s *Service) buildAppView(app deployv1.App) AppView {
	workloads := appWorkloadsForView(app)
	generation := AppGeneration(app)
	view := AppView{
		Metadata:          app.Metadata,
		DesiredGeneration: generation,
		DesiredWorkloads:  workloads.Len(),
		Workloads:         list.NewListWithCapacity[AppWorkloadView](workloads.Len()),
	}
	workloads.Range(func(_ int, workload deployv1.Workload) bool {
		item := s.buildAppWorkloadView(app.Metadata, workload, generation)
		view.addWorkload(item)
		return true
	})
	view.Status = aggregateAppStatus(view)
	if view.allDesiredWorkloadsObserved() {
		view.ObservedGeneration = generation
	}
	s.addAppRevisionSummary(&view)
	return view
}

func (s *Service) addAppRevisionSummary(view *AppView) {
	if s == nil || s.raft == nil || view == nil {
		return
	}
	revisions := s.raft.ListDeployAppRevisions(view.Metadata)
	view.RevisionCount = revisions.Len()
	previous, ok := s.raft.GetPreviousDeployApp(view.Metadata)
	if !ok {
		return
	}
	view.RollbackAvailable = true
	view.PreviousGeneration = strings.TrimSpace(previous.Generation)
}

func (view *AppView) addWorkload(item AppWorkloadView) {
	view.Workloads.Add(item)
	view.countWorkloadStatus(item.Status)
	if item.UpdatedAt.After(view.LastTransitionAt) {
		view.LastTransitionAt = item.UpdatedAt
	}
	if item.Error != "" && view.LastError == "" {
		view.LastError = item.Error
	}
}

func (view *AppView) countWorkloadStatus(status string) {
	switch status {
	case workloadmeta.AssignmentStatusRunning:
		view.Running++
	case workloadmeta.AssignmentStatusStopped:
		view.Stopped++
	case workloadmeta.AssignmentStatusFailed:
		view.Failed++
	default:
		view.Pending++
	}
}

func (view AppView) allDesiredWorkloadsObserved() bool {
	if view.DesiredWorkloads == 0 || view.Pending != 0 {
		return false
	}
	return view.Running+view.Stopped+view.Failed == view.DesiredWorkloads
}

func (s *Service) buildAppWorkloadView(meta deployv1.Metadata, workload deployv1.Workload, desiredGeneration string) AppWorkloadView {
	item := AppWorkloadView{
		Name:     workload.Name,
		Kind:     workload.Kind,
		Runtime:  workload.Runtime,
		Artifact: runconfig.ArtifactSummary(workload.Run),
		Status:   AppStatusPending,
	}
	if s == nil || s.raft == nil {
		return item
	}
	assignment, ok := s.raft.GetWorkloadAssignment(workloadmeta.AssignmentKey(meta, workload.Name))
	if !ok {
		return item
	}
	observedGeneration := strings.TrimSpace(assignment.Generation)
	item.Node = assignment.Node
	if assignment.Artifact != "" {
		item.Artifact = assignment.Artifact
	}
	item.Generation = observedGeneration
	if observedGeneration != desiredGeneration {
		item.Status = AppStatusPending
		item.UpdatedAt = assignment.UpdatedAt
		return item
	}
	item.Status = strings.TrimSpace(assignment.Status)
	if item.Status == "" {
		item.Status = AppStatusPending
	}
	item.Error = assignment.Error
	item.UpdatedAt = assignment.UpdatedAt
	return item
}

func AppGeneration(app deployv1.App) string {
	return deployv1.AppGeneration(app)
}

func appWorkloadsForView(app deployv1.App) *list.List[deployv1.Workload] {
	workloads, err := app.WorkloadsInDependencyOrder()
	if err != nil {
		return list.NewList(app.Workloads...)
	}
	return workloads
}

func aggregateAppStatus(view AppView) string {
	total := view.DesiredWorkloads
	if total == 0 {
		return AppStatusPending
	}
	switch {
	case view.Failed > 0:
		return AppStatusFailed
	case view.Running == total:
		return AppStatusRunning
	case view.Stopped == total:
		return AppStatusStopped
	case view.Running > 0 || view.Stopped > 0:
		return AppStatusPartial
	default:
		return AppStatusPending
	}
}
