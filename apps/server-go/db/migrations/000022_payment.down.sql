-- Refuse rollback when transaction facts exist; never silently drop money records.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM payment_order) THEN RAISE EXCEPTION 'payment facts exist; rollback requires an audited recovery plan'; END IF;
END $$;
DELETE FROM admin_role_permission WHERE permission_id IN (SELECT id FROM admin_permission WHERE permission_code='payment.refund');
DELETE FROM admin_permission WHERE permission_code='payment.refund';
DROP TABLE payment_event, payment_refund, payment_order;
ALTER TABLE customer_order DROP COLUMN payment_required, DROP COLUMN refunded_amount, DROP COLUMN payment_expires_at;
