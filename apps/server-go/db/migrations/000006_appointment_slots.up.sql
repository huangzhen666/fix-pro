ALTER TABLE work_order ADD COLUMN IF NOT EXISTS appointment_slot VARCHAR(5) NULL;
UPDATE work_order SET appointment_slot = to_char(appointment_at AT TIME ZONE 'Asia/Shanghai', 'HH24:MI') WHERE appointment_at IS NOT NULL AND appointment_slot IS NULL;
CREATE INDEX IF NOT EXISTS idx_work_order_worker_appointment_slot ON work_order(org_id, assignee_id, appointment_at, appointment_slot) WHERE assignee_id IS NOT NULL;
