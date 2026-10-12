DELETE FROM admin_role_permission WHERE permission_id IN (SELECT id FROM admin_permission WHERE permission_code='customer.view');
DELETE FROM admin_permission WHERE permission_code='customer.view';
DROP TABLE customer_initialization, customer_session, customer_wechat_identity;
ALTER TABLE customer DROP CONSTRAINT ck_customer_mobile_required, DROP CONSTRAINT uk_customer_org_id,
 DROP COLUMN status, DROP COLUMN last_login_at, DROP COLUMN last_activity_at, DROP COLUMN mobile_verified_at, DROP COLUMN avatar_media_id;
DROP INDEX uk_customer_mobile;
ALTER TABLE customer ALTER COLUMN mobile_cipher DROP NOT NULL, ALTER COLUMN mobile_hash DROP NOT NULL;
