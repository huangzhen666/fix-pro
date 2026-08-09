package main

import (
	"errors"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"log/slog"
	"os"
	"path/filepath"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c, e := config.Load()
	if e != nil {
		log.Error("config", "error", e)
		os.Exit(1)
	}
	root := os.Getenv("MIGRATIONS_PATH")
	if root == "" {
		root = "db/migrations"
	}
	root, e = filepath.Abs(root)
	if e != nil {
		log.Error("migration path", "error", e)
		os.Exit(1)
	}
	m, e := migrate.New("file://"+filepath.ToSlash(root), c.DBDSN)
	if e != nil {
		log.Error("migration init", "error", e)
		os.Exit(1)
	}
	defer m.Close()
	if e = m.Up(); e != nil && !errors.Is(e, migrate.ErrNoChange) {
		log.Error("migration", "error", e)
		os.Exit(1)
	}
	log.Info("migrations applied")
}
