package main_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	testDSN        = "root:root@tcp(localhost:3306)/golang_mysql?parseTime=true&interpolateParams=true"
	testMigrateDSN = testDSN + "&multiStatements=true"

	cursorTestActorID   = 7
	cursorTestRowCount  = 220
	cursorTestTieCount  = 12
	cursorTestPageLimit = 25
	cursorTestMaxPages  = 20
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

func auditLogTableExists(t *testing.T, db *sql.DB) bool {
	t.Helper()

	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'audit_log'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("check audit_log existence: %v", err)
	}

	return count != 0
}

func TestAuditLogMigrations(t *testing.T) {
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

	migrateTo(t, m, 2)
	if !auditLogTableExists(t, db) {
		t.Error("expected audit_log to exist after migrating to the latest version")
	}

	migrateDownAll(t, m)
	if auditLogTableExists(t, db) {
		t.Error("expected audit_log to not exist after migrating all the way down")
	}
}

func TestAuditLogCursorPagination(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	m := newTestMigrate(t)
	migrateTo(t, m, 2)
	m.Close()
	t.Cleanup(func() {
		m2 := newTestMigrate(t)
		defer m2.Close()
		_ = m2.Down()
	})

	if _, err := db.Exec("DELETE FROM audit_log WHERE actor_id = ?", cursorTestActorID); err != nil {
		t.Fatalf("clear fixture rows: %v", err)
	}

	wantIDs := seedCursorFixture(t, db)

	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}

	repo := repository.NewAuditLogRepositoryDB(gormDB)
	svc := service.NewAuditLogService(repo)
	hdlr := handler.NewAuditLogHandler(svc)

	app := fiber.New()
	app.Get("/audit-log", hdlr.ListByActor)

	gotIDs := map[int64]bool{}
	cursor := ""
	terminated := false

	for page := 0; page < cursorTestMaxPages; page++ {
		url := fmt.Sprintf("/audit-log?actor_id=%d&limit=%d", cursorTestActorID, cursorTestPageLimit)
		if cursor != "" {
			url += "&cursor=" + cursor
		}

		req := httptest.NewRequest(fiber.MethodGet, url, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("page %d: app.Test() error = %v", page, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("page %d: status = %d, want %d", page, resp.StatusCode, fiber.StatusOK)
		}

		var envelope struct {
			Data service.ListAuditLogResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("page %d: decode response: %v", page, err)
		}

		for _, item := range envelope.Data.Items {
			if gotIDs[item.ID] {
				t.Fatalf("page %d: duplicate id %d", page, item.ID)
			}
			gotIDs[item.ID] = true
		}

		if envelope.Data.NextCursor == nil {
			terminated = true
			break
		}
		cursor = *envelope.Data.NextCursor
	}

	if !terminated {
		t.Fatalf("cursor walk did not terminate within %d pages", cursorTestMaxPages)
	}

	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("collected %d ids, want %d", len(gotIDs), len(wantIDs))
	}
	for _, id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("missing id %d from cursor walk", id)
		}
	}
}

func seedCursorFixture(t *testing.T, db *sql.DB) []int64 {
	t.Helper()

	tieAt := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	ids := make([]int64, 0, cursorTestRowCount)

	for i := 0; i < cursorTestRowCount; i++ {
		createdAt := tieAt.Add(-time.Duration(i) * time.Minute)
		if i < cursorTestTieCount {
			createdAt = tieAt
		}

		result, err := db.Exec(
			`INSERT INTO audit_log (actor_id, action, entity_type, metadata, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			cursorTestActorID, "update", "order", "{}", createdAt,
		)
		if err != nil {
			t.Fatalf("insert fixture row %d: %v", i, err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("last insert id for fixture row %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	return ids
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
	repo.EXPECT().ListByActor(gomock.Any(), gomock.Any()).Return([]repository.AuditLog{created}, nil)

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
		Data service.ListAuditLogResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(envelope.Data.Items))
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
