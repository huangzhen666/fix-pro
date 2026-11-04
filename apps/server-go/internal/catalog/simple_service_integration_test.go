package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fixpro/server/internal/platform/httpx"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSimpleDraftPublishValidationAndIsolation(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("CATALOG_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var database string
	if e = db.QueryRow(`SELECT current_database()`).Scan(&database); e != nil || !strings.HasSuffix(database, "_test") {
		t.Fatal("test DB required", e)
	}
	ctx := context.Background()
	s := New(db)
	x, e := s.Create(ctx, SKUWrite{}, "test")
	if e != nil {
		t.Fatal("empty draft", e)
	}
	n, _ := id(x.ID)
	var media int64
	t.Cleanup(func() {
		db.Exec(`DELETE FROM sku_snapshot_media WHERE sku_id=$1`, n)
		db.Exec(`DELETE FROM service_sku_media WHERE sku_id=$1`, n)
		db.Exec(`DELETE FROM service_sku_version WHERE sku_id=$1`, n)
		db.Exec(`DELETE FROM service_sku WHERE id=$1`, n)
		db.Exec(`DELETE FROM media_asset WHERE id=$1`, media)
	})
	if x.Name != "" || x.CategoryID != "" {
		t.Fatal("placeholder persisted", x)
	}
	if _, e = s.AdminList(ctx, "", "", 1, 20); e != nil {
		t.Fatal("draft list", e)
	}
	_, e = s.Publish(ctx, n, "test")
	var validation *httpx.Error
	if !errors.As(e, &validation) || validation.Code != "SKU_VALIDATION_FAILED" {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(validation.Details)
	var data struct{ Errors []FieldIssue }
	json.Unmarshal(raw, &data)
	if len(data.Errors) != 6 {
		t.Fatalf("expected all six missing fields, got %s", raw)
	}
	if e = db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'ADMIN',1,'SKU_IMAGE','IMAGE','test.png',$1,'image/png',1,'READY') RETURNING id`, fmt.Sprintf("simple-validation-%d", time.Now().UnixNano())).Scan(&media); e != nil {
		t.Fatal(e)
	}
	w := SKUWrite{CategoryID: "1001", Name: "修", Unit: "次", BasePrice: 1, ServiceDescription: "修", CoverMediaID: fmt.Sprint(media), Version: x.Version}
	x, e = s.Update(ctx, n, w, "test")
	if e != nil {
		t.Fatal(e)
	}
	old := x.Version - 1
	if _, e = s.publish(ctx, n, "test", &old); e == nil {
		t.Fatal("stale draft published")
	}
	if _, e = s.Publish(ctx, n, "test"); e != nil {
		t.Fatal("one image, short description, empty notice", e)
	}
	w.Version = x.Version
	w.ServiceDescription = "新草稿\n第二段"
	w.ServiceNotice = "<script>alert(1)</script>"
	if _, e = s.Update(ctx, n, w, "test"); e != nil {
		t.Fatal(e)
	}
	public, e := s.PublicDetail(ctx, n)
	if e != nil || public.ServiceDescription != "修" || public.ServiceNotice != "" {
		t.Fatal("draft leaked", public, e)
	}
	w.Version++
	w.Name = strings.Repeat("字", 61)
	w.ServiceDescription = strings.Repeat("字", 5001)
	w.ServiceNotice = strings.Repeat("字", 3001)
	if _, e = s.Update(ctx, n, w, "test"); e == nil {
		t.Fatal("limits ignored")
	}
}
