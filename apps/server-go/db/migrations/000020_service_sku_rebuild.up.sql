BEGIN;
LOCK TABLE service_sku, service_sku_version, service_sku_media, service_sku_skill_requirement, shopping_cart, shopping_cart_item, shopping_cart_item_media, customer_order, order_item IN ACCESS EXCLUSIVE MODE;
DO $$
DECLARE n bigint;
BEGIN
 IF EXISTS(SELECT 1 FROM customer_order) OR EXISTS(SELECT 1 FROM order_item) THEN
  RAISE EXCEPTION 'SKU rebuild requires empty orders; no data deleted';
 END IF;
 SELECT count(*) INTO n FROM service_sku;
 RAISE NOTICE 'SKU rebuild: deleting % existing SKUs and their direct references',n;
END $$;
DELETE FROM shopping_cart_item_media m USING shopping_cart_item c WHERE m.cart_item_id=c.id AND m.org_id=c.org_id AND c.sku_id IN(SELECT id FROM service_sku);
UPDATE shopping_cart SET version=version+1 WHERE id IN(SELECT cart_id FROM shopping_cart_item WHERE sku_id IN(SELECT id FROM service_sku));
DELETE FROM shopping_cart_item WHERE sku_id IN(SELECT id FROM service_sku);
DELETE FROM service_sku_skill_requirement;
DELETE FROM service_sku_media;
DELETE FROM service_sku_version;
DELETE FROM service_sku;
ALTER TABLE service_sku DROP COLUMN service_scope, DROP COLUMN exclusions, DROP COLUMN warranty_description,
 ADD COLUMN seed_key TEXT UNIQUE, ADD COLUMN detail_profile JSONB NOT NULL CHECK(jsonb_typeof(detail_profile)='object' AND detail_profile @> '{"schemaVersion":1}');
ALTER TABLE order_item DROP COLUMN service_scope_snapshot, DROP COLUMN exclusions_snapshot, DROP COLUMN warranty_snapshot,
 ADD COLUMN detail_profile_snapshot JSONB NOT NULL CHECK(jsonb_typeof(detail_profile_snapshot)='object' AND detail_profile_snapshot @> '{"schemaVersion":1}');
ALTER TABLE shopping_cart_item RENAME COLUMN fault_description TO demand_description;
ALTER TABLE order_item RENAME COLUMN fault_description TO demand_description;
ALTER TABLE shopping_cart_item DROP CONSTRAINT shopping_cart_item_quantity_check;
ALTER TABLE shopping_cart_item ADD CONSTRAINT shopping_cart_item_quantity_check CHECK(quantity BETWEEN 1 AND 9999);
CREATE TABLE sku_snapshot_media (
 org_id bigint NOT NULL,
 sku_id bigint NOT NULL,
 version_no integer NOT NULL,
 media_id bigint NOT NULL REFERENCES media_asset(id),
 PRIMARY KEY(org_id,sku_id,version_no,media_id),
 FOREIGN KEY(org_id,sku_id,version_no) REFERENCES service_sku_version(org_id,sku_id,version_no)
);
UPDATE media_asset SET purpose='DEMAND_EVIDENCE' WHERE purpose='FAULT_EVIDENCE';
COMMIT;
