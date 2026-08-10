DROP TRIGGER IF EXISTS trg_worker_skill_updated_at ON worker_skill;
DROP TRIGGER IF EXISTS trg_worker_trade_updated_at ON worker_trade;
DROP TABLE IF EXISTS worker_profile_history;
DROP TABLE IF EXISTS service_sku_skill_requirement;
DROP TABLE IF EXISTS worker_skill_assignment;
DROP TABLE IF EXISTS worker_trade_assignment;
DROP TABLE IF EXISTS worker_skill;
DROP TABLE IF EXISTS worker_trade;
DROP INDEX IF EXISTS idx_employee_worker_filters;
DROP INDEX IF EXISTS uk_employee_org_mobile;
DROP INDEX IF EXISTS uk_employee_org_worker_no;
ALTER TABLE employee_account
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS remark,
    DROP COLUMN IF EXISTS joined_on,
    DROP COLUMN IF EXISTS worker_no;
