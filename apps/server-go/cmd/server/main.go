package main

import (
	"context"
	"github.com/fixpro/server/internal/app"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c, e := config.Load()
	if e != nil {
		log.Error("config", "error", e)
		os.Exit(1)
	}
	db, e := database.Open(context.Background(), c)
	if e != nil {
		log.Error("database", "error", e)
		os.Exit(1)
	}
	defer db.Close()
	h, e := app.New(c, db, log)
	if e != nil {
		log.Error("application", "error", e)
		os.Exit(1)
	}
	srv := &http.Server{Addr: c.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		log.Info("server started", "addr", c.HTTPAddr)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Error("server", "error", e)
			os.Exit(1)
		}
	}()
	<-done
	ctx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
