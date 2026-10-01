DROP INDEX IF EXISTS idx_work_order_evidence_pair;
DROP INDEX IF EXISTS uk_work_order_evidence_pair_stage;

ALTER TABLE work_order_evidence
    DROP COLUMN IF EXISTS unit_no,
    DROP COLUMN IF EXISTS work_order_item_id;
