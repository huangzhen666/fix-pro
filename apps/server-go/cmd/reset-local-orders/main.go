// reset-local-orders removes only local transactional test data. Never run on startup.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	execute := flag.Bool("execute", false, "delete local order and fulfillment test data")
	manifest := flag.String("manifest", "", "path to cleanup manifest (required)")
	retry := flag.Bool("retry-media", false, "retry files listed in a committed manifest")
	flag.Parse()
	c, err := config.Load()
	if err != nil {
		return err
	}
	u, err := url.Parse(c.DBDSN)
	if err != nil {
		return err
	}
	if c.Env != "local" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || strings.TrimPrefix(u.Path, "/") != "fix_pro" {
		return fmt.Errorf("only local loopback fix_pro database is allowed")
	}
	if *manifest == "" {
		return fmt.Errorf("-manifest is required")
	}
	type Report struct {
		Database  string           `json:"database"`
		Committed bool             `json:"committed"`
		Counts    map[string]int64 `json:"deletedCounts"`
		Preserved map[string]int64 `json:"preservedCounts"`
		Objects   []string         `json:"objects"`
	}
	report := Report{Counts: map[string]int64{}, Preserved: map[string]int64{}, Objects: []string{}}
	if *retry {
		b, e := os.ReadFile(*manifest)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &report); e != nil {
			return e
		}
		if !report.Committed {
			return fmt.Errorf("manifest is not committed")
		}
		return removeFiles(c.MediaLocalRoot, report.Objects)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, c)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `LOCK TABLE customer_order,work_order,media_asset IN ACCESS EXCLUSIVE MODE`); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&report.Database); err != nil {
		return err
	}
	preserved := []string{"organization", "customer", "customer_address", "admin_user", "admin_role", "employee_account", "service_category", "service_sku", "service_sku_version", "shopping_cart", "shopping_cart_item"}
	for _, table := range preserved {
		var count int64
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		report.Preserved[table] = count
	}
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE reset_media ON COMMIT DROP AS SELECT DISTINCT m.id,m.object_key FROM media_asset m WHERE m.id IN (SELECT media_id FROM order_item_media UNION SELECT media_id FROM work_order_evidence) OR (m.owner_type='WORK_ORDER' AND m.owner_id IN(SELECT id FROM work_order)); UPDATE work_order SET customer_service_confirmation_id=NULL; DELETE FROM outbox_event WHERE (upper(aggregate_type) IN('ORDER','CUSTOMER_ORDER') AND aggregate_id IN(SELECT id FROM customer_order)) OR (upper(aggregate_type)='WORK_ORDER' AND aggregate_id IN(SELECT id FROM work_order)); DELETE FROM idempotency_record WHERE (response_body ? 'orderId') OR (response_body ? 'orderNo') OR (idempotency_key LIKE 'demand:%') OR (principal_type='WORKER' AND response_body='{"updated":true}'::jsonb AND principal_id IN(SELECT assignee_id FROM work_order));`); err != nil {
		return err
	}
	var hasHistory bool
	if err = tx.QueryRowContext(ctx, `SELECT to_regclass('order_demand_history_media') IS NOT NULL`).Scan(&hasHistory); err != nil {
		return err
	}
	if hasHistory {
		if _, err = tx.ExecContext(ctx, `INSERT INTO reset_media SELECT DISTINCT m.id,m.object_key FROM media_asset m JOIN order_demand_history_media hm ON hm.media_id=m.id WHERE NOT EXISTS(SELECT 1 FROM reset_media r WHERE r.id=m.id)`); err != nil {
			return err
		}
	}
	tables := []string{"order_demand_history_media", "order_demand_history", "payment_event", "payment_refund", "payment_order", "customer_rating", "customer_acceptance", "internal_review", "work_order_evidence", "work_order_event", "work_order_assignment_history", "work_order_status_history", "work_order_item", "customer_service_confirmation", "completion_submission", "work_order", "order_item_media", "order_item", "order_status_history", "customer_order"}
	for _, table := range tables {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		r, e := tx.ExecContext(ctx, "DELETE FROM "+table)
		if e != nil {
			return fmt.Errorf("%s: %w", table, e)
		}
		report.Counts[table], _ = r.RowsAffected()
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id,r.object_key FROM reset_media r WHERE NOT EXISTS(SELECT 1 FROM sku_snapshot_media WHERE media_id=r.id) AND NOT EXISTS(SELECT 1 FROM service_sku WHERE cover_media_id=r.id) AND NOT EXISTS(SELECT 1 FROM service_sku_media WHERE media_id=r.id) AND NOT EXISTS(SELECT 1 FROM shopping_cart_item_media WHERE media_id=r.id) AND NOT EXISTS(SELECT 1 FROM employee_account WHERE avatar_media_id=r.id) AND NOT EXISTS(SELECT 1 FROM worker_certificate_media WHERE media_id=r.id) AND NOT EXISTS(SELECT 1 FROM customer WHERE avatar_media_id=r.id)`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var key string
		if err = rows.Scan(&id, &key); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
		report.Objects = append(report.Objects, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `DELETE FROM media_asset WHERE id=$1`, id); err != nil {
			return err
		}
	}
	for _, table := range preserved {
		var count int64
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		if count != report.Preserved[table] {
			return fmt.Errorf("preserved table changed: %s", table)
		}
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(*manifest, b, 0600); err != nil {
		return err
	}
	if !*execute {
		fmt.Println(string(b))
		return nil
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	report.Committed = true
	b, _ = json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(*manifest, b, 0600); err != nil {
		return err
	}
	fmt.Println(string(b))
	return removeFiles(c.MediaLocalRoot, report.Objects)
}
func removeFiles(root string, objects []string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, key := range objects {
		p := filepath.Clean(filepath.Join(abs, filepath.FromSlash(key)))
		if !strings.HasPrefix(p, abs+string(os.PathSeparator)) {
			return fmt.Errorf("invalid object path")
		}
		if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("file cleanup incomplete; use -retry-media: %w", err)
		}
	}
	return nil
}
