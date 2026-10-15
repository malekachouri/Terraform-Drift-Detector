package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// A workspace re-saved from config (new random id) must keep its stored id,
// otherwise schedules saved under the old id are orphaned.
func TestSaveWorkspaceKeepsID(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	ws := &model.Workspace{Name: "ls", Provider: "aws", Endpoint: "http://localhost:4566"}
	if err := st.SaveWorkspace(ctx, ws); err != nil {
		t.Fatal(err)
	}
	first := ws.ID

	again := &model.Workspace{Name: "ls", Provider: "aws", Endpoint: "http://localstack:4566"}
	if err := st.SaveWorkspace(ctx, again); err != nil {
		t.Fatal(err)
	}
	if again.ID != first {
		t.Fatalf("id changed on upsert: %s -> %s", first, again.ID)
	}
	got, err := st.GetWorkspace(ctx, first)
	if err != nil || got.Endpoint != "http://localstack:4566" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
