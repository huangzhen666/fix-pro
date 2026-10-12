package main

import (
	"context"
	"github.com/fixpro/server/internal/app"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() { os.Exit(run()) }
func run() int {
	bootstrap := logging.Bootstrap("fix-pro-api")
	defer bootstrap.Sync()
	c, err := config.Load()
	if err != nil {
		bootstrap.Error("invalid server configuration", logging.SafeError(err)...)
		return 1
	}
	logger, err := logging.New(c.Logging, nil)
	if err != nil {
		bootstrap.Error("invalid log configuration")
		return 1
	}
	defer logger.Sync()
	parent, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ctx := logging.Context(parent, logger, c.Logging, "")
	db, err := database.Open(ctx, c)
	if err != nil {
		logger.Error("database startup failed", logging.SafeError(err)...)
		return 1
	}
	defer db.Close()
	h, err := app.NewWithContext(ctx, c, db, logger)
	if err != nil {
		logger.Error("application startup failed", logging.SafeError(err)...)
		return 1
	}
	srv := &http.Server{Addr: c.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second, ErrorLog: log.New(serverLog{logger}, "", 0)}
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()
	logger.Info("server started", zap.String("module", "server"), zap.String("event", "server.started"))
	select {
	case err = <-done:
		if err != http.ErrServerClosed {
			logger.Error("server failed", logging.SafeError(err)...)
			return 1
		}
	case <-parent.Done():
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), c.ShutdownTimeout)
	defer stop()
	if err = srv.Shutdown(shutdown); err != nil {
		logger.Error("shutdown failed", logging.SafeError(err)...)
		return 1
	}
	if lifecycle, ok := h.(interface{ Wait(context.Context) error }); ok {
		if err = lifecycle.Wait(shutdown); err != nil {
			logger.Error("jobs shutdown timed out", logging.SafeError(err)...)
			return 1
		}
	}
	logger.Info("server stopped", zap.String("module", "server"), zap.String("event", "server.stopped"))
	return 0
}

// net/http error text may include a panic value or URL. Retain a safe category.
type serverLog struct{ l *zap.Logger }

func (w serverLog) Write(p []byte) (int, error) {
	w.l.Error("HTTP server transport error", zap.String("module", "server"), zap.String("event", "server.transport.failed"))
	return len(p), nil
}
