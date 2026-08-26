package main

import (
	"context"
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
)

const benchListLimit = 50

func main() {
	initConfig()

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run . <migrate|seed|bench|serve> [args]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "migrate":
		runMigrate(os.Args[2:])
	case "seed":
		runSeed(os.Args[2:])
	case "bench":
		runBench(os.Args[2:])
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

func initDB() *sql.DB {
	db, err := sql.Open("mysql", mysqlDSN())
	if err != nil {
		panic(fmt.Errorf("open mysql: %w", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("ping mysql: %w", err))
	}

	return db
}

func newMigrate() *migrate.Migrate {
	db, err := sql.Open("mysql", mysqlDSN()+"&multiStatements=true")
	if err != nil {
		panic(fmt.Errorf("open mysql for migrate: %w", err))
	}

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

func runSeed(args []string) {
	rowCount := viper.GetInt("seed.row_count")
	if len(args) >= 1 {
		var err error
		rowCount, err = strconv.Atoi(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "row count must be an integer")
			os.Exit(1)
		}
	}
	if rowCount <= 0 {
		fmt.Fprintln(os.Stderr, "usage: seed [rowCount] (or set seed.row_count in config.yaml)")
		os.Exit(1)
	}

	db := initDB()
	defer db.Close()

	repo := repository.NewAuditLogRepositoryDB(db)
	seedSvc := service.NewSeedService(repo, viper.GetInt("seed.actor_cardinality"), viper.GetInt("seed.batch_size"))

	inserted, err := seedSvc.Seed(context.Background(), rowCount)
	if err != nil {
		panic(fmt.Errorf("seed: %w", err))
	}

	logs.Info(fmt.Sprintf("seeded %d rows total", inserted))
}

func runBench(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: bench actorID")
		os.Exit(1)
	}
	actorID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "actorID must be an integer")
		os.Exit(1)
	}

	db := initDB()
	defer db.Close()

	repo := repository.NewAuditLogRepositoryDB(db)
	benchSvc := service.NewBenchService(repo)

	result, err := benchSvc.ListByActorPlan(context.Background(), actorID, benchListLimit)
	if err != nil {
		panic(fmt.Errorf("bench: %w", err))
	}

	fmt.Println(result.Summary)
	fmt.Println(result.RawPlan)
}

func runServe() {
	db := initDB()
	defer db.Close()

	auditLogRepo := repository.NewAuditLogRepositoryDB(db)
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
