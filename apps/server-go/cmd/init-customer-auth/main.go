package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/fixpro/server/internal/customer"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"os"
	"time"
)

func main() { os.Exit(run()) }
func run() int {
	l := logging.Bootstrap("fix-pro-init-customer-auth")
	defer func() { _ = l.Sync() }()
	execute := flag.Bool("execute", false, "clear customer test data once; stop application writers first")
	retry := flag.Bool("retry-media", false, "retry only media removals from committed manifest")
	flag.Parse()
	c, e := config.Load()
	if e != nil {
		l.Error("initialization failed", logging.SafeError(e)...)
		return 1
	}
	c.Logging.Service = "fix-pro-init-customer-auth"
	configured, err := logging.New(c.Logging, nil)
	if err != nil {
		l.Error("invalid log configuration")
		return 1
	}
	l = configured
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, e := database.Open(ctx, c)
	if e != nil {
		l.Error("initialization failed", logging.SafeError(e)...)
		return 1
	}
	defer db.Close()
	out, e := customer.Initialize(ctx, db, c.MediaLocalRoot, *execute, *retry)
	// Raw object keys stay in the existing protected DB cleanup manifest.
	out.Objects = nil
	if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
		l.Error("report write failed")
		return 1
	}
	l.Info("initialization result", zap.String("module", "maintenance"), zap.String("event", "maintenance.initialization_completed"), zap.Bool("result", out.Completed))
	if e != nil {
		l.Error("initialization failed", logging.SafeError(e)...)
		return 1
	}
	return 0
}
