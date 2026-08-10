ALTER TABLE worker_trade
    ADD CONSTRAINT uk_worker_trade_org_id UNIQUE (org_id, id);

ALTER TABLE worker_skill
    ADD CONSTRAINT uk_worker_skill_org_id UNIQUE (org_id, id);

ALTER TABLE worker_skill
    ADD CONSTRAINT fk_worker_skill_trade_org
    FOREIGN KEY (org_id, trade_id) REFERENCES worker_trade (org_id, id);

ALTER TABLE worker_trade_assignment
    ADD CONSTRAINT fk_worker_trade_assignment_trade_org
    FOREIGN KEY (org_id, trade_id) REFERENCES worker_trade (org_id, id);

ALTER TABLE worker_skill_assignment
    ADD CONSTRAINT fk_worker_skill_assignment_skill_org
    FOREIGN KEY (org_id, skill_id) REFERENCES worker_skill (org_id, id);
