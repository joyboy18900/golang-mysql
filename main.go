package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"golang-mysql/handler"
	"golang-mysql/logs"
	"golang-mysql/repository"
	"golang-mysql/service"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/viper"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	initConfig()
	runMigrations()

	gormDB := openGormDB()
	sqlDB, err := gormDB.DB()
	if err != nil {
		panic(fmt.Errorf("get underlying sql.DB: %w", err))
	}
	defer sqlDB.Close()

	auditLogRepo := repository.NewAuditLogRepositoryDB(gormDB)
	auditLogSvc := service.NewAuditLogService(auditLogRepo)
	auditLogHdlr := handler.NewAuditLogHandler(auditLogSvc)

	app := fiber.New()
	app.Post("/audit-log", auditLogHdlr.Create)
	app.Get("/audit-log", auditLogHdlr.ListByActor)

	port := viper.GetString("app.port")
	logs.Info("server started on port " + port)
	if err := app.Listen(":" + port); err != nil {
		logs.Error(err)
		os.Exit(1)
	}
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("read config: %w", err))
	}
}

func mysqlDSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&interpolateParams=true",
		viper.GetString("db.user"),
		viper.GetString("db.password"),
		viper.GetString("db.host"),
		viper.GetInt("db.port"),
		viper.GetString("db.name"),
	)
}

func openGormDB() *gorm.DB {
	db, err := gorm.Open(gormmysql.Open(mysqlDSN()), &gorm.Config{})
	if err != nil {
		panic(fmt.Errorf("open gorm mysql: %w", err))
	}

	return db
}

func runMigrations() {
	db, err := sql.Open("mysql", mysqlDSN()+"&multiStatements=true")
	if err != nil {
		panic(fmt.Errorf("open migrate mysql: %w", err))
	}
	defer db.Close()

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		panic(fmt.Errorf("new migrate mysql driver: %w", err))
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "mysql", driver)
	if err != nil {
		panic(fmt.Errorf("new migrate: %w", err))
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		panic(fmt.Errorf("migrate up: %w", err))
	}

	logs.Info("migrations up to date")
}
