BEGIN;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM service_sku) OR EXISTS(SELECT 1 FROM customer_order) OR EXISTS(SELECT 1 FROM order_item) THEN
  RAISE EXCEPTION 'Cannot undo simple SKU schema with populated data; restore a verified backup';
 END IF;
END $$;
ALTER TABLE service_sku DROP COLUMN service_description, DROP COLUMN service_notice,
 ADD COLUMN description VARCHAR(500), ADD COLUMN detail_profile JSONB NOT NULL,
 ALTER COLUMN category_id SET NOT NULL;
ALTER TABLE order_item DROP COLUMN service_description_snapshot, DROP COLUMN service_notice_snapshot, DROP COLUMN gallery_image_urls_snapshot,
 ADD COLUMN detail_profile_snapshot JSONB NOT NULL;
COMMIT;
