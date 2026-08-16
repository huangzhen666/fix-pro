package fulfillment

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"strings"
	"time"
)

type ReviewLevelRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
	Version  int    `json:"version"`
}
type RatingRequest struct {
	Stars   int    `json:"stars"`
	Content string `json:"content"`
	Version int    `json:"version"`
}
type ServiceConfirmationRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
	Version  int    `json:"version"`
}
type TimelineEvent struct {
	Code         string    `json:"code"`
	OperatorType string    `json:"operatorType"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (s *Service) InternalReview(ctx context.Context, p auth.Principal, id int64, level string, req ReviewLevelRequest) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权审核完工", 403)
	}
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		return httpx.E("VALIDATION_ERROR", "审核决定不合法", 400)
	}
	if req.Decision == "REJECT" && strings.TrimSpace(req.Note) == "" {
		return httpx.E("REASON_REQUIRED", "驳回原因必填", 400)
	}
	if level != "QA" && level != "DIRECTOR" {
		return httpx.E("VALIDATION_ERROR", "审核级别不合法", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, reviewStatus, customerAcceptanceStatus, closureStatus, visitStatus, completionOutcome string
	var version int
	var orderID int64
	var submissionID int64
	if err = tx.QueryRowContext(ctx, `SELECT order_id,status,internal_review_status,customer_acceptance_status,closure_status,visit_status,COALESCE(completion_outcome,''),version FROM work_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, id).Scan(&orderID, &status, &reviewStatus, &customerAcceptanceStatus, &closureStatus, &visitStatus, &completionOutcome, &version); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if level == "QA" && reviewStatus != "PENDING_QA" {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前不在初审阶段", 409)
	}
	if level == "DIRECTOR" && reviewStatus != "PENDING_DIRECTOR" {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前不在总监审核阶段", 409)
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM completion_submission WHERE org_id=$1 AND work_order_id=$2 ORDER BY attempt_no DESC LIMIT 1`, p.OrgID, id).Scan(&submissionID); err == sql.ErrNoRows {
		return httpx.E("COMPLETION_SUBMISSION_NOT_FOUND", "未找到完工提交记录，请让师傅重新提交完工", 409)
	} else if err != nil {
		return err
	}
	nextReview := "APPROVED"
	nextStatus := status
	nextClosure := closureStatus
	nextVisit := visitStatus
	nextOutcome := completionOutcome
	finished := false
	event := "QA_APPROVED"
	if level == "QA" {
		nextReview = "PENDING_DIRECTOR"
		nextStatus = WorkOrderWaitingDirectorAudit
	}
	if level == "DIRECTOR" {
		event = "DIRECTOR_APPROVED"
		if customerAcceptanceStatus == "MANUAL_ACCEPTED" || customerAcceptanceStatus == "AUTO_ACCEPTED" {
			nextStatus = WorkOrderFinished
			nextClosure = "FINISHED"
			nextVisit = "FINISHED"
			nextOutcome = "NORMAL"
			finished = true
		} else {
			nextStatus = WorkOrderWaitingAcceptance
		}
	}
	if req.Decision == "REJECT" {
		nextReview = "REJECTED"
		nextStatus = WorkOrderWaitingCustomerService
		nextClosure = "WAITING_CUSTOMER_SERVICE_CONFIRMATION"
		event = level + "_REJECTED"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO internal_review(org_id,work_order_id,submission_id,level,decision,reviewer_id,note) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''))`, p.OrgID, id, submissionID, level, req.Decision, p.SubjectID, strings.TrimSpace(req.Note)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,internal_review_status=$2,closure_status=$3,visit_status=$4,completion_outcome=NULLIF($5,''),review_note=NULLIF($6,''),reviewed_at=CURRENT_TIMESTAMP(3),closed_at=CASE WHEN $7::boolean THEN CURRENT_TIMESTAMP(3) ELSE closed_at END,finished_at=CASE WHEN $7::boolean THEN CURRENT_TIMESTAMP(3) ELSE finished_at END,version=version+1 WHERE org_id=$8 AND id=$9 AND version=$10`, nextStatus, nextReview, nextClosure, nextVisit, nextOutcome, strings.TrimSpace(req.Note), finished, p.OrgID, id, req.Version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,'ADMIN',$6,$7,NULLIF($8,''))`, p.OrgID, id, status, nextStatus, event, p.SubjectID, p.Name, strings.TrimSpace(req.Note)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_event(org_id,work_order_id,event_code,operator_type,operator_id,note) VALUES($1,$2,$3,'ADMIN',$4,NULLIF($5,''))`, p.OrgID, id, event, p.SubjectID, strings.TrimSpace(req.Note)); err != nil {
		return err
	}
	if finished {
		var statuses []string
		rows, queryErr := tx.QueryContext(ctx, `SELECT status FROM work_order WHERE org_id=$1 AND order_id=$2`, p.OrgID, orderID)
		if queryErr != nil {
			return queryErr
		}
		for rows.Next() {
			var value string
			if queryErr = rows.Scan(&value); queryErr != nil {
				rows.Close()
				return queryErr
			}
			statuses = append(statuses, value)
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return queryErr
		}
		rows.Close()
		nextOrderStatus := rollupOrder(statuses)
		var previousOrderStatus string
		if queryErr = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&previousOrderStatus); queryErr != nil {
			return queryErr
		}
		if _, queryErr = tx.ExecContext(ctx, `UPDATE customer_order SET status=$1,version=version+1,completed_at=CASE WHEN $4::boolean THEN CURRENT_TIMESTAMP(3) ELSE completed_at END WHERE org_id=$2 AND id=$3`, nextOrderStatus, p.OrgID, orderID, nextOrderStatus == OrderCompleted); queryErr != nil {
			return queryErr
		}
		if previousOrderStatus != nextOrderStatus {
			if _, queryErr = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$4,'ORDER_ROLLED_UP','SYSTEM',0,'system')`, p.OrgID, orderID, previousOrderStatus, nextOrderStatus); queryErr != nil {
				return queryErr
			}
		}
	}
	return tx.Commit()
}

func (s *Service) SubmitRating(ctx context.Context, p auth.Principal, id int64, req RatingRequest) error {
	if p.Role != "CUSTOMER" {
		return httpx.E("FORBIDDEN", "无权评价工单", 403)
	}
	if req.Stars < 1 || req.Stars > 5 {
		return httpx.E("VALIDATION_ERROR", "评分需为1-5星", 400)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO customer_rating(org_id,work_order_id,submission_id,customer_id,stars,content) SELECT $1,$2,id,$3,$4,NULLIF($5,'') FROM completion_submission WHERE org_id=$1 AND work_order_id=$2 ORDER BY attempt_no DESC LIMIT 1`, p.OrgID, id, p.SubjectID, req.Stars, strings.TrimSpace(req.Content))
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) CustomerServiceConfirmation(ctx context.Context, p auth.Principal, id int64, req ServiceConfirmationRequest) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权处理客服确认", 403)
	}
	if req.Decision != "SECOND_VISIT_REQUIRED" && req.Decision != "NO_SECOND_VISIT" {
		return httpx.E("VALIDATION_ERROR", "客服决定不合法", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT status,version FROM work_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, id).Scan(&status, &version); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderWaitingCustomerService {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前不在客服确认阶段", 409)
	}
	nextStatus := WorkOrderFinishedReviewException
	closure := "FINISHED_WITH_REVIEW_EXCEPTION"
	outcome := "CUSTOMER_CONFIRMED_NO_SECOND_VISIT"
	event := "CUSTOMER_SERVICE_NO_SECOND_VISIT"
	if req.Decision == "SECOND_VISIT_REQUIRED" {
		nextStatus = WorkOrderSecondVisitPending
		closure = "SECOND_VISIT_PENDING"
		outcome = "SECOND_VISIT_COMPLETED"
		event = "CUSTOMER_SERVICE_SECOND_VISIT_REQUIRED"
	}
	var confirmationID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO customer_service_confirmation(org_id,work_order_id,decision,note,operator_id) VALUES($1,$2,$3,NULLIF($4,''),$5) RETURNING id`, p.OrgID, id, req.Decision, strings.TrimSpace(req.Note), p.SubjectID).Scan(&confirmationID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,closure_status=$2,completion_outcome=$3,customer_service_confirmation_id=$4,closed_at=CASE WHEN $2 LIKE 'FINISHED%' THEN CURRENT_TIMESTAMP(3) ELSE closed_at END,finished_at=CASE WHEN $2 LIKE 'FINISHED%' THEN CURRENT_TIMESTAMP(3) ELSE finished_at END,version=version+1 WHERE org_id=$5 AND id=$6 AND version=$7`, nextStatus, closure, outcome, confirmationID, p.OrgID, id, req.Version); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO work_order_event(org_id,work_order_id,event_code,operator_type,operator_id,note) VALUES($1,$2,$3,'ADMIN',$4,NULLIF($5,''))`, p.OrgID, id, event, p.SubjectID, strings.TrimSpace(req.Note))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) WorkOrderTimeline(ctx context.Context, p auth.Principal, id int64) ([]TimelineEvent, error) {
	if p.Role != "ADMIN" && p.Role != "CUSTOMER" && p.Role != "WORKER" {
		return nil, httpx.E("FORBIDDEN", "无权查看流程", 403)
	}
	if p.Role == "CUSTOMER" {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id WHERE w.org_id=$1 AND w.id=$2 AND o.customer_id=$3`, p.OrgID, id, p.SubjectID).Scan(&exists); err == sql.ErrNoRows {
			return nil, httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
		} else if err != nil {
			return nil, err
		}
	}
	if p.Role == "WORKER" {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3`, p.OrgID, id, p.SubjectID).Scan(&exists); err == sql.ErrNoRows {
			return nil, httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
		} else if err != nil {
			return nil, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_code,operator_type,COALESCE(reason,''),created_at FROM work_order_status_history WHERE org_id=$1 AND work_order_id=$2 AND event_code NOT IN ('ASSIGNED','REASSIGNED','RESCHEDULED','WORKER_RESCHEDULE_REQUESTED','COMPLETION_SUBMITTED','CUSTOMER_ACCEPTED','CUSTOMER_REJECTED','QA_APPROVED','QA_REJECTED','DIRECTOR_APPROVED','DIRECTOR_REJECTED') UNION ALL SELECT event_code,operator_type,COALESCE(note,''),created_at FROM work_order_event WHERE org_id=$1 AND work_order_id=$2 AND event_code='COMPLETION_SUBMITTED' UNION ALL SELECT h.event_code,h.operator_type,COALESCE(NULLIF(CONCAT_WS('；',CASE WHEN h.to_assignee_id IS NOT NULL THEN '派给：'||COALESCE(e.display_name,'师傅#'||h.to_assignee_id::text) END,CASE WHEN h.to_appointment_at IS NOT NULL THEN '预约：'||to_char(h.to_appointment_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD HH24:MI') END,NULLIF(h.reason,'')),''),'') AS note,h.created_at FROM work_order_assignment_history h LEFT JOIN employee_account e ON e.org_id=h.org_id AND e.id=h.to_assignee_id WHERE h.org_id=$1 AND h.work_order_id=$2 UNION ALL SELECT 'CUSTOMER_ACCEPTANCE_'||decision,'CUSTOMER',COALESCE(reason,''),created_at FROM customer_acceptance WHERE org_id=$1 AND work_order_id=$2 UNION ALL SELECT 'CUSTOMER_RATING','CUSTOMER',('评分：'||stars::text||' 星 '||COALESCE(content,'')),created_at FROM customer_rating WHERE org_id=$1 AND work_order_id=$2 UNION ALL SELECT 'INTERNAL_'||level||'_'||decision,'ADMIN',COALESCE(note,''),created_at FROM internal_review WHERE org_id=$1 AND work_order_id=$2 UNION ALL SELECT 'CUSTOMER_SERVICE_'||decision,'ADMIN',COALESCE(note,''),created_at FROM customer_service_confirmation WHERE org_id=$1 AND work_order_id=$2 ORDER BY created_at`, p.OrgID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimelineEvent{}
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.Code, &e.OperatorType, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AutoAcceptDue marks untouched customer acceptances as system accepted.
// It is safe to run repeatedly because customer_acceptance has a unique submission constraint.
func (s *Service) AutoAcceptDue(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT w.id,w.org_id,o.customer_id,w.order_id,w.status,w.internal_review_status,w.closure_status,w.visit_status,COALESCE(w.completion_outcome,''),w.version FROM work_order w JOIN completion_submission cs ON cs.org_id=w.org_id AND cs.work_order_id=w.id JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id WHERE w.customer_acceptance_status='PENDING' AND w.auto_accept_due_at IS NOT NULL AND w.auto_accept_due_at<=CURRENT_TIMESTAMP(3) AND w.closure_status='OPEN'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, orgID, customerID, orderID int64
		var version int
		var status, reviewStatus, closureStatus, visitStatus, completionOutcome string
		if err := rows.Scan(&id, &orgID, &customerID, &orderID, &status, &reviewStatus, &closureStatus, &visitStatus, &completionOutcome, &version); err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var submissionID int64
		if err = tx.QueryRowContext(ctx, `SELECT id FROM completion_submission WHERE org_id=$1 AND work_order_id=$2 ORDER BY attempt_no DESC LIMIT 1`, orgID, id).Scan(&submissionID); err != nil {
			tx.Rollback()
			continue
		}
		nextStatus, nextClosure, nextVisit, nextOutcome := status, closureStatus, visitStatus, completionOutcome
		finished := false
		if reviewStatus == "APPROVED" {
			nextStatus, nextClosure, nextVisit, nextOutcome, finished = WorkOrderFinished, "FINISHED", "FINISHED", "NORMAL", true
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,customer_acceptance_status='AUTO_ACCEPTED',customer_acceptance_source='AUTO',customer_acceptance_at=CURRENT_TIMESTAMP(3),closure_status=$2,visit_status=$3,completion_outcome=NULLIF($4,''),closed_at=CASE WHEN $5::boolean THEN CURRENT_TIMESTAMP(3) ELSE closed_at END,finished_at=CASE WHEN $5::boolean THEN CURRENT_TIMESTAMP(3) ELSE finished_at END,version=version+1 WHERE org_id=$6 AND id=$7 AND version=$8 AND customer_acceptance_status='PENDING'`, nextStatus, nextClosure, nextVisit, nextOutcome, finished, orgID, id, version); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO customer_acceptance(org_id,work_order_id,submission_id,customer_id,decision,source) VALUES($1,$2,$3,$4,'ACCEPT','AUTO') ON CONFLICT (org_id,submission_id) DO NOTHING`, orgID, id, submissionID, customerID); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_event(org_id,work_order_id,event_code,operator_type,operator_id,note) VALUES($1,$2,'CUSTOMER_AUTO_ACCEPTED','SYSTEM',0,'7-day timeout')`, orgID, id); err != nil {
			tx.Rollback()
			return err
		}
		if finished {
			var statuses []string
			statusRows, queryErr := tx.QueryContext(ctx, `SELECT status FROM work_order WHERE org_id=$1 AND order_id=$2`, orgID, orderID)
			if queryErr != nil {
				tx.Rollback()
				return queryErr
			}
			for statusRows.Next() {
				var value string
				if queryErr = statusRows.Scan(&value); queryErr != nil {
					statusRows.Close()
					tx.Rollback()
					return queryErr
				}
				statuses = append(statuses, value)
			}
			if queryErr = statusRows.Err(); queryErr != nil {
				statusRows.Close()
				tx.Rollback()
				return queryErr
			}
			statusRows.Close()
			nextOrderStatus := rollupOrder(statuses)
			var previousOrderStatus string
			if queryErr = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, orgID, orderID).Scan(&previousOrderStatus); queryErr != nil {
				tx.Rollback()
				return queryErr
			}
			if _, queryErr = tx.ExecContext(ctx, `UPDATE customer_order SET status=$1,version=version+1,completed_at=CASE WHEN $4::boolean THEN CURRENT_TIMESTAMP(3) ELSE completed_at END WHERE org_id=$2 AND id=$3`, nextOrderStatus, orgID, orderID, nextOrderStatus == OrderCompleted); queryErr != nil {
				tx.Rollback()
				return queryErr
			}
			if previousOrderStatus != nextOrderStatus {
				if _, queryErr = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$4,'ORDER_ROLLED_UP','SYSTEM',0,'system')`, orgID, orderID, previousOrderStatus, nextOrderStatus); queryErr != nil {
					tx.Rollback()
					return queryErr
				}
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Service) StringifyOutcome(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return fmt.Sprint(v.String)
}
