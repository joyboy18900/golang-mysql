package main_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang-mysql/handler"
	"golang-mysql/mock/mock_repository"
	"golang-mysql/repository"
	"golang-mysql/service"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/mock/gomock"
)

const (
	testDSN        = "root:root@tcp(localhost:3306)/golang_mysql?parseTime=true&interpolateParams=true"
	testMigrateDSN = testDSN + "&multiStatements=true"
	testActorCount = 1000
	testRowCount   = 50000
	testBatchSize  = 10000
	targetActorID  = 1
)

func connectTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		t.Skipf("skipping integration test: open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping integration test: mysql not reachable: %v", err)
	}

	return db
}

func newTestMigrate(t *testing.T) *migrate.Migrate {
	t.Helper()

	migrateDB, err := sql.Open("mysql", testMigrateDSN)
	if err != nil {
		t.Fatalf("open migrate db: %v", err)
	}

	driver, err := migratemysql.WithInstance(migrateDB, &migratemysql.Config{})
	if err != nil {
		t.Fatalf("new migrate mysql driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "mysql", driver)
	if err != nil {
		t.Fatalf("new migrate: %v", err)
	}

	return m
}

func migrateTo(t *testing.T, m *migrate.Migrate, version uint) {
	t.Helper()

	if err := m.Migrate(version); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to %d: %v", version, err)
	}
}

func migrateDownAll(t *testing.T, m *migrate.Migrate) {
	t.Helper()

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate down: %v", err)
	}
}

func TestAuditLogMigrationsAndQueryPlan(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	m := newTestMigrate(t)
	defer m.Close()

	migrateDownAll(t, m)
	t.Cleanup(func() {
		m2 := newTestMigrate(t)
		defer m2.Close()
		_ = m2.Down()
	})

	migrateTo(t, m, 1)

	ctx := context.Background()
	repo := repository.NewAuditLogRepositoryDB(db)
	seeder := service.NewSeedService(repo, testActorCount, testBatchSize)

	if _, err := seeder.Seed(ctx, testRowCount); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("table scan before index", func(t *testing.T) {
		rawPlan, err := repo.ExplainListByActor(ctx, targetActorID, 50)
		if err != nil {
			t.Fatalf("explain: %v", err)
		}
		if !strings.Contains(rawPlan, `"access_type": "ALL"`) {
			t.Errorf("expected plan to contain a full table scan (access_type ALL), got: %s", rawPlan)
		}
	})

	migrateTo(t, m, 2)
	if err := repo.Analyze(ctx); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	t.Run("index access after index", func(t *testing.T) {
		rawPlan, err := repo.ExplainListByActor(ctx, targetActorID, 50)
		if err != nil {
			t.Fatalf("explain: %v", err)
		}
		if strings.Contains(rawPlan, `"access_type": "ALL"`) {
			t.Errorf("expected the index to be used instead of a full table scan, got: %s", rawPlan)
		}
		if !strings.Contains(rawPlan, `"key": "idx_audit_log_actor_id_created_at"`) {
			t.Errorf("expected plan to use the new index, got: %s", rawPlan)
		}
	})

	migrateTo(t, m, 3)
	if err := repo.Analyze(ctx); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	t.Run("partition pruning on created_at range", func(t *testing.T) {
		var rawPlan string
		err := db.QueryRowContext(ctx,
			`EXPLAIN FORMAT=JSON SELECT * FROM audit_log
			 WHERE created_at >= '2026-08-01' AND created_at < '2026-09-01' LIMIT 50`,
		).Scan(&rawPlan)
		if err != nil {
			t.Fatalf("explain range query: %v", err)
		}

		for _, other := range []string{"p2026_05", "p2026_06", "p2026_07", "p_max"} {
			if strings.Contains(rawPlan, `"`+other+`"`) {
				t.Errorf("expected only p2026_08 to be scanned, but plan mentions %s: %s", other, rawPlan)
			}
		}
		if !strings.Contains(rawPlan, `"p2026_08"`) {
			t.Errorf("expected plan to mention p2026_08, got: %s", rawPlan)
		}
	})

	t.Run("row visible directly in its month partition", func(t *testing.T) {
		result, err := db.ExecContext(ctx,
			`INSERT INTO audit_log (actor_id, action, entity_type, metadata, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			targetActorID, "create", "order", "{}", time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("insert row for partition check: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id: %v", err)
		}

		const partition = "p2026_06"
		var found bool
		err = db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM audit_log PARTITION ("+partition+") WHERE id = ?)",
			id,
		).Scan(&found)
		if err != nil {
			t.Fatalf("query partition %s: %v", partition, err)
		}
		if !found {
			t.Errorf("expected row %d to be visible in partition %s", id, partition)
		}
	})

	migrateDownAll(t, m)

	t.Run("migrations round trip leaves no audit_log table", func(t *testing.T) {
		var count int
		err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'audit_log'",
		).Scan(&count)
		if err != nil {
			t.Fatalf("check audit_log existence: %v", err)
		}
		if count != 0 {
			t.Error("expected audit_log to not exist after migrating all the way down")
		}
	})
}

func newHandlerTestApp(repo repository.AuditLogRepository) *fiber.App {
	svc := service.NewAuditLogService(repo)
	hdlr := handler.NewAuditLogHandler(svc)

	app := fiber.New()
	app.Post("/audit-log", hdlr.Create)
	app.Get("/audit-log", hdlr.ListByActor)

	return app
}

func TestAuditLogHandler_CreateAndList(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock_repository.NewMockAuditLogRepository(ctrl)

	created := repository.AuditLog{
		ID: 1, ActorID: 42, Action: "login", EntityType: "session",
		Metadata: map[string]any{}, CreatedAt: time.Now(),
	}
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(&created, nil)
	repo.EXPECT().ListByActor(gomock.Any(), int64(42), gomock.Any()).Return([]repository.AuditLog{created}, nil)

	app := newHandlerTestApp(repo)

	createReq := httptest.NewRequest(fiber.MethodPost, "/audit-log", strings.NewReader(
		`{"actor_id":42,"action":"login","entity_type":"session"}`,
	))
	createReq.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(createReq)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("create status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
	}

	listReq := httptest.NewRequest(fiber.MethodGet, "/audit-log?actor_id=42", nil)

	resp, err = app.Test(listReq)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("list status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var envelope struct {
		Data []service.AuditLogResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(envelope.Data))
	}
}

func TestAuditLogHandler_ListRequiresActorID(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := mock_repository.NewMockAuditLogRepository(ctrl)
	app := newHandlerTestApp(repo)

	req := httptest.NewRequest(fiber.MethodGet, "/audit-log", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnprocessableEntity)
	}
}
