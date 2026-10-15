package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/malekachouri/terraform-drift-detector/internal/api"
	"github.com/malekachouri/terraform-drift-detector/internal/config"
	"github.com/malekachouri/terraform-drift-detector/internal/scan"
	"github.com/malekachouri/terraform-drift-detector/internal/scheduler"
	"github.com/malekachouri/terraform-drift-detector/internal/store"
)

func main() {
	cfgPath := flag.String("config", "configs/driftctl.yaml", "path to config file")
	flag.Parse()

	if err := run(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "drift-server: %v\n", err)
		os.Exit(1)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	st, err := store.OpenSQLite(cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	for i := range cfg.Workspaces {
		ws := &cfg.Workspaces[i]
		if err := st.SaveWorkspace(ctx, ws); err != nil {
			return fmt.Errorf("save workspace %s: %w", ws.Name, err)
		}
		if ws.Schedule != nil && ws.Schedule.Cron != "" {
			if err := st.SaveSchedule(ctx, ws.ID, ws.Schedule.Cron); err != nil {
				return fmt.Errorf("save schedule %s: %w", ws.Name, err)
			}
		}
		log.Printf("workspace %q (endpoint=%q, state=%s)", ws.Name, ws.Endpoint, ws.State.Backend)
	}

	scanner := scan.NewScanner(st)
	sched := scheduler.New(scanner, st)
	if err := sched.LoadFromStore(ctx); err != nil {
		return fmt.Errorf("load schedules: %w", err)
	}
	sched.Start()
	defer sched.Stop()

	srv := api.NewServer(cfg.API, st, scanner, sched)

	runCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return api.Run(runCtx, cfg.API.Addr, srv.Handler())
}
