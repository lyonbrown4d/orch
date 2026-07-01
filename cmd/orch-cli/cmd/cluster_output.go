package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/spf13/cobra"

	"github.com/lyonbrown4d/orch/internal/api"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

func writeDeployOperationHuman(label string, out *api.DeployOperationOutput) error {
	body := out.Body
	fields := []string{
		viewField("status", statusBadge(body.Status)),
		viewField("app", body.App),
		viewField("namespace", body.Namespace),
		viewField("moved", strconv.Itoa(body.Moved)+"/"+strconv.Itoa(body.Workloads)),
	}
	if body.TargetNode != "" {
		fields = append(fields, viewField("target", body.TargetNode))
	}
	return writeInfoLine(label, fields...)
}

func writeHostinfoHuman(out *api.HostinfoOutput) error {
	body := out.Body
	h := body.Host
	cpu := body.CPU
	mem := body.Memory
	rows := list.NewGrid[string](
		[]string{"hostname", h.Hostname},
		[]string{"os", h.OS},
		[]string{"platform", h.Platform},
		[]string{"kernel", h.KernelVersion},
		[]string{"arch", h.KernelArch},
		[]string{"cpu_cores", strconv.Itoa(cpu.LogicalCores)},
		[]string{"cpu_model", cpu.ModelName},
		[]string{"cpu_usage", formatPercent(cpu.UsagePercent)},
		[]string{"memory_total", formatBytes(mem.TotalBytes)},
		[]string{"memory_used", formatPercent(mem.UsedPercent)},
	)
	if body.Load != nil {
		l := body.Load
		rows.AddRow("load_1", strconv.FormatFloat(l.Load1, 'f', 2, 64))
		rows.AddRow("load_5", strconv.FormatFloat(l.Load5, 'f', 2, 64))
		rows.AddRow("load_15", strconv.FormatFloat(l.Load15, 'f', 2, 64))
	}
	return writeKVTable(rows)
}

func writeWorkloadsHuman(items *list.List[api.WorkloadItem]) error {
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, w api.WorkloadItem) bool {
		node := w.Node
		if node == "" {
			node = "-"
		}
		rows.AddRow(w.Name, node, w.Runtime, statusBadge(w.Status), w.Artifact)
		return true
	})
	return writeTable(list.NewList("NAME", "NODE", "RUNTIME", "STATUS", "ARTIFACT"), rows)
}

func writeAssignmentsHuman(items *list.List[api.AssignmentItem]) error {
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, a api.AssignmentItem) bool {
		node := nonEmpty(a.Node)
		artifact := nonEmpty(a.Artifact)
		errMsg := nonEmpty(a.Error)
		rows.AddRow(a.Key, node, string(a.Runtime), statusBadge(a.Status), artifact, errMsg)
		return true
	})
	return writeTable(list.NewList("KEY", "NODE", "RUNTIME", "STATUS", "ARTIFACT", "ERROR"), rows)
}

func writeVolumesHuman(items *list.List[api.VolumeBindingItem]) error {
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, v api.VolumeBindingItem) bool {
		mount := nonEmpty(v.Workload)
		if v.Target != "" {
			mount += ":" + v.Target
		}
		rows.AddRow(v.Key, nonEmpty(v.Node), string(v.Runtime), statusBadge(v.Status), nonEmpty(v.Source), mount, nonEmpty(v.Error))
		return true
	})
	return writeTable(list.NewList("KEY", "NODE", "RUNTIME", "STATUS", "SOURCE", "MOUNT", "ERROR"), rows)
}

func writeRuntimesHuman(items *list.List[api.RuntimeProviderItem]) error {
	if items == nil {
		return writeLine(viewMutedStyle.Render("No runtime providers reported."))
	}
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, item api.RuntimeProviderItem) bool {
		rows.AddRow(string(item.Kind), string(item.Policy), strconv.FormatBool(item.Available), strconv.FormatBool(item.Registered), statusBadge(item.Status), nonEmpty(item.Reason))
		return true
	})
	return writeTable(list.NewList("RUNTIME", "POLICY", "AVAILABLE", "REGISTERED", "STATUS", "REASON"), rows)
}

func writeAppsHuman(items *list.List[api.AppItem]) error {
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, app api.AppItem) bool {
		rows.AddRow(app.Namespace, app.Name, statusBadge(app.Status), appReadyText(app.Running, app.DesiredWorkloads), nonEmpty(app.DesiredGeneration), nonEmpty(app.ObservedGeneration), strconv.Itoa(app.RevisionCount), boolText(app.RollbackAvailable), nonEmpty(app.PreviousGeneration), appCountsText(app), formatTime(app.LastTransitionAt), nonEmpty(app.LastError))
		return true
	})
	return writeTable(list.NewList("NAMESPACE", "NAME", "STATUS", "READY", "GENERATION", "OBSERVED", "REVISIONS", "ROLLBACK", "PREVIOUS", "COUNTS", "UPDATED", "ERROR"), rows)
}

func writeAppRevisionsHuman(items *list.List[api.AppRevisionItem]) error {
	if items == nil || items.Len() == 0 {
		return writeLine(viewMutedStyle.Render("No rollback revisions recorded."))
	}
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, item api.AppRevisionItem) bool {
		rows.AddRow(workloadmetaNamespace(item.Metadata.Namespace), item.Metadata.Name, nonEmpty(item.Generation), strconv.Itoa(item.Workloads), revisionArtifacts(item.App))
		return true
	})
	return writeTable(list.NewList("NAMESPACE", "NAME", "GENERATION", "WORKLOADS", "ARTIFACTS"), rows)
}

func revisionArtifacts(app deployv1.App) string {
	parts := list.NewList[string]()
	for i := range app.Workloads {
		workload := app.Workloads[i]
		artifact := workload.Run.Artifact.Image
		if artifact == "" {
			artifact = workload.Run.Artifact.Path
		}
		if artifact == "" {
			artifact = "-"
		}
		parts.Add(workload.Name + "=" + artifact)
	}
	return strings.Join(parts.Values(), ",")
}

func workloadmetaNamespace(namespace string) string {
	if namespace == "" {
		return "default"
	}
	return namespace
}
func writeAppDetailHuman(app *api.AppDetailItem) error {
	if app == nil {
		return writeLine(viewMutedStyle.Render("No resources found."))
	}
	rows := list.NewGrid[string](
		[]string{"namespace", app.Namespace},
		[]string{"name", app.Name},
		[]string{"status", statusBadge(app.Status)},
		[]string{"generation", nonEmpty(app.DesiredGeneration)},
		[]string{"observed_generation", nonEmpty(app.ObservedGeneration)},
		[]string{"ready", appReadyText(app.Running, app.DesiredWorkloads)},
		[]string{"workloads", strconv.Itoa(app.DesiredWorkloads)},
		[]string{"running", strconv.Itoa(app.Running)},
		[]string{"stopped", strconv.Itoa(app.Stopped)},
		[]string{"failed", strconv.Itoa(app.Failed)},
		[]string{"pending", strconv.Itoa(app.Pending)},
		[]string{"last_transition", formatTime(app.LastTransitionAt)},
		[]string{"last_error", nonEmpty(app.LastError)},
		[]string{"revision_count", strconv.Itoa(app.RevisionCount)},
		[]string{"rollback_available", strconv.FormatBool(app.RollbackAvailable)},
		[]string{"previous_generation", nonEmpty(app.PreviousGeneration)},
	)
	if err := writeKVTable(rows); err != nil {
		return err
	}
	if err := writeLine(""); err != nil {
		return err
	}
	workloadRows := list.NewGridWithCapacity[string](app.Workloads.Len())
	app.Workloads.Range(func(_ int, workload api.AppWorkloadItem) bool {
		workloadRows.AddRow(workload.Name, string(workload.Kind), string(workload.Runtime), nonEmpty(workload.Node), statusBadge(workload.Status), nonEmpty(workload.Generation), nonEmpty(workload.Artifact), nonEmpty(workload.Error))
		return true
	})
	return writeTable(list.NewList("WORKLOAD", "KIND", "RUNTIME", "NODE", "STATUS", "GENERATION", "ARTIFACT", "ERROR"), workloadRows)
}

func appReadyText(running, total int) string {
	return strconv.Itoa(running) + "/" + strconv.Itoa(total)
}

func appCountsText(app api.AppItem) string {
	return fmt.Sprintf("run=%d stop=%d fail=%d pending=%d", app.Running, app.Stopped, app.Failed, app.Pending)
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func writeRaftStatusHuman(out *api.RaftStatusOutput) error {
	body := out.Body
	role := "follower"
	if body.IsLeader {
		role = "leader"
	}
	if !body.Ready {
		role = "-"
	}
	memberCount := 0
	if body.Members != nil {
		memberCount = body.Members.Len()
	}
	rows := list.NewGrid[string](
		[]string{"ready", strconv.FormatBool(body.Ready)},
		[]string{"state", statusBadge(body.State)},
		[]string{"role", role},
		[]string{"node_id", nonEmpty(body.NodeID)},
		[]string{"leader_id", nonEmpty(body.LeaderID)},
		[]string{"leader_address", nonEmpty(body.LeaderAddress)},
		[]string{"leader_api", nonEmpty(body.LeaderAPIURL)},
		[]string{"local_address", nonEmpty(body.LocalAddress)},
		[]string{"members", strconv.Itoa(memberCount)},
	)
	return writeKVTable(rows)
}

func writeRaftMembersHuman(items *list.List[api.RaftMemberItem]) error {
	rows := list.NewGridWithCapacity[string](items.Len())
	items.Range(func(_ int, member api.RaftMemberItem) bool {
		rows.AddRow(member.ID, member.Address, member.Suffrage)
		return true
	})
	return writeTable(list.NewList("ID", "ADDRESS", "SUFFRAGE"), rows)
}

func nonEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func contextFromCmd(cmd *cobra.Command) context.Context {
	if cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}
