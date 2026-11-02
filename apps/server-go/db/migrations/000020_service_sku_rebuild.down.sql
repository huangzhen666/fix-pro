-- Structure only: deleted prelaunch SKU data cannot be restored.
BEGIN;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM service_sku) OR EXISTS(SELECT 1 FROM order_item) THEN RAISE EXCEPTION 'Cannot downgrade populated structured SKU model'; END IF; END $$;
DROP TABLE sku_snapshot_media;
ALTER TABLE service_sku DROP COLUMN detail_profile, DROP COLUMN seed_key, ADD COLUMN service_scope varchar(1000) NOT NULL, ADD COLUMN exclusions varchar(1000) NOT NULL, ADD COLUMN warranty_description varchar(500) NOT NULL;
ALTER TABLE order_item DROP COLUMN detail_profile_snapshot, ADD COLUMN service_scope_snapshot varchar(1000) NOT NULL, ADD COLUMN exclusions_snapshot varchar(1000) NOT NULL, ADD COLUMN warranty_snapshot varchar(500) NOT NULL;
ALTER TABLE shopping_cart_item RENAME COLUMN demand_description TO fault_description;
ALTER TABLE order_item RENAME COLUMN demand_description TO fault_description;
ALTER TABLE shopping_cart_item DROP CONSTRAINT shopping_cart_item_quantity_check;
ALTER TABLE shopping_cart_item ADD CONSTRAINT shopping_cart_item_quantity_check CHECK(quantity BETWEEN 1 AND 99);
COMMIT;
