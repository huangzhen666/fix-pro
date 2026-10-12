-- Only use on disposable development databases; removes diagnostic associations.
ALTER TABLE work_order_event DROP COLUMN request_id, DROP COLUMN job_run_id;
ALTER TABLE work_order_assignment_history DROP COLUMN request_id, DROP COLUMN job_run_id;
ALTER TABLE work_order_status_history DROP COLUMN request_id, DROP COLUMN job_run_id;
ALTER TABLE order_status_history DROP COLUMN request_id, DROP COLUMN job_run_id;
ALTER TABLE worker_profile_history DROP COLUMN request_id, DROP COLUMN job_run_id;
