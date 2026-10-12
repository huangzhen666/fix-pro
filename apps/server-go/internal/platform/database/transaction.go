package database

import (
	"context"
	"database/sql"
	"github.com/fixpro/server/internal/platform/logging"
)

// BeginTx makes diagnostic IDs available to history-column defaults, scoped to
// this transaction only. Nothing is set on a reused pooled connection globally.
func BeginTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	requestID, jobRunID := "", ""
	if s := logging.StateFrom(ctx); s != nil {
		requestID = s.ID /* job ID accessor uses locked state */
		jobRunID = logging.FieldString(ctx, "jobRunId")
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('fixpro.request_id',$1,true),set_config('fixpro.job_run_id',$2,true)`, requestID, jobRunID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}
