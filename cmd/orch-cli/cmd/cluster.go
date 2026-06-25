// Package cmd contains cobra command wiring for orch-cli.
package cmd

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/lyonbrown4d/orch/cmd/orch-cli/cliapp"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func newHealthCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check connectivity to the orch control plane",
		Long:  `Contacts the server configured with --server (or ORCH_SERVER) and reports readiness.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			conn := cliapp.ConnFromGlobals(serverURL, authToken)
			return cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
				out, err := c.Health(ctx)
				if err != nil {
					return oopsx.B("cli").Wrapf(err, "health")
				}
				if jsonOut {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(out)
				}
				return writeInfoLine("health",
					viewField("status", statusBadge(out.Body.Status)),
					viewField("time", out.Body.Timestamp),
				)
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newHostinfoCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "hostinfo",
		Short: "Show CPU, memory, load, and disk snapshot from the server host",
		Long:  `Diagnostics for the node your --server resolves to (or any peer in a cluster when pointed at it).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			conn := cliapp.ConnFromGlobals(serverURL, authToken)
			return cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
				out, err := c.Hostinfo(ctx)
				if err != nil {
					return oopsx.B("cli").Wrapf(err, "hostinfo")
				}
				if jsonOut {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(out.Body)
				}
				return writeHostinfoHuman(out)
			})
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON (full hostinfo report)")
	return cmd
}

func newWorkloadsCmd() *cobra.Command {
	return newWorkloadsListCmd("workloads", []string{"workload", "ps"})
}

func newWorkloadsListCmd(use string, aliases []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "List workloads registered on the server",
		Long:    `Shows workloads this control plane node knows about for the current cluster context (--server).`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			return runListWorkloads(ctx, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newAssignmentsCmd() *cobra.Command {
	return newAssignmentsListCmd("assignments", []string{"assignment"})
}

func newVolumesCmd() *cobra.Command {
	return newVolumesListCmd("volumes", []string{"volume", "vol"})
}

func newRuntimesCmd() *cobra.Command {
	return newRuntimesListCmd("runtimes", []string{"runtime", "providers"})
}

func newAppsCmd() *cobra.Command {
	return newAppsListCmd("apps", []string{"app"})
}

func newAppsListCmd(use string, aliases []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "List deployed apps",
		Long:    `Shows desired apps with status aggregated from workload assignments.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			return runListApps(ctx, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newAssignmentsListCmd(use string, aliases []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "List scheduler workload assignments",
		Long:    `Shows persisted scheduler decisions and deploy results from the current control plane context (--server).`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			return runListAssignments(ctx, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newVolumesListCmd(use string, aliases []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "List local runtime volume bindings",
		Long:    `Shows persisted local runtime volume bindings from the current control plane context (--server).`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			return runListVolumes(ctx, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newRuntimesListCmd(use string, aliases []string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "List runtime providers on the server node",
		Long:    `Shows runtime provider policy, host availability, and registration status for the current control plane node.`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			return runListRuntimeProviders(ctx, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON array")
	return cmd
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Display cluster resources",
		Long:  `Display cluster resources from the current control plane context (--server), similar to kubectl get.`,
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newAppsListCmd("apps", []string{"app"}))
	cmd.AddCommand(newWorkloadsListCmd("workloads", []string{"workload", "wl", "ps"}))
	cmd.AddCommand(newAssignmentsListCmd("assignments", []string{"assignment", "assign"}))
	cmd.AddCommand(newVolumesListCmd("volumes", []string{"volume", "vol", "pv"}))
	cmd.AddCommand(newRuntimesListCmd("runtimes", []string{"runtime", "providers"}))
	return cmd
}

func newDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Show detailed cluster resource state",
		Long:  `Show detailed resource state from the current control plane context (--server).`,
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newDescribeAppCmd())
	cmd.AddCommand(newDescribeNodeCmd())
	cmd.AddCommand(newDescribeWorkloadCmd())
	return cmd
}

func newDescribeAppCmd() *cobra.Command {
	var namespace string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "app NAME",
		Short: "Describe an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			conn := cliapp.ConnFromGlobals(serverURL, authToken)
			return cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
				out, err := c.GetApp(ctx, namespace, args[0])
				if err != nil {
					return oopsx.B("cli").Wrapf(err, "describe app")
				}
				if jsonOut {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(out.Body)
				}
				return writeAppDetailHuman(&out.Body)
			})
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "App namespace")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}
