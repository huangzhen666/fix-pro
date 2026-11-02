// Explicit, idempotent draft initialization. Never run by server startup.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"os"
	"strings"
)

func main() {
	demo := flag.Bool("demo", false, "fill test parameters; only databases ending in _test")
	flag.Parse()
	if e := run(*demo); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(demo bool) error {
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx := context.Background()
	db, e := database.Open(ctx, c)
	if e != nil {
		return e
	}
	defer db.Close()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var name string
	if e = tx.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); e != nil {
		return e
	}
	if demo && !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("demo values are restricted to disposable _test databases")
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(130020)`); e != nil {
		return e
	}
	for i, sample := range catalog.Examples() {
		p := sample.Profile
		price := sample.Price
		if !demo {
			t := catalog.Templates()
			idx := []int{1, 2, 0, 3}[i]
			p = t[idx].Profile
			price = 0
		}
		b := make([]byte, 12)
		if _, e = rand.Read(b); e != nil {
			return e
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO service_sku(org_id,category_id,sku_code,name,description,detail_profile,price_mode,base_price,unit,status,created_by,updated_by,seed_key) SELECT 1,c.id,$1,$2,'', $3,'FIXED',$4,$5,'DRAFT','seed-service-skus','seed-service-skus',$6 FROM service_category c WHERE c.org_id=1 AND c.status='ACTIVE' ORDER BY c.sort_order,c.id LIMIT 1 ON CONFLICT(seed_key) DO NOTHING`, "SVC-"+strings.ToUpper(hex.EncodeToString(b)), sample.Name, p, price, sample.Unit, sample.Key)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		fmt.Printf("%s: created %d draft(s)\n", sample.Key, n)
	}
	return tx.Commit()
}
