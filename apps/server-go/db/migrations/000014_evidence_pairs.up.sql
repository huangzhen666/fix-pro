-- 每个工单服务单位保留一组施工前/施工后凭证，确保照片数量与客户购买数量一一对应。
ALTER TABLE work_order_evidence
    ADD COLUMN IF NOT EXISTS work_order_item_id BIGINT NULL CHECK (work_order_item_id IS NULL OR work_order_item_id >= 0),
    ADD COLUMN IF NOT EXISTS unit_no INTEGER NULL CHECK (unit_no IS NULL OR unit_no > 0);

-- 将历史凭证按上传顺序映射到各服务单位；无法匹配的旧凭证仍保留，但不会作为新的成对凭证使用。
WITH ranked_evidence AS (
    SELECT e.id,
           e.org_id,
           e.work_order_id,
           e.stage,
           ROW_NUMBER() OVER (PARTITION BY e.org_id, e.work_order_id, e.stage ORDER BY e.created_at, e.id) AS sequence_no
    FROM work_order_evidence e
    WHERE e.work_order_item_id IS NULL
      AND e.stage IN ('BEFORE', 'AFTER')
), evidence_slots AS (
    SELECT wi.org_id,
           wi.work_order_id,
           wi.id AS work_order_item_id,
           unit.unit_no,
           ROW_NUMBER() OVER (PARTITION BY wi.org_id, wi.work_order_id ORDER BY wi.id, unit.unit_no) AS sequence_no
    FROM work_order_item wi
    CROSS JOIN LATERAL generate_series(1, wi.quantity) AS unit(unit_no)
)
UPDATE work_order_evidence e
SET work_order_item_id = slot.work_order_item_id,
    unit_no = slot.unit_no
FROM ranked_evidence ranked
JOIN evidence_slots slot
  ON slot.org_id = ranked.org_id
 AND slot.work_order_id = ranked.work_order_id
 AND slot.sequence_no = ranked.sequence_no
WHERE e.id = ranked.id;

CREATE UNIQUE INDEX IF NOT EXISTS uk_work_order_evidence_pair_stage
    ON work_order_evidence (org_id, work_order_id, work_order_item_id, unit_no, stage)
    WHERE work_order_item_id IS NOT NULL AND unit_no IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_work_order_evidence_pair
    ON work_order_evidence (org_id, work_order_id, work_order_item_id, unit_no, stage, created_at);
