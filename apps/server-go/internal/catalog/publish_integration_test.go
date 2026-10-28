package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPublishAfterOffShelf(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable CATALOG_TEST_DSN required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	var mediaID, skuID int64
	if err = db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'ADMIN',1,'SKU_IMAGE','IMAGE','cover.png',$1,'image/png',1,'READY') RETURNING id`, stamp).Scan(&mediaID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM media_asset WHERE id=$1`, mediaID) })
	if err = db.QueryRow(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,base_price,unit,cover_media_id,status) VALUES(1,1001,$1,'测试维修','维修检测服务范围覆盖十个字以上','不包含额外材料','提供售后质保服务','FIXED',11800,'次',$2,'DRAFT') RETURNING id`, stamp, mediaID).Scan(&skuID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM service_sku_version WHERE sku_id=$1`, skuID)
		db.Exec(`DELETE FROM service_sku WHERE id=$1`, skuID)
	})
	service := New(db)
	assertPublished := func(wantVersion int, wantPrice int64) PublishResult {
		t.Helper()
		result, err := service.Publish(ctx, skuID, "test-admin")
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "PUBLISHED" || result.PublishedVersion != wantVersion {
			t.Fatalf("publish result=%+v, want PUBLISHED version %d", result, wantVersion)
		}
		detail, err := service.Detail(ctx, skuID)
		if err != nil || detail.Status != "PUBLISHED" {
			t.Fatalf("persisted status=%s, error=%v", detail.Status, err)
		}
		public, err := service.PublicDetail(ctx, skuID)
		if err != nil || public.PublishedVersion != wantVersion || public.Price != wantPrice {
			t.Fatalf("public detail=%+v, error=%v", public, err)
		}
		var versions int
		if err = db.QueryRow(`SELECT COUNT(*) FROM service_sku_version WHERE sku_id=$1`, skuID).Scan(&versions); err != nil || versions != wantVersion {
			t.Fatalf("snapshot count=%d, want %d, error=%v", versions, wantVersion, err)
		}
		return result
	}
	assertOffShelf := func() {
		t.Helper()
		if err = service.OffShelf(ctx, skuID); err != nil {
			t.Fatal(err)
		}
		if _, err = service.PublicDetail(ctx, skuID); err == nil {
			t.Fatal("off-shelf SKU remains publicly available")
		}
	}
	first := assertPublished(1, 11800)
	assertOffShelf()
	republished := assertPublished(1, 11800)
	if !first.PublishedAt.Round(time.Millisecond).Equal(republished.PublishedAt) {
		t.Fatal("unchanged snapshot publication time changed")
	}
	assertPublished(1, 11800)
	assertOffShelf()
	assertPublished(1, 11800)
	assertOffShelf()
	if _, err = db.Exec(`UPDATE service_sku SET base_price=12800 WHERE id=$1`, skuID); err != nil {
		t.Fatal(err)
	}
	assertPublished(2, 12800)
}
