package cmd

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/lyonbrown4d/orch/cmd/orch-cli/cliapp"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func newStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stack",
		Aliases: []string{"stacks"},
		Short:   "Manage deployed application stacks",
		Long:    `Manage deployed application stacks from manifest submission through status and lifecycle operations.`,
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(newStackApplyCmd("apply", "Deploy or update a stack from a manifest"))
	cmd.AddCommand(newStackApplyCmd("update", "Update a stack from a manifest"))
	cmd.AddCommand(newStackRollbackCmd())
	cmd.AddCommand(newStackHistoryCmd())
	cmd.AddCommand(newStackListCmd())
	cmd.AddCommand(newStackStatusCmd())
	cmd.AddCommand(newStackOperationCmd("delete NAME", "Stop and delete a stack", "delete", deleteAppStatus))
	cmd.AddCommand(newStackOperationCmd("start NAME", "Start a stopped stack", "start", startAppStatus))
	cmd.AddCommand(newStackOperationCmd("stop NAME", "Stop a stack", "stop", stopAppStatus))
	cmd.AddCommand(newStackOperationCmd("restart NAME", "Restart a stack", "restart", restartAppStatus))
	return cmd
}

func newStackApplyCmd(use, short string) *cobra.Command {
	var file string
	var jsonOut bool
	var watch bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   use + " -f FILE",
		Short: short,
		Long:  `Reads the stack manifest locally and submits it to the orch control plane. Existing desired state with the same namespace/name is updated in place.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return oopsx.B("cli").Errorf("--file is required")
			}
			src, err := readManifestFile(file)
			if err != nil {
				return err
			}
			return runApplyCommand(contextFromCmd(cmd), applyOptions{
				File:    file,
				Source:  src,
				JSON:    jsonOut,
				Watch:   watch,
				Timeout: timeout,
			})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to stack manifest (.orch or YAML)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON response")
	cmd.Flags().BoolVar(&watch, "watch", false, "Wait until deployed workloads are running")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Maximum time to wait with --watch")
	return cmd
}

func newStackRollbackCmd() *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "rollback NAME",
		Short: "Roll back a stack to the previous desired revision",
		Long:  `Restores the previous desired app revision stored in Raft and lets reconcile apply it. To roll back to an explicit file, use stack apply -f FILE.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStackOperationCommand(contextFromCmd(cmd), namespace, args[0], "rollback", jsonOut, rollbackAppStatus)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Stack namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newStackHistoryCmd() *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "history NAME",
		Aliases: []string{"revisions"},
		Short:   "List stack rollback revisions",
		Long:    `Lists previous desired app revisions stored in Raft. The newest previous revision is shown first.`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStackHistoryCommand(contextFromCmd(cmd), namespace, args[0], jsonOut)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Stack namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func runStackHistoryCommand(ctx context.Context, namespace, name string, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListDeployRevisions(ctx, namespace, name)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list stack revisions")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeAppRevisionsHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "stack history command")
	}
	return nil
}
func newStackListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployed stacks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListApps(contextFromCmd(cmd), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newStackStatusCmd() *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status NAME",
		Short: "Show stack status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppDetailCommand(contextFromCmd(cmd), namespace, args[0], "get stack status", jsonOut)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Stack namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newStackOperationCmd(use, short, label string, action appStatusAction) *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStackOperationCommand(contextFromCmd(cmd), namespace, args[0], label, jsonOut, action)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Stack namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func runStackOperationCommand(ctx context.Context, namespace, name, label string, jsonOut bool, action appStatusAction) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		body, raw, err := action(ctx, c, namespace, name)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "%s stack", label)
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(raw)
		}
		return writeInfoLine(label,
			viewField("status", statusBadge(body.Status)),
			viewField("stack", body.App),
			viewField("namespace", body.Namespace),
		)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "%s stack command", label)
	}
	return nil
}
