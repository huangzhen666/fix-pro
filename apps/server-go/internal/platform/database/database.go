package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/fixpro/server/internal/platform/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, c config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", c.DBDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(c.DBMaxOpen)
	db.SetMaxIdleConns(c.DBMaxIdle)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
