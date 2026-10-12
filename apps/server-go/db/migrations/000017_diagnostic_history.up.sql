-- Defaults consume transaction-local settings created by database.BeginTx.
-- Legacy rows stay NULL; no historical IDs are invented.
ALTER TABLE order_status_history
 ADD COLUMN request_id VARCHAR(64),
 ADD COLUMN job_run_id VARCHAR(64);
ALTER TABLE order_status_history
 ALTER COLUMN request_id SET DEFAULT NULLIF(current_setting('fixpro.request_id',true),''),
 ALTER COLUMN job_run_id SET DEFAULT NULLIF(current_setting('fixpro.job_run_id',true),'');
ALTER TABLE work_order_status_history
 ADD COLUMN request_id VARCHAR(64),
 ADD COLUMN job_run_id VARCHAR(64);
ALTER TABLE work_order_status_history
 ALTER COLUMN request_id SET DEFAULT NULLIF(current_setting('fixpro.request_id',true),''),
 ALTER COLUMN job_run_id SET DEFAULT NULLIF(current_setting('fixpro.job_run_id',true),'');
ALTER TABLE work_order_assignment_history
 ADD COLUMN request_id VARCHAR(64),
 ADD COLUMN job_run_id VARCHAR(64);
ALTER TABLE work_order_assignment_history
 ALTER COLUMN request_id SET DEFAULT NULLIF(current_setting('fixpro.request_id',true),''),
 ALTER COLUMN job_run_id SET DEFAULT NULLIF(current_setting('fixpro.job_run_id',true),'');
ALTER TABLE work_order_event
 ADD COLUMN request_id VARCHAR(64),
 ADD COLUMN job_run_id VARCHAR(64);
ALTER TABLE work_order_event
 ALTER COLUMN request_id SET DEFAULT NULLIF(current_setting('fixpro.request_id',true),''),
 ALTER COLUMN job_run_id SET DEFAULT NULLIF(current_setting('fixpro.job_run_id',true),'');
CREATE INDEX idx_order_history_request ON order_status_history(org_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX idx_work_history_request ON work_order_status_history(org_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX idx_assignment_history_request ON work_order_assignment_history(org_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX idx_work_event_request ON work_order_event(org_id,request_id) WHERE request_id IS NOT NULL;
CREATE INDEX idx_work_event_job ON work_order_event(job_run_id) WHERE job_run_id IS NOT NULL;

ALTER TABLE worker_profile_history
 ADD COLUMN request_id VARCHAR(64),
 ADD COLUMN job_run_id VARCHAR(64);
ALTER TABLE worker_profile_history
 ALTER COLUMN request_id SET DEFAULT NULLIF(current_setting('fixpro.request_id',true),''),
 ALTER COLUMN job_run_id SET DEFAULT NULLIF(current_setting('fixpro.job_run_id',true),'');
CREATE INDEX idx_worker_history_request ON worker_profile_history(org_id,request_id) WHERE request_id IS NOT NULL;
