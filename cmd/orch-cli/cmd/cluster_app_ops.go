package cmd

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/lyonbrown4d/orch/cmd/orch-cli/cliapp"
	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func newDeleteCmd(use string, aliases []string) *cobra.Command {
	return newAppStatusCmd(appStatusCommandSpec{
		Use:     use + " app NAME",
		Aliases: aliases,
		Short:   "Stop and delete a deployed app",
		Long:    `Stops workloads for the named app, removes the desired app document, and records stopped assignments.`,
		Label:   "delete",
		Action:  deleteAppStatus,
	})
}

func newStartCmd() *cobra.Command {
	return newAppStatusCmd(appStatusCommandSpec{
		Use:    "start app NAME",
		Short:  "Start a stopped app",
		Long:   `Starts workloads for the named app from the desired app document retained by the control plane.`,
		Label:  "start",
		Action: startAppStatus,
	})
}

func newStopCmd() *cobra.Command {
	return newAppStatusCmd(appStatusCommandSpec{
		Use:    "stop app NAME",
		Short:  "Stop a deployed app",
		Long:   `Stops workloads for the named app and records stopped assignments while keeping the desired app document.`,
		Label:  "stop",
		Action: stopAppStatus,
	})
}

func newRestartCmd() *cobra.Command {
	return newAppStatusCmd(appStatusCommandSpec{
		Use:    "restart app NAME",
		Short:  "Restart a deployed app",
		Long:   `Stops then starts workloads for the named app using its desired app document.`,
		Label:  "restart",
		Action: restartAppStatus,
	})
}

func newRollbackCmd() *cobra.Command {
	return newAppStatusCmd(appStatusCommandSpec{
		Use:    "rollback app NAME",
		Short:  "Roll back a deployed app to the previous desired revision",
		Long:   `Restores the previous desired app revision stored in Raft and lets reconcile apply it.`,
		Label:  "rollback",
		Action: rollbackAppStatus,
	})
}

func newMigrateCmd() *cobra.Command {
	var targetNode string
	cmd := newDeployOperationCmd(deployOperationCommandSpec{
		Use:   "migrate app NAME --to NODE",
		Short: "Move app workloads to a target node",
		Long:  `Stops selected workloads, starts them on the target node, and updates scheduler assignments. Desired app state is kept.`,
		Label: "migrate",
		Action: func(ctx context.Context, c *apiclient.Client, namespace, name string, workloads []string) (*api.DeployOperationOutput, error) {
			return c.MigrateDeploy(ctx, namespace, name, targetNode, workloads)
		},
	})
	cmd.Flags().StringVar(&targetNode, "to", "", "Target node ID")
	mustMarkFlagRequired(cmd, "to")
	return cmd
}

func newFailoverCmd() *cobra.Command {
	var targetNode string
	cmd := newDeployOperationCmd(deployOperationCommandSpec{
		Use:   "failover app NAME",
		Short: "Move failed app workloads to another node",
		Long:  `Moves failed workloads, or selected workloads, to --to when provided or another available node.`,
		Label: "failover",
		Action: func(ctx context.Context, c *apiclient.Client, namespace, name string, workloads []string) (*api.DeployOperationOutput, error) {
			return c.FailoverDeploy(ctx, namespace, name, targetNode, workloads)
		},
	})
	cmd.Flags().StringVar(&targetNode, "to", "", "Optional target node ID")
	return cmd
}

func newRebalanceCmd() *cobra.Command {
	return newDeployOperationCmd(deployOperationCommandSpec{
		Use:   "rebalance app NAME",
		Short: "Re-run placement and move workloads when needed",
		Long:  `Re-runs placement for selected workloads and migrates only those whose selected node changes.`,
		Label: "rebalance",
		Action: func(ctx context.Context, c *apiclient.Client, namespace, name string, workloads []string) (*api.DeployOperationOutput, error) {
			return c.RebalanceDeploy(ctx, namespace, name, workloads)
		},
	})
}

type appStatusBody struct {
	App       string
	Namespace string
	Status    string
}

type appStatusAction func(context.Context, *apiclient.Client, string, string) (appStatusBody, any, error)

type appStatusCommandSpec struct {
	Use     string
	Aliases []string
	Short   string
	Long    string
	Label   string
	Action  appStatusAction
}

func newAppStatusCmd(spec appStatusCommandSpec) *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     spec.Use,
		Aliases: spec.Aliases,
		Short:   spec.Short,
		Long:    spec.Long,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppStatusCommand(cmd, args, namespace, jsonOut, spec)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "App namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func runAppStatusCommand(cmd *cobra.Command, args []string, namespace string, jsonOut bool, spec appStatusCommandSpec) error {
	if args[0] != "app" {
		return oopsx.B("cli").Errorf("expected resource type app, got %q", args[0])
	}
	ctx := contextFromCmd(cmd)
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		body, raw, err := spec.Action(ctx, c, namespace, args[1])
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "%s app", spec.Label)
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(raw)
		}
		return writeInfoLine(spec.Label,
			viewField("status", statusBadge(body.Status)),
			viewField("app", body.App),
			viewField("namespace", body.Namespace),
		)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "%s app command", spec.Label)
	}
	return nil
}

func deleteAppStatus(ctx context.Context, c *apiclient.Client, namespace, name string) (appStatusBody, any, error) {
	out, err := c.DeleteDeploy(ctx, namespace, name)
	if err != nil {
		return appStatusBody{}, nil, oopsx.B("cli").Wrapf(err, "delete deploy")
	}
	return appStatusBody{App: out.Body.App, Namespace: out.Body.Namespace, Status: out.Body.Status}, out.Body, nil
}

func startAppStatus(ctx context.Context, c *apiclient.Client, namespace, name string) (appStatusBody, any, error) {
	out, err := c.StartDeploy(ctx, namespace, name)
	if err != nil {
		return appStatusBody{}, nil, oopsx.B("cli").Wrapf(err, "start deploy")
	}
	return appStatusBody{App: out.Body.App, Namespace: out.Body.Namespace, Status: out.Body.Status}, out.Body, nil
}

func stopAppStatus(ctx context.Context, c *apiclient.Client, namespace, name string) (appStatusBody, any, error) {
	out, err := c.StopDeploy(ctx, namespace, name)
	if err != nil {
		return appStatusBody{}, nil, oopsx.B("cli").Wrapf(err, "stop deploy")
	}
	return appStatusBody{App: out.Body.App, Namespace: out.Body.Namespace, Status: out.Body.Status}, out.Body, nil
}

func restartAppStatus(ctx context.Context, c *apiclient.Client, namespace, name string) (appStatusBody, any, error) {
	out, err := c.RestartDeploy(ctx, namespace, name)
	if err != nil {
		return appStatusBody{}, nil, oopsx.B("cli").Wrapf(err, "restart deploy")
	}
	return appStatusBody{App: out.Body.App, Namespace: out.Body.Namespace, Status: out.Body.Status}, out.Body, nil
}

func rollbackAppStatus(ctx context.Context, c *apiclient.Client, namespace, name string) (appStatusBody, any, error) {
	out, err := c.RollbackDeploy(ctx, namespace, name)
	if err != nil {
		return appStatusBody{}, nil, oopsx.B("cli").Wrapf(err, "rollback deploy")
	}
	return appStatusBody{App: out.Body.App, Namespace: out.Body.Namespace, Status: out.Body.Status}, out.Body, nil
}

type deployOperationAction func(context.Context, *apiclient.Client, string, string, []string) (*api.DeployOperationOutput, error)

type deployOperationCommandSpec struct {
	Use    string
	Short  string
	Long   string
	Label  string
	Action deployOperationAction
}

func newDeployOperationCmd(spec deployOperationCommandSpec) *cobra.Command {
	var namespace string
	var workloads []string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeployOperationCommand(cmd, args, namespace, workloads, jsonOut, spec)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "App namespace")
	cmd.Flags().StringArrayVar(&workloads, "workload", nil, "Workload name to operate on (repeatable; default operation-specific)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func runDeployOperationCommand(cmd *cobra.Command, args []string, namespace string, workloads []string, jsonOut bool, spec deployOperationCommandSpec) error {
	if args[0] != "app" {
		return oopsx.B("cli").Errorf("expected resource type app, got %q", args[0])
	}
	ctx := contextFromCmd(cmd)
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := spec.Action(ctx, c, namespace, args[1], workloads)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "%s app", spec.Label)
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body)
		}
		return writeDeployOperationHuman(spec.Label, out)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "%s app command", spec.Label)
	}
	return nil
}
