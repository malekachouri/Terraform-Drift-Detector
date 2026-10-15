package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/malekachouri/terraform-drift-detector/internal/config"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
	"github.com/malekachouri/terraform-drift-detector/internal/output"
	"github.com/malekachouri/terraform-drift-detector/internal/scan"
	"github.com/malekachouri/terraform-drift-detector/internal/store"
)

// Exit codes: 0 = no drift, 1 = drift detected, 2 = error.
const (
	exitDrift = 1
	exitError = 2
)

// errDrift signals "scan worked, drift found" so main can exit with 1.
var errDrift = errors.New("drift detected")

const localstackEndpoint = "http://localhost:4566"

func main() {
	err := newRootCmd().Execute()
	switch {
	case err == nil:
	case errors.Is(err, errDrift):
		os.Exit(exitDrift)
	default:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}
}

func newRootCmd() *cobra.Command {
	var cfgFile string
	root := &cobra.Command{
		Use:           "driftctl",
		Short:         "Terraform drift detection CLI (works with AWS and LocalStack)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "path to driftctl.yaml")
	root.AddCommand(newScanCmd(&cfgFile))
	root.AddCommand(newReportCmd(&cfgFile))
	root.AddCommand(newWorkspaceCmd(&cfgFile))
	root.AddCommand(newScheduleCmd(&cfgFile))
	return root
}

func openStore(cfgFile string) (store.Store, *config.File, error) {
	dbPath := config.ExpandEnv("${DRIFTCTL_DB:-driftctl.db}")
	var cfg *config.File
	if cfgFile != "" {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return nil, nil, err
		}
		dbPath = cfg.Database
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		return nil, nil, err
	}
	if cfg != nil {
		for i := range cfg.Workspaces {
			if err := st.SaveWorkspace(context.Background(), &cfg.Workspaces[i]); err != nil {
				st.Close()
				return nil, nil, err
			}
		}
	}
	return st, cfg, nil
}

func newScanCmd(cfgFile *string) *cobra.Command {
	var (
		workspace       string
		statePath       string
		provider        string
		regions         []string
		outputFmt       string
		skipCloud       bool
		stateBucket     string
		stateKey        string
		stateRegion     string
		endpoint        string
		localstack      bool
		detectUnmanaged bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a drift scan",
		Example: `  # Workspace from config
  driftctl scan --config configs/driftctl.yaml --workspace localstack

  # Ad-hoc: state in LocalStack S3, resources in LocalStack
  driftctl scan --localstack --state-bucket tf-state --state-key demo/terraform.tfstate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore(*cfgFile)
			if err != nil {
				return err
			}
			defer st.Close()

			var opts scan.Options
			if workspace != "" {
				ws, err := st.GetWorkspaceByName(cmd.Context(), workspace)
				if err != nil {
					return fmt.Errorf("workspace %q not found (did you pass --config?): %w", workspace, err)
				}
				opts = scan.OptionsFromWorkspace(ws)
			} else {
				if statePath == "" && stateBucket == "" {
					return fmt.Errorf("either --workspace, --state, or --state-bucket is required")
				}
				opts = scan.Options{
					WorkspaceName: "adhoc",
					Provider:      provider,
					Regions:       regions,
					Compare:       model.CompareConfig{DetectUnmanaged: detectUnmanaged},
				}
				if stateBucket != "" {
					opts.State = model.StateConfig{Backend: "s3", Bucket: stateBucket, Key: stateKey, Region: stateRegion}
				} else {
					opts.State = model.StateConfig{Backend: "local", Path: statePath}
				}
			}
			if localstack && endpoint == "" {
				endpoint = localstackEndpoint
			}
			if endpoint != "" {
				opts.Endpoint = endpoint
			}
			if cmd.Flags().Changed("region") {
				opts.Regions = regions
			}
			if cmd.Flags().Changed("detect-unmanaged") {
				opts.Compare.DetectUnmanaged = detectUnmanaged
			}
			opts.SkipCloud = skipCloud

			report, err := scan.NewScanner(st).Run(cmd.Context(), opts)
			if report == nil {
				return err
			}
			if ferr := output.Format(os.Stdout, report, outputFmt); ferr != nil {
				return ferr
			}
			if err != nil {
				return err
			}
			if output.HasDrift(report) {
				return errDrift
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace name (from --config)")
	cmd.Flags().StringVar(&statePath, "state", "", "path to terraform.tfstate")
	cmd.Flags().StringVar(&provider, "provider", "aws", "cloud provider")
	cmd.Flags().StringSliceVar(&regions, "region", []string{"us-east-1"}, "cloud regions (repeatable)")
	cmd.Flags().StringVar(&outputFmt, "output", "table", "output format: json|table")
	cmd.Flags().BoolVar(&skipCloud, "skip-cloud", false, "skip cloud fetch (state-only, for testing)")
	cmd.Flags().StringVar(&stateBucket, "state-bucket", "", "S3 bucket for terraform state")
	cmd.Flags().StringVar(&stateKey, "state-key", "terraform.tfstate", "S3 key for terraform state")
	cmd.Flags().StringVar(&stateRegion, "state-region", "us-east-1", "S3 region for terraform state")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "custom AWS endpoint, e.g. http://localhost:4566 (also: AWS_ENDPOINT_URL)")
	cmd.Flags().BoolVar(&localstack, "localstack", false, "shortcut for --endpoint "+localstackEndpoint)
	cmd.Flags().BoolVar(&detectUnmanaged, "detect-unmanaged", true, "report cloud resources that are not in the state")
	return cmd
}

func newReportCmd(cfgFile *string) *cobra.Command {
	var outputFmt string
	cmd := &cobra.Command{
		Use:   "report [scan-id]",
		Short: "Show a saved drift report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore(*cfgFile)
			if err != nil {
				return err
			}
			defer st.Close()
			report, err := st.GetScan(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("scan %s: %w", args[0], err)
			}
			return output.Format(os.Stdout, report, outputFmt)
		},
	}
	cmd.Flags().StringVar(&outputFmt, "output", "table", "output format: json|table")
	return cmd
}

func newWorkspaceCmd(cfgFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore(*cfgFile)
			if err != nil {
				return err
			}
			defer st.Close()
			list, err := st.ListWorkspaces(cmd.Context())
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(list)
		},
	})
	return cmd
}

func newScheduleCmd(cfgFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scan schedules (executed by drift-server)",
	}
	var cronExpr, workspace string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create or update a schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := openStore(*cfgFile)
			if err != nil {
				return err
			}
			defer st.Close()
			ws, err := st.GetWorkspaceByName(cmd.Context(), workspace)
			if err != nil {
				return fmt.Errorf("workspace %q: %w", workspace, err)
			}
			if err := st.SaveSchedule(cmd.Context(), ws.ID, cronExpr); err != nil {
				return err
			}
			fmt.Printf("schedule %q saved for workspace %s (picked up on next drift-server start)\n", cronExpr, workspace)
			return nil
		},
	}
	create.Flags().StringVar(&workspace, "workspace", "", "workspace name")
	create.Flags().StringVar(&cronExpr, "cron", "", "cron expression, e.g. \"*/5 * * * *\"")
	_ = create.MarkFlagRequired("workspace")
	_ = create.MarkFlagRequired("cron")
	cmd.AddCommand(create)
	return cmd
}
