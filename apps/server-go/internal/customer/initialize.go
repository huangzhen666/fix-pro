package customer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var cleanupTables = []string{"customer_session", "customer_wechat_identity", "customer_address", "shopping_cart_item_media", "shopping_cart_item", "shopping_cart", "customer_rating", "customer_acceptance", "internal_review", "work_order_evidence", "work_order_event", "work_order_assignment_history", "work_order_status_history", "work_order_item", "customer_service_confirmation", "completion_submission", "work_order", "order_item_media", "order_item", "order_status_history", "customer_order", "customer"}

type InitReport struct {
	Database  string           `json:"database"`
	Counts    map[string]int64 `json:"counts"`
	Objects   []string         `json:"objects"`
	Completed bool             `json:"completed"`
}

func Initialize(ctx context.Context, db *sql.DB, root string, execute, retry bool) (InitReport, error) {
	out := InitReport{Counts: map[string]int64{}, Objects: []string{}}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(111016)`); e != nil {
		return out, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&out.Database); e != nil {
		return out, e
	}
	var objects []byte
	e = tx.QueryRowContext(ctx, `SELECT media_objects FROM customer_initialization WHERE name='customer-auth-v1'`).Scan(&objects)
	if e == nil {
		out.Completed = true
		if json.Unmarshal(objects, &out.Objects) != nil {
			return out, errors.New("invalid cleanup manifest")
		}
		if execute && !retry {
			return out, errors.New("customer initialization already completed; only -retry-media is allowed")
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		if retry {
			return out, removeObjects(root, out.Objects)
		}
		return out, nil
	}
	if e != sql.ErrNoRows {
		return out, e
	}
	if retry {
		return out, errors.New("no completed initialization to retry")
	}
	for _, table := range cleanupTables {
		var n int64
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); e != nil {
			return out, e
		}
		out.Counts[table] = n
	}
	// Keep any object referenced by retained catalog or worker records, even if its owner is a customer.
	rows, e := tx.QueryContext(ctx, `SELECT object_key FROM media_asset m WHERE ((owner_type='CUSTOMER') OR (owner_type='WORK_ORDER' AND purpose='WORK_ORDER_EVIDENCE')) AND NOT EXISTS(SELECT 1 FROM service_sku WHERE cover_media_id=m.id) AND NOT EXISTS(SELECT 1 FROM service_sku_media WHERE media_id=m.id) AND NOT EXISTS(SELECT 1 FROM employee_account WHERE avatar_media_id=m.id) AND NOT EXISTS(SELECT 1 FROM worker_certificate_media WHERE media_id=m.id)`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			rows.Close()
			return out, e
		}
		out.Objects = append(out.Objects, key)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if !execute {
		return out, nil
	}
	// Application writers must be stopped; locks also prevent partial concurrent SQL writes.
	if _, e = tx.ExecContext(ctx, `LOCK TABLE customer,customer_order,work_order,media_asset IN ACCESS EXCLUSIVE MODE`); e != nil {
		return out, e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM outbox_event WHERE (upper(aggregate_type) IN ('CUSTOMER','CUSTOMER_ORDER','ORDER','WORK_ORDER') AND aggregate_id IN (SELECT id FROM customer UNION SELECT id FROM customer_order UNION SELECT id FROM work_order)); DELETE FROM audit_log WHERE upper(resource_type) IN ('CUSTOMER','ORDER','CUSTOMER_ORDER','WORK_ORDER') AND resource_id IN(SELECT id::text FROM customer UNION SELECT id::text FROM customer_order UNION SELECT id::text FROM work_order); DELETE FROM idempotency_record WHERE principal_type='CUSTOMER' OR (principal_type='WORKER' AND principal_id IN (SELECT assignee_id FROM work_order WHERE assignee_id IS NOT NULL)); UPDATE work_order SET customer_service_confirmation_id=NULL; UPDATE customer SET avatar_media_id=NULL WHERE mobile_cipher IS NOT NULL;`)
	if e != nil {
		return out, e
	}
	for _, table := range cleanupTables {
		if _, e = tx.ExecContext(ctx, "DELETE FROM "+table); e != nil {
			return out, e
		}
	}
	for _, key := range out.Objects {
		if _, e = tx.ExecContext(ctx, `DELETE FROM media_asset WHERE object_key=$1`, key); e != nil {
			return out, e
		}
	}
	_, e = tx.ExecContext(ctx, `ALTER TABLE customer VALIDATE CONSTRAINT ck_customer_mobile_required; ALTER TABLE customer ALTER COLUMN mobile_cipher SET NOT NULL,ALTER COLUMN mobile_hash SET NOT NULL;`)
	if e != nil {
		return out, e
	}
	objects, e = json.Marshal(out.Objects)
	if e != nil {
		return out, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO customer_initialization(name,media_objects) VALUES('customer-auth-v1',$1)`, string(objects))
	if e != nil {
		return out, e
	}
	if e = tx.Commit(); e != nil {
		return out, e
	}
	out.Completed = true
	return out, removeObjects(root, out.Objects)
}
func removeObjects(root string, objects []string) error {
	abs, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	for _, key := range objects {
		path := filepath.Clean(filepath.Join(abs, filepath.FromSlash(key)))
		if !strings.HasPrefix(path, abs+string(os.PathSeparator)) {
			return errors.New("invalid cleanup object path")
		}
		if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
			return errors.New("private media cleanup incomplete; use -retry-media")
		}
	}
	return nil
}
