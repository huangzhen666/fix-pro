DROP TABLE IF EXISTS work_order_event;
DROP TABLE IF EXISTS customer_service_confirmation;
DROP TABLE IF EXISTS internal_review;
DROP TABLE IF EXISTS customer_rating;
DROP TABLE IF EXISTS customer_acceptance;
DROP TABLE IF EXISTS completion_submission;
ALTER TABLE work_order DROP COLUMN IF EXISTS customer_service_confirmation_id, DROP COLUMN IF EXISTS auto_accept_due_at, DROP COLUMN IF EXISTS closed_at, DROP COLUMN IF EXISTS completion_submission_at, DROP COLUMN IF EXISTS completion_outcome, DROP COLUMN IF EXISTS closure_status, DROP COLUMN IF EXISTS internal_review_status, DROP COLUMN IF EXISTS customer_acceptance_at, DROP COLUMN IF EXISTS customer_acceptance_source, DROP COLUMN IF EXISTS customer_acceptance_status, DROP COLUMN IF EXISTS visit_status;
