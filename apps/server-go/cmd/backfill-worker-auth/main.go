// Command backfill-worker-auth converts historical Worker placeholder
// passwords to Argon2id hashes of w+手机号. It only touches placeholder
// accounts, so running it repeatedly cannot overwrite a password already
// changed by a real user.
package main

import (
	"context"
	"fmt"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"os"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
)

func main() { os.Exit(run()) }
func run() int {
	log := logging.Bootstrap("fix-pro-backfill-worker-auth")
	defer func() { _ = log.Sync() }()
	c, err := config.Load()
	if err != nil {
		log.Error("config", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "config"))...)
		return 1
	}
	c.Logging.Service = "fix-pro-backfill-worker-auth"
	configured, logErr := logging.New(c.Logging, nil)
	if logErr != nil {
		log.Error("invalid log configuration", append(logging.SafeError(logErr), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "invalid log configuration"))...)
		return 1
	}
	log = configured
	db, err := database.Open(context.Background(), c)
	if err != nil {
		log.Error("database", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "database"))...)
		return 1
	}
	defer db.Close()
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `SELECT id,org_id,mobile,password_hash FROM employee_account WHERE role='WORKER' AND deleted_at IS NULL AND mobile ~ '^1[0-9]{10}$' AND password_hash LIKE '!local-worker%'`)
	if err != nil {
		log.Error("query workers", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "query workers"))...)
		return 1
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
			log.Error("scan worker", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "scan worker"))...)
			return 1
		}
		workers = append(workers, item)
	}
	if err := rows.Err(); err != nil {
		log.Error("read workers", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "read workers"))...)
		return 1
	}
	for _, item := range workers {
		hash, err := auth.HashPassword("w" + item.mobile)
		if err != nil {
			log.Error("hash worker password", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "hash worker password"))...)
			return 1
		}
		result, err := db.ExecContext(ctx, `UPDATE employee_account SET username=mobile,password_hash=$1,must_change_password=TRUE,password_version=password_version+1 WHERE id=$2 AND org_id=$3 AND password_hash=$4`, hash, item.id, item.orgID, item.oldHash)
		if err != nil {
			log.Error("update worker", append(logging.SafeError(err), zap.String("module", "maintenance"), zap.String("event", "maintenance.failed"), zap.String("operation", "update worker"))...)
			return 1
		}
		if count, _ := result.RowsAffected(); count > 0 {
			log.Info("worker password migrated", zap.String("principalId", fmt.Sprint(item.id)))
		}
	}
	return 0
}
