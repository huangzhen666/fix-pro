package main

import (
	"errors"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/logging"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"
	"os"
	"path/filepath"
)

func main() { os.Exit(run()) }
func run() int {
	log := logging.Bootstrap("fix-pro-migrate")
	defer func() { _ = log.Sync() }()
	c, e := config.Load()
	if e != nil {
		log.Error("config", append(logging.SafeError(e), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "config"))...)
		return 1
	}
	c.Logging.Service = "fix-pro-migrate"
	configured, logErr := logging.New(c.Logging, nil)
	if logErr != nil {
		log.Error("invalid log configuration", append(logging.SafeError(logErr), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "invalid log configuration"))...)
		return 1
	}
	log = configured
	root := os.Getenv("MIGRATIONS_PATH")
	if root == "" {
		root = "db/migrations"
	}
	root, e = filepath.Abs(root)
	if e != nil {
		log.Error("migration path", append(logging.SafeError(e), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "migration path"))...)
		return 1
	}
	m, e := migrate.New("file://"+filepath.ToSlash(root), c.DBDSN)
	if e != nil {
		log.Error("migration init", append(logging.SafeError(e), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "migration init"))...)
		return 1
	}
	defer m.Close()
	if e = m.Up(); e != nil && !errors.Is(e, migrate.ErrNoChange) {
		log.Error("migration", append(logging.SafeError(e), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "migration"))...)
		return 1
	}
	log.Info("migrations applied", zap.String("module", "maintenance"), zap.String("event", "maintenance.migrations_applied"))
	return 0
}
