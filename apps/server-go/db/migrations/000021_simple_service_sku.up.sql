BEGIN;
LOCK TABLE service_sku, service_sku_version, service_sku_media, service_sku_skill_requirement, sku_snapshot_media, shopping_cart, shopping_cart_item, shopping_cart_item_media, customer_order, order_item IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM customer_order) OR EXISTS(SELECT 1 FROM order_item) THEN
  RAISE EXCEPTION 'Simple SKU rebuild requires empty orders; no data deleted';
 END IF;
END $$;
DELETE FROM shopping_cart_item_media m USING shopping_cart_item c WHERE m.org_id=c.org_id AND m.cart_item_id=c.id AND c.sku_id IN(SELECT id FROM service_sku);
UPDATE shopping_cart SET version=version+1 WHERE id IN(SELECT cart_id FROM shopping_cart_item WHERE sku_id IN(SELECT id FROM service_sku));
DELETE FROM shopping_cart_item WHERE sku_id IN(SELECT id FROM service_sku);
DELETE FROM service_sku_skill_requirement;
DELETE FROM service_sku_media;
DELETE FROM sku_snapshot_media;
DELETE FROM service_sku_version;
DELETE FROM service_sku;
ALTER TABLE service_sku DROP COLUMN detail_profile, DROP COLUMN description,
 ALTER COLUMN category_id DROP NOT NULL,
 ADD COLUMN service_description TEXT NOT NULL DEFAULT '',
 ADD COLUMN service_notice TEXT NOT NULL DEFAULT '',
 ADD CONSTRAINT service_description_length CHECK(char_length(btrim(service_description))<=5000),
 ADD CONSTRAINT service_notice_length CHECK(char_length(btrim(service_notice))<=3000);
ALTER TABLE order_item DROP COLUMN detail_profile_snapshot,
 ADD COLUMN service_description_snapshot TEXT NOT NULL DEFAULT '',
 ADD COLUMN service_notice_snapshot TEXT NOT NULL DEFAULT '',
 ADD COLUMN gallery_image_urls_snapshot JSONB NOT NULL DEFAULT '[]';
COMMIT;
