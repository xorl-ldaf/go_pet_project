package integration

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	permissionpostgres "go_pet_project/internal/permission/adapter/out/postgres"
	"go_pet_project/internal/platform/database"
	"go_pet_project/internal/platform/migrations"

	"github.com/google/uuid"
)

func TestPostgresPermissionRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := "todo_permission_repository_test_" + uuid.NewString()
	adminDB := openAdminDB(ctx, t, cfg.DB)
	createTestDatabase(ctx, t, adminDB, testDBName)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer dropCancel()
		dropTestDatabase(dropCtx, t, adminDB, testDBName)
		adminDB.Close()
	})

	testCfg := cfg.DB
	testCfg.Name = testDBName

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrations.Up(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration up: %v", err)
	}

	pg, err := database.Open(ctx, testCfg, logger)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pg.Close()

	assignerID := uuid.MustParse("70000000-0000-4000-8000-000000000001")
	assigneeBID := uuid.MustParse("70000000-0000-4000-8000-000000000002")
	assigneeCID := uuid.MustParse("70000000-0000-4000-8000-000000000003")
	otherAssignerID := uuid.MustParse("70000000-0000-4000-8000-000000000004")
	insertUser(ctx, t, pg.SQL, assignerID.String(), "repo-assigner@example.com", "repo_assigner", true, true)
	insertUser(ctx, t, pg.SQL, assigneeBID.String(), "repo-assignee-b@example.com", "repo_assignee_b", true, true)
	insertUser(ctx, t, pg.SQL, assigneeCID.String(), "repo-assignee-c@example.com", "repo_assignee_c", true, true)
	insertUser(ctx, t, pg.SQL, otherAssignerID.String(), "repo-other@example.com", "repo_other", true, true)
	insertAssignmentPermission(ctx, t, pg.SQL, assignerID.String(), assigneeBID.String())
	insertAssignmentPermission(ctx, t, pg.SQL, otherAssignerID.String(), assignerID.String())

	repo := permissionpostgres.NewRepository(pg.GORM)

	exists, err := repo.Exists(ctx, assignerID, assigneeBID)
	if err != nil {
		t.Fatalf("Exists A->B: %v", err)
	}
	if !exists {
		t.Fatalf("Exists A->B = false, want true")
	}

	exists, err = repo.Exists(ctx, assigneeBID, assignerID)
	if err != nil {
		t.Fatalf("Exists B->A: %v", err)
	}
	if exists {
		t.Fatalf("Exists B->A = true, want false for directed permission")
	}

	exists, err = repo.Exists(ctx, assignerID, assigneeCID)
	if err != nil {
		t.Fatalf("Exists A->C: %v", err)
	}
	if exists {
		t.Fatalf("Exists A->C = true, want false")
	}

	insertAssignmentPermission(ctx, t, pg.SQL, assignerID.String(), assigneeCID.String())

	ids, err := repo.ListAssigneeIDs(ctx, assignerID)
	if err != nil {
		t.Fatalf("ListAssigneeIDs: %v", err)
	}

	want := []uuid.UUID{assigneeBID, assigneeCID}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("assignee IDs = %v, want %v", ids, want)
	}
}
