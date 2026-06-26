package task_test

import (
	"context"
	"sync"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	orchruntime "github.com/lyonbrown4d/orch/internal/runtime"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
)

type fakeRuntimeProvider struct {
	mu             sync.Mutex
	deployed       []deployv1.Workload
	stopped        []string
	events         []string
	statuses       map[string]string
	deployErr      error
	artifactErrors map[string]error
	ch             chan deployv1.Workload
	stopCh         chan string
}

func newFakeRuntimeProvider() *fakeRuntimeProvider {
	return &fakeRuntimeProvider{
		statuses:       map[string]string{},
		artifactErrors: map[string]error{},
		ch:             make(chan deployv1.Workload, 4),
		stopCh:         make(chan string, 4),
	}
}

func (p *fakeRuntimeProvider) Kind() deployv1.RuntimeKind {
	return deployv1.RuntimeDocker
}

func (p *fakeRuntimeProvider) Deploy(_ context.Context, _ deployv1.Metadata, workload deployv1.Workload) error {
	p.mu.Lock()
	if p.deployErr != nil {
		err := p.deployErr
		p.mu.Unlock()
		return err
	}
	artifact := workload.Run.Artifact.Image
	p.events = append(p.events, "deploy:"+workload.Name+":"+artifact)
	if err := p.artifactErrors[artifact]; err != nil {
		p.mu.Unlock()
		return err
	}
	p.deployed = append(p.deployed, workload)
	p.statuses[workload.Name] = workloadmeta.AssignmentStatusRunning
	p.mu.Unlock()
	p.ch <- workload
	return nil
}

func (p *fakeRuntimeProvider) Stop(_ context.Context, _ deployv1.Metadata, name string) error {
	p.mu.Lock()
	p.stopped = append(p.stopped, name)
	p.events = append(p.events, "stop:"+name)
	p.statuses[name] = workloadmeta.AssignmentStatusStopped
	p.mu.Unlock()
	select {
	case p.stopCh <- name:
	default:
	}
	return nil
}

func (p *fakeRuntimeProvider) Status(_ context.Context, _ deployv1.Metadata, name string) (orchruntime.Status, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := p.statuses[name]
	if status == "" {
		status = workloadmeta.AssignmentStatusRunning
	}
	return orchruntime.Status{Name: name, Runtime: deployv1.RuntimeDocker, Status: status}, nil
}

func (p *fakeRuntimeProvider) setStatus(name, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statuses[name] = status
}

func (p *fakeRuntimeProvider) failDeploy(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deployErr = err
}

func (p *fakeRuntimeProvider) failDeployArtifact(artifact string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.artifactErrors[artifact] = err
}

func (p *fakeRuntimeProvider) runtimeEvents() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}
