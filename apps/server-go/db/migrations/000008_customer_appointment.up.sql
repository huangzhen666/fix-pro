ALTER TABLE customer_order
    ADD COLUMN IF NOT EXISTS appointment_at TIMESTAMPTZ(3) NULL,
    ADD COLUMN IF NOT EXISTS appointment_slot VARCHAR(5) NULL;
ALTER TABLE work_order
    ADD COLUMN IF NOT EXISTS appointment_slot VARCHAR(5) NULL;
