package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go_pet_project/internal/platform/config"
	"go_pet_project/internal/platform/migrations"

	_ "github.com/lib/pq"
)

const migrationsDir = "../../migrations"

func TestUserAndRefreshTokenMigrations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := loadConfig(t)
	startPostgres(ctx, t)
	waitForPostgres(ctx, t, cfg.DB)

	testDBName := fmt.Sprintf("todo_test_%d_%d", os.Getpid(), time.Now().UnixNano())
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

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if err := migrations.Up(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration up: %v", err)
	}

	db, err := sql.Open("postgres", testCfg.URL())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()

	assertUsersSchema(ctx, t, db)
	assertRefreshTokensSchema(ctx, t, db)
	assertTasksSchema(ctx, t, db)
	assertUserConstraints(ctx, t, db)
	assertRefreshTokenConstraints(ctx, t, db)
	assertTaskConstraints(ctx, t, db)

	if err := migrations.Down(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration down: %v", err)
	}
	assertTableMissing(ctx, t, db, "tasks")
	assertTableMissing(ctx, t, db, "refresh_tokens")
	assertTableMissing(ctx, t, db, "users")

	if err := migrations.Up(ctx, testCfg, migrationsDir, logger); err != nil {
		t.Fatalf("migration up after down: %v", err)
	}
	assertTableExists(ctx, t, db, "users")
	assertTableExists(ctx, t, db, "refresh_tokens")
	assertTableExists(ctx, t, db, "tasks")
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	return cfg
}

func startPostgres(ctx context.Context, t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker CLI is required for integration tests")
	}

	infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if output, err := exec.CommandContext(infoCtx, "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Docker daemon is required for integration tests: %s", strings.TrimSpace(string(output)))
	}

	composeCtx, composeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer composeCancel()

	cmd := exec.CommandContext(composeCtx, "docker", "compose", "-f", "../../deploy/compose.yaml", "up", "-d")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose up: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func waitForPostgres(ctx context.Context, t *testing.T, cfg config.DBConfig) {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for {
		db, err := sql.Open("postgres", cfg.URL())
		if err == nil {
			pingErr := db.PingContext(ctx)
			db.Close()
			if pingErr == nil {
				return
			}
			err = pingErr
		}

		if time.Now().After(deadline) {
			t.Fatalf("postgres did not become ready: %v", err)
		}

		select {
		case <-ctx.Done():
			t.Fatalf("wait postgres: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func openAdminDB(ctx context.Context, t *testing.T, cfg config.DBConfig) *sql.DB {
	t.Helper()

	adminCfg := cfg
	adminCfg.Name = "postgres"

	db, err := sql.Open("postgres", adminCfg.URL())
	if err != nil {
		t.Fatalf("open admin database: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("ping admin database: %v", err)
	}

	return db
}

func createTestDatabase(ctx context.Context, t *testing.T, db *sql.DB, name string) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `CREATE DATABASE `+quoteIdent(name)); err != nil {
		t.Fatalf("create test database: %v", err)
	}
}

func dropTestDatabase(ctx context.Context, t *testing.T, db *sql.DB, name string) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()
	`, name); err != nil {
		t.Logf("terminate test database connections: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DROP DATABASE IF EXISTS `+quoteIdent(name)); err != nil {
		t.Logf("drop test database: %v", err)
	}
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func assertUsersSchema(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	assertTableExists(ctx, t, db, "users")
	assertColumn(ctx, t, db, "users", "id", "uuid", "NO")
	assertColumn(ctx, t, db, "users", "email", "text", "NO")
	assertColumn(ctx, t, db, "users", "username", "text", "NO")
	assertColumn(ctx, t, db, "users", "password_hash", "text", "NO")
	assertColumn(ctx, t, db, "users", "timezone", "text", "NO")
	assertColumn(ctx, t, db, "users", "created_at", "timestamp with time zone", "NO")
	assertColumn(ctx, t, db, "users", "updated_at", "timestamp with time zone", "NO")
	assertPrimaryKey(ctx, t, db, "users", "users_pkey", "id")
	assertUniqueConstraint(ctx, t, db, "users", "users_email_key", "email")
	assertUniqueConstraint(ctx, t, db, "users", "users_username_key", "username")
}

func assertRefreshTokensSchema(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	assertTableExists(ctx, t, db, "refresh_tokens")
	assertColumn(ctx, t, db, "refresh_tokens", "id", "uuid", "NO")
	assertColumn(ctx, t, db, "refresh_tokens", "user_id", "uuid", "NO")
	assertColumn(ctx, t, db, "refresh_tokens", "token_hash", "text", "NO")
	assertColumn(ctx, t, db, "refresh_tokens", "expires_at", "timestamp with time zone", "NO")
	assertColumn(ctx, t, db, "refresh_tokens", "revoked_at", "timestamp with time zone", "YES")
	assertColumn(ctx, t, db, "refresh_tokens", "created_at", "timestamp with time zone", "NO")
	assertPrimaryKey(ctx, t, db, "refresh_tokens", "refresh_tokens_pkey", "id")
	assertForeignKey(ctx, t, db, "refresh_tokens", "refresh_tokens_user_id_fkey", "users", "CASCADE")
	assertIndex(ctx, t, db, "refresh_tokens_user_id_idx")
	assertUniqueIndex(ctx, t, db, "refresh_tokens_token_hash_key")
}

func assertTasksSchema(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	assertTableExists(ctx, t, db, "tasks")
	assertColumn(ctx, t, db, "tasks", "id", "uuid", "NO")
	assertColumn(ctx, t, db, "tasks", "series_id", "uuid", "YES")
	assertColumn(ctx, t, db, "tasks", "creator_id", "uuid", "NO")
	assertColumn(ctx, t, db, "tasks", "assignee_id", "uuid", "NO")
	assertColumn(ctx, t, db, "tasks", "title", "text", "NO")
	assertColumn(ctx, t, db, "tasks", "description", "text", "NO")
	assertColumn(ctx, t, db, "tasks", "status", "text", "NO")
	assertColumn(ctx, t, db, "tasks", "deadline_at", "timestamp with time zone", "YES")
	assertColumn(ctx, t, db, "tasks", "created_at", "timestamp with time zone", "NO")
	assertColumn(ctx, t, db, "tasks", "updated_at", "timestamp with time zone", "NO")
	assertColumn(ctx, t, db, "tasks", "completed_at", "timestamp with time zone", "YES")
	assertColumn(ctx, t, db, "tasks", "archived_at", "timestamp with time zone", "YES")
	assertPrimaryKey(ctx, t, db, "tasks", "tasks_pkey", "id")
	assertForeignKey(ctx, t, db, "tasks", "tasks_creator_id_fkey", "users", "NO ACTION")
	assertForeignKey(ctx, t, db, "tasks", "tasks_assignee_id_fkey", "users", "NO ACTION")
	assertIndexColumns(ctx, t, db, "tasks_assignee_status_deadline_idx", "assignee_id", "status", "deadline_at")
	assertIndexColumns(ctx, t, db, "tasks_creator_created_at_idx", "creator_id", "created_at")
	assertIndexColumns(ctx, t, db, "tasks_deadline_at_idx", "deadline_at")
}

func assertTableExists(ctx context.Context, t *testing.T, db *sql.DB, table string) {
	t.Helper()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s exists: %v", table, err)
	}
	if !exists {
		t.Fatalf("expected table %s to exist", table)
	}
}

func assertTableMissing(ctx context.Context, t *testing.T, db *sql.DB, table string) {
	t.Helper()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s missing: %v", table, err)
	}
	if exists {
		t.Fatalf("expected table %s to be dropped", table)
	}
}

func assertColumn(ctx context.Context, t *testing.T, db *sql.DB, table string, column string, dataType string, nullable string) {
	t.Helper()

	var actualType, actualNullable string
	err := db.QueryRowContext(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	`, table, column).Scan(&actualType, &actualNullable)
	if err != nil {
		t.Fatalf("column %s.%s: %v", table, column, err)
	}
	if actualType != dataType || actualNullable != nullable {
		t.Fatalf("column %s.%s = (%s, nullable %s), want (%s, nullable %s)", table, column, actualType, actualNullable, dataType, nullable)
	}
}

func assertPrimaryKey(ctx context.Context, t *testing.T, db *sql.DB, table string, constraint string, column string) {
	t.Helper()

	assertConstraintColumns(ctx, t, db, table, constraint, "PRIMARY KEY", column)
}

func assertUniqueConstraint(ctx context.Context, t *testing.T, db *sql.DB, table string, constraint string, column string) {
	t.Helper()

	assertConstraintColumns(ctx, t, db, table, constraint, "UNIQUE", column)
}

func assertConstraintColumns(ctx context.Context, t *testing.T, db *sql.DB, table string, constraint string, constraintType string, column string) {
	t.Helper()

	var actualColumn string
	err := db.QueryRowContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_schema = kcu.constraint_schema
			AND tc.constraint_name = kcu.constraint_name
			AND tc.table_name = kcu.table_name
		WHERE tc.table_schema = 'public'
			AND tc.table_name = $1
			AND tc.constraint_name = $2
			AND tc.constraint_type = $3
	`, table, constraint, constraintType).Scan(&actualColumn)
	if err != nil {
		t.Fatalf("constraint %s on %s: %v", constraint, table, err)
	}
	if actualColumn != column {
		t.Fatalf("constraint %s column = %s, want %s", constraint, actualColumn, column)
	}
}

func assertForeignKey(ctx context.Context, t *testing.T, db *sql.DB, table string, constraint string, foreignTable string, deleteRule string) {
	t.Helper()

	var actualForeignTable, actualDeleteRule string
	err := db.QueryRowContext(ctx, `
		SELECT ccu.table_name, rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_schema = ccu.constraint_schema
			AND tc.constraint_name = ccu.constraint_name
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_schema = rc.constraint_schema
			AND tc.constraint_name = rc.constraint_name
		WHERE tc.table_schema = 'public'
			AND tc.table_name = $1
			AND tc.constraint_name = $2
			AND tc.constraint_type = 'FOREIGN KEY'
	`, table, constraint).Scan(&actualForeignTable, &actualDeleteRule)
	if err != nil {
		t.Fatalf("foreign key %s on %s: %v", constraint, table, err)
	}
	if actualForeignTable != foreignTable || actualDeleteRule != deleteRule {
		t.Fatalf("foreign key %s references %s on delete %s, want %s on delete %s", constraint, actualForeignTable, actualDeleteRule, foreignTable, deleteRule)
	}
}

func assertIndex(ctx context.Context, t *testing.T, db *sql.DB, index string) {
	t.Helper()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public' AND indexname = $1
		)
	`, index).Scan(&exists)
	if err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	if !exists {
		t.Fatalf("expected index %s to exist", index)
	}
}

func assertUniqueIndex(ctx context.Context, t *testing.T, db *sql.DB, index string) {
	t.Helper()

	var unique bool
	err := db.QueryRowContext(ctx, `
		SELECT i.indisunique
		FROM pg_class c
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = $1
	`, index).Scan(&unique)
	if err != nil {
		t.Fatalf("check unique index %s: %v", index, err)
	}
	if !unique {
		t.Fatalf("expected index %s to be unique", index)
	}
}

func assertIndexColumns(ctx context.Context, t *testing.T, db *sql.DB, index string, columns ...string) {
	t.Helper()

	rows, err := db.QueryContext(ctx, `
		SELECT a.attname
		FROM pg_class idx
		JOIN pg_index i ON i.indexrelid = idx.oid
		JOIN pg_class tbl ON tbl.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = idx.relnamespace
		JOIN unnest(i.indkey) WITH ORDINALITY AS key(attnum, ordinality) ON true
		JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = key.attnum
		WHERE n.nspname = 'public' AND idx.relname = $1
		ORDER BY key.ordinality
	`, index)
	if err != nil {
		t.Fatalf("read index %s columns: %v", index, err)
	}
	defer rows.Close()

	var actual []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan index %s column: %v", index, err)
		}
		actual = append(actual, column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate index %s columns: %v", index, err)
	}
	if strings.Join(actual, ",") != strings.Join(columns, ",") {
		t.Fatalf("index %s columns = %v, want %v", index, actual, columns)
	}
}

func assertUserConstraints(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	insertUser(ctx, t, db, "10000000-0000-4000-8000-000000000001", "test@example.com", "tester", true, true)
	insertUser(ctx, t, db, "10000000-0000-4000-8000-000000000002", "other@example.com", "tester2", true, true)

	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, timezone)
		VALUES ('10000000-0000-4000-8000-000000000003', 'test@example.com', 'unique_user', 'hash', 'UTC')
	`)
	expectExecError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash, timezone)
		VALUES ('10000000-0000-4000-8000-000000000004', 'unique@example.com', 'tester', 'hash', 'UTC')
	`)
	expectExecError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, timezone)
		VALUES ('10000000-0000-4000-8000-000000000005', 'missing_password@example.com', 'missing_password', 'UTC')
	`)
	expectExecError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, password_hash)
		VALUES ('10000000-0000-4000-8000-000000000006', 'missing_timezone@example.com', 'missing_timezone', 'hash')
	`)
	expectExecError(t, err)
}

func assertRefreshTokenConstraints(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	userID := "20000000-0000-4000-8000-000000000001"
	insertUser(ctx, t, db, userID, "tokens@example.com", "token_user", true, true)

	for i := 1; i <= 3; i++ {
		tokenID := fmt.Sprintf("30000000-0000-4000-8000-%012d", i)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP + INTERVAL '24 hours')
		`, tokenID, userID, fmt.Sprintf("token_hash_%d", i)); err != nil {
			t.Fatalf("insert refresh token %d: %v", i, err)
		}
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = '30000000-0000-4000-8000-000000000001'
	`); err != nil {
		t.Fatalf("set revoked_at: %v", err)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES (
			'30000000-0000-4000-8000-000000000004',
			'99999999-9999-4999-8999-999999999999',
			'orphan_hash',
			CURRENT_TIMESTAMP + INTERVAL '24 hours'
		)
	`)
	expectExecError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash)
		VALUES ('30000000-0000-4000-8000-000000000005', $1, 'missing_expiration_hash')
	`, userID)
	expectExecError(t, err)

	var tokenCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1`, userID).Scan(&tokenCount); err != nil {
		t.Fatalf("count refresh tokens: %v", err)
	}
	if tokenCount != 3 {
		t.Fatalf("refresh token count = %d, want 3", tokenCount)
	}

	var revokedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NOT NULL`, userID).Scan(&revokedCount); err != nil {
		t.Fatalf("count revoked refresh tokens: %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("revoked refresh token count = %d, want 1", revokedCount)
	}
}

func assertTaskConstraints(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	creatorID := "40000000-0000-4000-8000-000000000001"
	assigneeID := "40000000-0000-4000-8000-000000000002"
	insertUser(ctx, t, db, creatorID, "task-creator@example.com", "task_creator", true, true)
	insertUser(ctx, t, db, assigneeID, "task-assignee@example.com", "task_assignee", true, true)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (
			id,
			creator_id,
			assignee_id,
			title,
			description,
			status,
			deadline_at
		)
		VALUES (
			'50000000-0000-4000-8000-000000000001',
			$1,
			$2,
			'Cross-user task',
			'',
			'OPEN',
			CURRENT_TIMESTAMP + INTERVAL '24 hours'
		)
	`, creatorID, assigneeID); err != nil {
		t.Fatalf("insert task with distinct creator/assignee: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (
			id,
			creator_id,
			assignee_id,
			title,
			status,
			archived_at
		)
		VALUES (
			'50000000-0000-4000-8000-000000000002',
			$1,
			$1,
			'Self-assigned archived task',
			'DONE',
			CURRENT_TIMESTAMP
		)
	`, creatorID); err != nil {
		t.Fatalf("insert self-assigned task: %v", err)
	}

	var activeArchivedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `
		SELECT archived_at
		FROM tasks
		WHERE id = '50000000-0000-4000-8000-000000000001'
	`).Scan(&activeArchivedAt); err != nil {
		t.Fatalf("read active task archived_at: %v", err)
	}
	if activeArchivedAt.Valid {
		t.Fatalf("active task archived_at valid = true, want false")
	}

	var archivedArchivedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `
		SELECT archived_at
		FROM tasks
		WHERE id = '50000000-0000-4000-8000-000000000002'
	`).Scan(&archivedArchivedAt); err != nil {
		t.Fatalf("read archived task archived_at: %v", err)
	}
	if !archivedArchivedAt.Valid {
		t.Fatalf("archived task archived_at valid = false, want true")
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO tasks (id, creator_id, assignee_id, title, status)
		VALUES (
			'50000000-0000-4000-8000-000000000003',
			'99999999-9999-4999-8999-999999999999',
			$1,
			'Missing creator',
			'OPEN'
		)
	`, assigneeID)
	expectExecError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO tasks (id, creator_id, assignee_id, title, status)
		VALUES (
			'50000000-0000-4000-8000-000000000004',
			$1,
			'99999999-9999-4999-8999-999999999999',
			'Missing assignee',
			'OPEN'
		)
	`, creatorID)
	expectExecError(t, err)

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 2 {
		t.Fatalf("task count = %d, want 2", count)
	}
}

func insertUser(ctx context.Context, t *testing.T, db *sql.DB, id string, email string, username string, withPassword bool, withTimezone bool) {
	t.Helper()

	columns := []string{"id", "email", "username"}
	args := []any{id, email, username}
	placeholders := []string{"$1", "$2", "$3"}

	if withPassword {
		columns = append(columns, "password_hash")
		args = append(args, "password_hash")
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}

	if withTimezone {
		columns = append(columns, "timezone")
		args = append(args, "UTC")
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}

	query := fmt.Sprintf("INSERT INTO users (%s) VALUES (%s)", strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func expectExecError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected PostgreSQL constraint error, got nil")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected PostgreSQL constraint error, got context error: %v", err)
	}
}
