DROP INDEX IF EXISTS idx_work_order_worker_appointment_slot;
ALTER TABLE work_order DROP COLUMN IF EXISTS appointment_slot;
