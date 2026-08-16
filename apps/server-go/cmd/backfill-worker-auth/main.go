// Command backfill-worker-auth converts historical Worker placeholder
// passwords to Argon2id hashes of w+手机号. It only touches placeholder
// accounts, so running it repeatedly cannot overwrite a password already
// changed by a real user.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	c, err := config.Load()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(context.Background(), c)
	if err != nil {
		log.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `SELECT id,org_id,mobile,password_hash FROM employee_account WHERE role='WORKER' AND deleted_at IS NULL AND mobile ~ '^1[0-9]{10}$' AND password_hash LIKE '!local-worker%'`)
	if err != nil {
		log.Error("query workers", "error", err)
		os.Exit(1)
	}
	defer rows.Close()
	type worker struct {
		id, orgID int64
		mobile    string
		oldHash   string
	}
	workers := make([]worker, 0)
	for rows.Next() {
		var item worker
		if err := rows.Scan(&item.id, &item.orgID, &item.mobile, &item.oldHash); err != nil {
			log.Error("scan worker", "error", err)
			os.Exit(1)
		}
		workers = append(workers, item)
	}
	if err := rows.Err(); err != nil {
		log.Error("read workers", "error", err)
		os.Exit(1)
	}
	for _, item := range workers {
		hash, err := auth.HashPassword("w" + item.mobile)
		if err != nil {
			log.Error("hash worker password", "workerId", item.id, "error", err)
			os.Exit(1)
		}
		result, err := db.ExecContext(ctx, `UPDATE employee_account SET username=mobile,password_hash=$1,must_change_password=TRUE,password_version=password_version+1 WHERE id=$2 AND org_id=$3 AND password_hash=$4`, hash, item.id, item.orgID, item.oldHash)
		if err != nil {
			log.Error("update worker", "workerId", item.id, "error", err)
			os.Exit(1)
		}
		if count, _ := result.RowsAffected(); count > 0 {
			log.Info("worker password migrated", "workerId", item.id)
		}
	}
}
