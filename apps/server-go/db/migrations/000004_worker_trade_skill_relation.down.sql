ALTER TABLE worker_skill_assignment DROP CONSTRAINT IF EXISTS fk_worker_skill_assignment_skill_org;
ALTER TABLE worker_trade_assignment DROP CONSTRAINT IF EXISTS fk_worker_trade_assignment_trade_org;
ALTER TABLE worker_skill DROP CONSTRAINT IF EXISTS fk_worker_skill_trade_org;
ALTER TABLE worker_skill DROP CONSTRAINT IF EXISTS uk_worker_skill_org_id;
ALTER TABLE worker_trade DROP CONSTRAINT IF EXISTS uk_worker_trade_org_id;
