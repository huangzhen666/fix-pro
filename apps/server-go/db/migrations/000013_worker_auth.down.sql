DROP TABLE IF EXISTS worker_session;
DROP INDEX IF EXISTS idx_employee_worker_mobile;
ALTER TABLE employee_account
    DROP COLUMN IF EXISTS must_change_password,
    DROP COLUMN IF EXISTS last_login_at,
    DROP COLUMN IF EXISTS last_password_changed_at,
    DROP COLUMN IF EXISTS password_version;
DELETE FROM admin_role_permission WHERE permission_id IN (SELECT id FROM admin_permission WHERE permission_code = 'worker.reset_password');
DELETE FROM admin_permission WHERE permission_code = 'worker.reset_password';
