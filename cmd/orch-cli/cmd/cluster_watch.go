package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/cenkalti/backoff/v5"
	"github.com/pterm/pterm"

	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/internal/workloadmeta"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

type deploySnapshot struct {
	Assignments        *list.List[api.AssignmentItem]
	Workloads          *list.List[api.WorkloadItem]
	Total              int
	RunningAssignments int
	RunningWorkloads   int
	FailedAssignment   *api.AssignmentItem
}

func newDeploySnapshot(total int) *deploySnapshot {
	return &deploySnapshot{
		Assignments: list.NewList[api.AssignmentItem](),
		Workloads:   list.NewList[api.WorkloadItem](),
		Total:       total,
	}
}

func watchDeployment(ctx context.Context, c *apiclient.Client, app *deployv1.App, timeout time.Duration, progress bool) (*deploySnapshot, error) {
	if timeout <= 0 {
		return nil, oopsx.B("cli").Errorf("--timeout must be greater than zero")
	}
	expectedKeys, expectedNames := expectedDeployWorkloads(app)
	expectedGeneration := ""
	if app != nil {
		expectedGeneration = deployv1.AppGeneration(*app)
	}
	if expectedKeys.Len() == 0 {
		return newDeploySnapshot(0), nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spinner := startWatchSpinner(progress, expectedKeys.Len())
	watcher := newDeployWatcher(c, expectedKeys, expectedNames, expectedGeneration, spinner)

	snapshot, err := backoff.Retry(waitCtx, watcher.poll(waitCtx), backoff.WithBackOff(backoff.NewConstantBackOff(500*time.Millisecond)))
	return watcher.result(snapshot, err, timeout)
}

type deployWatcher struct {
	client             *apiclient.Client
	expectedKeys       *set.Set[string]
	expectedNames      *set.Set[string]
	expectedGeneration string
	spinner            *pterm.SpinnerPrinter
	last               *deploySnapshot
	lastErr            error
	permanentErr       error
}

func newDeployWatcher(
	client *apiclient.Client,
	expectedKeys *set.Set[string],
	expectedNames *set.Set[string],
	expectedGeneration string,
	spinner *pterm.SpinnerPrinter,
) *deployWatcher {
	return &deployWatcher{
		client:             client,
		expectedKeys:       expectedKeys,
		expectedNames:      expectedNames,
		expectedGeneration: expectedGeneration,
		spinner:            spinner,
	}
}

func (w *deployWatcher) poll(ctx context.Context) func() (*deploySnapshot, error) {
	return func() (*deploySnapshot, error) {
		snapshot, err := readDeploySnapshot(ctx, w.client, w.expectedKeys, w.expectedNames, w.expectedGeneration)
		if err != nil {
			w.lastErr = err
			updateWatchSpinner(w.spinner, w.last, w.expectedKeys.Len(), err)
			return nil, err
		}
		return w.recordSnapshot(snapshot)
	}
}

func (w *deployWatcher) recordSnapshot(snapshot *deploySnapshot) (*deploySnapshot, error) {
	w.last = snapshot
	updateWatchSpinner(w.spinner, snapshot, w.expectedKeys.Len(), nil)
	if snapshot.FailedAssignment != nil {
		return nil, w.failedAssignment(snapshot.FailedAssignment)
	}
	if deploySnapshotReady(snapshot) {
		successWatchSpinner(w.spinner, fmt.Sprintf("workloads running assignments=%d/%d runtime=%d/%d",
			snapshot.RunningAssignments, snapshot.Total, snapshot.RunningWorkloads, snapshot.Total))
		return snapshot, nil
	}
	return nil, errWaitPending
}

func (w *deployWatcher) failedAssignment(failed *api.AssignmentItem) error {
	failWatchSpinner(w.spinner, "workload failed key="+failed.Key)
	w.permanentErr = oopsx.B("cli").Errorf("workload %s failed on node %s: %s",
		failed.Key,
		nonEmpty(failed.Node),
		nonEmpty(failed.Error),
	)
	return oopsx.B("cli").Wrapf(backoff.Permanent(w.permanentErr), "deploy failed")
}

func (w *deployWatcher) result(snapshot *deploySnapshot, err error, timeout time.Duration) (*deploySnapshot, error) {
	if err == nil {
		return snapshot, nil
	}
	if w.permanentErr != nil {
		return w.last, w.permanentErr
	}
	failWatchSpinner(w.spinner, watchStatusText(w.last, w.expectedKeys.Len(), "timed out"))
	if w.lastErr != nil && !errors.Is(err, errWaitPending) {
		return w.last, oopsx.B("cli").Wrapf(w.lastErr, "wait for deploy status timed out after %s", timeout)
	}
	return w.last, oopsx.B("cli").Errorf("wait for deploy status timed out after %s: %s", timeout, watchStatusText(w.last, w.expectedKeys.Len(), ""))
}

func deploySnapshotReady(snapshot *deploySnapshot) bool {
	return snapshot.RunningAssignments == snapshot.Total && snapshot.RunningWorkloads == snapshot.Total
}

func expectedDeployWorkloads(app *deployv1.App) (*set.Set[string], *set.Set[string]) {
	keys := set.NewSet[string]()
	names := set.NewSet[string]()
	if app == nil {
		return keys, names
	}
	for i := range app.Workloads {
		workload := &app.Workloads[i]
		key := workloadmeta.AssignmentKey(app.Metadata, workload.Name)
		if key == "" {
			continue
		}
		keys.Add(key)
		names.Add(workload.Name)
	}
	return keys, names
}

func readDeploySnapshot(ctx context.Context, c *apiclient.Client, expectedKeys, expectedNames *set.Set[string], expectedGeneration string) (*deploySnapshot, error) {
	assignments, err := c.ListAssignments(ctx)
	if err != nil {
		return nil, oopsx.B("cli").Wrapf(err, "list assignments")
	}
	workloads, err := c.ListWorkloads(ctx)
	if err != nil {
		return nil, oopsx.B("cli").Wrapf(err, "list workloads")
	}

	snapshot := newDeploySnapshot(expectedKeys.Len())
	collectAssignmentSnapshot(snapshot, assignments.Body.Items, expectedKeys, expectedGeneration)
	collectWorkloadSnapshot(snapshot, workloads.Body.Items, expectedNames)
	return snapshot, nil
}

func collectAssignmentSnapshot(snapshot *deploySnapshot, items *list.List[api.AssignmentItem], expectedKeys *set.Set[string], expectedGeneration string) {
	items.Range(func(_ int, assignment api.AssignmentItem) bool {
		if !expectedKeys.Contains(assignment.Key) {
			return true
		}
		snapshot.Assignments.Add(assignment)
		matchesGeneration := assignmentMatchesExpectedGeneration(assignment, expectedGeneration)
		if matchesGeneration && assignment.Status == workloadmeta.AssignmentStatusRunning {
			snapshot.RunningAssignments++
		}
		if matchesGeneration && assignment.Status == workloadmeta.AssignmentStatusFailed && snapshot.FailedAssignment == nil {
			failed := assignment
			snapshot.FailedAssignment = &failed
		}
		return true
	})
}

func assignmentMatchesExpectedGeneration(assignment api.AssignmentItem, expectedGeneration string) bool {
	expectedGeneration = strings.TrimSpace(expectedGeneration)
	return expectedGeneration == "" || strings.TrimSpace(assignment.Generation) == expectedGeneration
}

func collectWorkloadSnapshot(snapshot *deploySnapshot, items *list.List[api.WorkloadItem], expectedNames *set.Set[string]) {
	items.Range(func(_ int, workload api.WorkloadItem) bool {
		if !expectedNames.Contains(workload.Name) {
			return true
		}
		snapshot.Workloads.Add(workload)
		if workload.Status == "running" {
			snapshot.RunningWorkloads++
		}
		return true
	})
}

func startWatchSpinner(progress bool, total int) *pterm.SpinnerPrinter {
	if !progress || !stderrIsTerminal() {
		return nil
	}
	spinner, err := pterm.DefaultSpinner.WithRemoveWhenDone(false).Start(
		fmt.Sprintf("waiting for workloads assignments=0/%d runtime=0/%d", total, total),
	)
	if err != nil {
		return nil
	}
	return spinner
}

func updateWatchSpinner(spinner *pterm.SpinnerPrinter, snapshot *deploySnapshot, total int, err error) {
	if spinner == nil {
		return
	}
	if err != nil {
		spinner.UpdateText(watchStatusText(snapshot, total, fmt.Sprintf("last_error=%v", err)))
		return
	}
	spinner.UpdateText(watchStatusText(snapshot, total, ""))
}

func successWatchSpinner(spinner *pterm.SpinnerPrinter, msg string) {
	if spinner != nil {
		spinner.Success(msg)
	}
}

func failWatchSpinner(spinner *pterm.SpinnerPrinter, msg string) {
	if spinner != nil {
		spinner.Fail(msg)
	}
}

func watchStatusText(snapshot *deploySnapshot, total int, suffix string) string {
	runningAssignments := 0
	runningWorkloads := 0
	if snapshot != nil {
		runningAssignments = snapshot.RunningAssignments
		runningWorkloads = snapshot.RunningWorkloads
		if snapshot.Total > 0 {
			total = snapshot.Total
		}
	}
	text := fmt.Sprintf("waiting for workloads assignments=%d/%d runtime=%d/%d", runningAssignments, total, runningWorkloads, total)
	if suffix != "" {
		return text + " " + suffix
	}
	return text
}

func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
