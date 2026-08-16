DROP TRIGGER IF EXISTS trg_customer_address_updated_at ON customer_address;
DROP INDEX IF EXISTS uk_customer_address_default;
DROP INDEX IF EXISTS idx_customer_address_owner;
DROP TABLE IF EXISTS customer_address;
