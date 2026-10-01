WITH rolled AS (
    SELECT
        o.id AS order_id,
        o.org_id,
        o.status AS previous_status,
        CASE
            WHEN BOOL_AND(w.status = 'CANCELLED') THEN 'CANCELLED'
            WHEN BOOL_OR(w.status IN ('PENDING_DISPATCH', 'PENDING_ACCEPT', 'PENDING_ARRIVAL', 'ARRIVED', 'IN_SERVICE', 'WAITING_COMPLETION_REVIEW', 'REWORK_REQUIRED', 'WAITING_CUSTOMER_SERVICE_CONFIRMATION', 'SECOND_VISIT_PENDING')) THEN 'FULFILLING'
            WHEN BOOL_AND(w.status IN ('FINISHED', 'FINISHED_WITH_REVIEW_EXCEPTION') OR COALESCE(w.customer_acceptance_status, '') IN ('MANUAL_ACCEPTED', 'AUTO_ACCEPTED')) THEN 'COMPLETED'
            WHEN BOOL_OR(w.status = 'WAITING_ACCEPTANCE') THEN 'WAITING_ACCEPTANCE'
            ELSE 'FULFILLING'
        END AS next_status
    FROM customer_order o
    JOIN work_order w ON w.org_id = o.org_id AND w.order_id = o.id
    WHERE o.status IN ('FULFILLING', 'WAITING_ACCEPTANCE', 'COMPLETED')
    GROUP BY o.id
), changed AS (
    UPDATE customer_order o
    SET status = rolled.next_status,
        version = version + 1,
        updated_at = CURRENT_TIMESTAMP(3),
        completed_at = CASE WHEN rolled.next_status = 'COMPLETED' THEN COALESCE(completed_at, CURRENT_TIMESTAMP(3)) ELSE completed_at END
    FROM rolled
    WHERE o.org_id = rolled.org_id
      AND o.id = rolled.order_id
      AND o.status <> rolled.next_status
    RETURNING o.org_id, o.id, rolled.previous_status, rolled.next_status
)
INSERT INTO order_status_history(org_id, order_id, from_status, to_status, event_code, operator_type, operator_id, operator_name)
SELECT org_id, id, previous_status, next_status, 'ORDER_ROLLUP_BACKFILL', 'SYSTEM', 0, 'system'
FROM changed;
