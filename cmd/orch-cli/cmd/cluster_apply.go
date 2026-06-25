package cmd

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/spf13/cobra"

	"github.com/lyonbrown4d/orch/cmd/orch-cli/cliapp"
	"github.com/lyonbrown4d/orch/internal/api"
	"github.com/lyonbrown4d/orch/internal/apiclient"
	"github.com/lyonbrown4d/orch/internal/deploy/loader"
	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func newApplyCmd() *cobra.Command {
	var file string
	var jsonOut bool
	var watch bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Deploy a manifest to the cluster from a .orch or YAML file",
		Long: `Reads the deploy file locally and sends its source to orch-server. The server parses the document (virtual path
suffix selects .orch vs YAML), replicates desired state through Raft, then reconciles workloads on each node.
Requires a reachable control plane (--server / ORCH_SERVER); follower nodes forward clustered Raft deploys when configured with the leader API URL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := contextFromCmd(cmd)
			if file == "" {
				return oopsx.B("cli").Errorf("--file is required")
			}
			src, err := readManifestFile(file)
			if err != nil {
				return err
			}
			return runApplyCommand(ctx, applyOptions{
				File:    file,
				Source:  src,
				JSON:    jsonOut,
				Watch:   watch,
				Timeout: timeout,
			})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to deploy file (.orch or YAML)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON response")
	cmd.Flags().BoolVar(&watch, "watch", false, "Wait until deployed workloads are running")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Maximum time to wait with --watch")
	return cmd
}

func runListWorkloads(ctx context.Context, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListWorkloads(ctx)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list workloads")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeWorkloadsHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "list workloads")
	}
	return nil
}

func readManifestFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	dir, name := filepath.Split(clean)
	if name == "" {
		return nil, oopsx.B("cli").Errorf("empty manifest file name")
	}
	if dir == "" {
		dir = "."
	}
	src, err := fs.ReadFile(os.DirFS(dir), name)
	if err != nil {
		return nil, oopsx.B("cli").Wrapf(err, "read manifest file")
	}
	return src, nil
}

func runListAssignments(ctx context.Context, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListAssignments(ctx)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list assignments")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeAssignmentsHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "list assignments")
	}
	return nil
}

func runListVolumes(ctx context.Context, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListVolumes(ctx)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list volumes")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeVolumesHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "list volumes")
	}
	return nil
}

func runListApps(ctx context.Context, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListApps(ctx)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list apps")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeAppsHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "list apps")
	}
	return nil
}

func runListRuntimeProviders(ctx context.Context, jsonOut bool) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, _ *loader.Loader) error {
		out, err := c.ListRuntimeProviders(ctx)
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "list runtime providers")
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out.Body.Items)
		}
		return writeRuntimesHuman(out.Body.Items)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "list runtime providers")
	}
	return nil
}

type applyWatchOutput struct {
	Accepted    bool                           `json:"accepted"`
	App         string                         `json:"app"`
	Workloads   int                            `json:"workloads"`
	Assignments *list.List[api.AssignmentItem] `json:"assignments"`
	Registry    *list.List[api.WorkloadItem]   `json:"registry"`
}

type applyOptions struct {
	File    string
	Source  []byte
	JSON    bool
	Watch   bool
	Timeout time.Duration
}

func runApplyCommand(ctx context.Context, opts applyOptions) error {
	conn := cliapp.ConnFromGlobals(serverURL, authToken)
	if err := cliapp.RunCluster(ctx, conn, func(ctx context.Context, c *apiclient.Client, deploy *loader.Loader) error {
		var app *deployv1.App
		if opts.Watch {
			var err error
			app, err = loadWatchApp(ctx, deploy, opts)
			if err != nil {
				return err
			}
		}
		out, err := c.DeploySource(ctx, filepath.Base(opts.File), string(opts.Source))
		if err != nil {
			return oopsx.B("cli").Wrapf(err, "deploy")
		}
		return writeApplyResult(ctx, c, app, out, opts)
	}); err != nil {
		return oopsx.B("cli").Wrapf(err, "apply")
	}
	return nil
}

func loadWatchApp(ctx context.Context, deploy *loader.Loader, opts applyOptions) (*deployv1.App, error) {
	app, err := deploy.LoadAppString(ctx, filepath.Base(opts.File), string(opts.Source))
	if err != nil {
		return nil, oopsx.B("cli").Wrapf(err, "parse manifest for watch")
	}
	return app, nil
}

func writeApplyResult(ctx context.Context, c *apiclient.Client, app *deployv1.App, out *api.DeployOutput, opts applyOptions) error {
	if opts.JSON && !opts.Watch {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return oopsx.B("cli").Wrapf(err, "write apply json")
		}
		return nil
	}
	if err := writeApplyAccepted(out, opts.JSON); err != nil {
		return err
	}
	if !opts.Watch {
		return nil
	}
	snapshot, err := watchDeployment(ctx, c, app, opts.Timeout, !opts.JSON)
	if err != nil {
		return err
	}
	return writeApplyWatch(out, snapshot, opts.JSON)
}

func writeApplyAccepted(out *api.DeployOutput, jsonOut bool) error {
	if jsonOut {
		return nil
	}
	return writeInfoLine("deploy",
		viewField("status", statusBadge("accepted")),
		viewField("app", out.Body.App),
		viewField("workloads", strconv.Itoa(out.Body.Workloads)),
	)
}

func writeApplyWatch(out *api.DeployOutput, snapshot *deploySnapshot, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(applyWatchOutput{
			Accepted:    out.Body.Accepted,
			App:         out.Body.App,
			Workloads:   out.Body.Workloads,
			Assignments: snapshot.Assignments,
			Registry:    snapshot.Workloads,
		}); err != nil {
			return oopsx.B("cli").Wrapf(err, "write apply watch json")
		}
		return nil
	}
	if err := writeAssignmentsHuman(snapshot.Assignments); err != nil {
		return err
	}
	return writeWorkloadsHuman(snapshot.Workloads)
}
