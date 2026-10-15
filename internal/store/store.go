package store

import (
	"context"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// Store persists workspaces, scans, and schedules.
type Store interface {
	Close() error

	SaveWorkspace(ctx context.Context, ws *model.Workspace) error
	GetWorkspace(ctx context.Context, id string) (*model.Workspace, error)
	GetWorkspaceByName(ctx context.Context, name string) (*model.Workspace, error)
	ListWorkspaces(ctx context.Context) ([]model.Workspace, error)
	DeleteWorkspace(ctx context.Context, id string) error

	SaveScan(ctx context.Context, report *model.DriftReport) error
	GetScan(ctx context.Context, id string) (*model.DriftReport, error)
	ListScans(ctx context.Context, workspaceID string, limit int) ([]model.DriftReport, error)

	SaveSchedule(ctx context.Context, workspaceID, cron string) error
	GetSchedule(ctx context.Context, workspaceID string) (string, error)
	ListSchedules(ctx context.Context) (map[string]string, error)
	DeleteSchedule(ctx context.Context, workspaceID string) error
}
