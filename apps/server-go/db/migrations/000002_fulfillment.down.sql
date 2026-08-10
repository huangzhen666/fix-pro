DROP TABLE IF EXISTS work_order_evidence;
DROP TABLE IF EXISTS work_order_assignment_history;
DROP TABLE IF EXISTS work_order_item;

ALTER TABLE work_order_status_history
    DROP COLUMN IF EXISTS operator_type,
    DROP COLUMN IF EXISTS operator_name;
ALTER TABLE work_order
    DROP COLUMN IF EXISTS accepted_at,
    DROP COLUMN IF EXISTS arrived_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS completion_submitted_at,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS finished_at,
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS completion_summary,
    DROP COLUMN IF EXISTS review_note,
    DROP COLUMN IF EXISTS exception_code,
    DROP COLUMN IF EXISTS cancel_reason;

ALTER TABLE employee_account DROP CONSTRAINT IF EXISTS ck_employee_role;
ALTER TABLE employee_account DROP COLUMN IF EXISTS role, DROP COLUMN IF EXISTS mobile;

ALTER TABLE idempotency_record DROP CONSTRAINT IF EXISTS uk_idempotency_principal_key;
ALTER TABLE idempotency_record DROP COLUMN IF EXISTS principal_type;
ALTER TABLE idempotency_record ADD CONSTRAINT uk_idempotency_principal_key UNIQUE (org_id, principal_id, idempotency_key);

DROP TABLE IF EXISTS order_status_history;
ALTER TABLE customer_order
    DROP COLUMN IF EXISTS confirmed_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS cancel_reason;
