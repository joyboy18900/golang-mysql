package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
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

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run . <migrate|serve> [args]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "migrate":
		runMigrate(os.Args[2:])
	case "serve":
		runServe()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
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

func mustOpenDB(dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Errorf("open mysql: %w", err))
	}

	return db
}

func openGormDB() *gorm.DB {
	db, err := gorm.Open(gormmysql.Open(mysqlDSN()), &gorm.Config{})
	if err != nil {
		panic(fmt.Errorf("open gorm mysql: %w", err))
	}

	return db
}

func openAuditLogRepo() (repository.AuditLogRepository, *gorm.DB) {
	db := openGormDB()
	return repository.NewAuditLogRepositoryDB(db), db
}

func newMigrate() *migrate.Migrate {
	db := mustOpenDB(mysqlDSN() + "&multiStatements=true")

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		panic(fmt.Errorf("new migrate mysql driver: %w", err))
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "mysql", driver)
	if err != nil {
		panic(fmt.Errorf("new migrate: %w", err))
	}

	return m
}

func runMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|goto N>")
		os.Exit(1)
	}

	m := newMigrate()
	defer m.Close()

	var err error
	switch args[0] {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "goto":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: migrate goto N")
			os.Exit(1)
		}
		var version int
		version, err = strconv.Atoi(args[1])
		if err == nil {
			err = m.Migrate(uint(version))
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown migrate subcommand %q\n", args[0])
		os.Exit(1)
	}

	if err != nil && err != migrate.ErrNoChange {
		panic(fmt.Errorf("migrate %s: %w", args[0], err))
	}

	logs.Info("migrate " + args[0] + " complete")
}

func runServe() {
	auditLogRepo, gormDB := openAuditLogRepo()
	sqlDB, err := gormDB.DB()
	if err != nil {
		panic(fmt.Errorf("get underlying sql.DB: %w", err))
	}
	defer sqlDB.Close()

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
