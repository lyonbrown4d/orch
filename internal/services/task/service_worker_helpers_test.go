package task_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lyonbrown4d/orch/internal/runtime/runtimeinfo"
	"github.com/lyonbrown4d/orch/internal/workerapi"
)

func newDeployWorkerServer(t *testing.T, dispatchCh chan<- workerapi.DeployWorkloadBody, status string) *httptest.Server {
	t.Helper()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireWorkerPath(t, r, workerapi.PathV1WorkerDeploy)
		in := decodeDeployRequest(t, r)
		dispatchCh <- in
		out := workerapi.DeployWorkloadOutput{}
		out.Body.Accepted = true
		out.Body.Node = in.Node
		out.Body.Status = status
		out.Body.Workload = in.Workload.Name
		writeWorkerResponse(t, w, out.Body)
	}))
	t.Cleanup(worker.Close)
	return worker
}

func newFailingDeployWorkerServer(t *testing.T, dispatchCh chan<- struct{}) *httptest.Server {
	t.Helper()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireWorkerPath(t, r, workerapi.PathV1WorkerDeploy)
		select {
		case dispatchCh <- struct{}{}:
		default:
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(worker.Close)
	return worker
}

func newInspectWorkerServer(
	t *testing.T,
	statusCh chan<- workerapi.WorkloadStatusBody,
	logsCh chan<- workerapi.WorkloadLogsBody,
) *httptest.Server {
	t.Helper()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case workerapi.PathV1WorkerStatus:
			in := decodeStatusRequest(t, r)
			statusCh <- in
			out := workerapi.WorkloadStatusOutput{}
			out.Body = runtimeinfo.Status{
				Name:     in.Workload.Name,
				Runtime:  in.Workload.Runtime,
				Status:   "running",
				NativeID: "remote-native-id",
			}
			writeWorkerResponse(t, w, out.Body)
		case workerapi.PathV1WorkerLogs:
			in := decodeLogsRequest(t, r)
			logsCh <- in
			out := workerapi.WorkloadLogsOutput{}
			out.Body = runtimeinfo.LogResult{
				Name:    in.Workload.Name,
				Runtime: in.Workload.Runtime,
				Source:  "remote-log",
				Content: "remote log line\n",
			}
			writeWorkerResponse(t, w, out.Body)
		default:
			t.Fatalf("worker path = %q", r.URL.Path)
		}
	}))
	t.Cleanup(worker.Close)
	return worker
}

func requireWorkerPath(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.URL.Path != want {
		t.Fatalf("worker path = %q, want %q", r.URL.Path, want)
	}
}

func decodeDeployRequest(t *testing.T, r *http.Request) workerapi.DeployWorkloadBody {
	t.Helper()
	var in workerapi.DeployWorkloadBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		t.Fatalf("decode worker request: %v", err)
	}
	return in
}

func decodeStatusRequest(t *testing.T, r *http.Request) workerapi.WorkloadStatusBody {
	t.Helper()
	var in workerapi.WorkloadStatusBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		t.Fatalf("decode worker status request: %v", err)
	}
	return in
}

func decodeLogsRequest(t *testing.T, r *http.Request) workerapi.WorkloadLogsBody {
	t.Helper()
	var in workerapi.WorkloadLogsBody
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		t.Fatalf("decode worker logs request: %v", err)
	}
	return in
}

func writeWorkerResponse(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode worker response: %v", err)
	}
}

func waitWorkerDispatch(t *testing.T, dispatchCh <-chan workerapi.DeployWorkloadBody) workerapi.DeployWorkloadBody {
	t.Helper()
	select {
	case got := <-dispatchCh:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for worker dispatch")
		return workerapi.DeployWorkloadBody{}
	}
}

func waitWorkerStatus(t *testing.T, statusCh <-chan workerapi.WorkloadStatusBody, timeout time.Duration) workerapi.WorkloadStatusBody {
	t.Helper()
	select {
	case got := <-statusCh:
		return got
	case <-time.After(timeout):
		t.Fatal("timed out waiting for worker status")
		return workerapi.WorkloadStatusBody{}
	}
}

func waitWorkerLogs(t *testing.T, logsCh <-chan workerapi.WorkloadLogsBody, timeout time.Duration) workerapi.WorkloadLogsBody {
	t.Helper()
	select {
	case got := <-logsCh:
		return got
	case <-time.After(timeout):
		t.Fatal("timed out waiting for worker logs")
		return workerapi.WorkloadLogsBody{}
	}
}

func requireWorkerDispatch(t *testing.T, got workerapi.DeployWorkloadBody, nodeID string) {
	t.Helper()
	if got.Node != nodeID || got.Workload.Name != "worker" {
		t.Fatalf("dispatch request = %#v", got)
	}
}
