package fulfillment

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fixpro/server/internal/media"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"mime/multipart"
)

type Service struct {
	db    *sql.DB
	media *media.Service
}

func New(db *sql.DB, ms *media.Service) *Service { return &Service{db: db, media: ms} }

type ConfirmRequest struct {
	Version  int    `json:"version"`
	Priority string `json:"priority"`
}

type OrderRejectRequest struct {
	Version int    `json:"version"`
	Reason  string `json:"reason"`
}

type WorkOrderResult struct {
	ID          string `json:"id"`
	WorkOrderNo string `json:"workOrderNo"`
	Status      string `json:"status"`
}

type ConfirmResult struct {
	OrderID     string            `json:"orderId"`
	OrderStatus string            `json:"orderStatus"`
	WorkOrders  []WorkOrderResult `json:"workOrders"`
}

type RejectResult struct {
	OrderID      string `json:"orderId"`
	OrderStatus  string `json:"orderStatus"`
	CancelReason string `json:"cancelReason"`
}

type Worker struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"displayName"`
	Mobile             string `json:"mobile"`
	Status             string `json:"status"`
	OpenWorkOrderCount int    `json:"openWorkOrderCount"`
}

type WorkOrderSummary struct {
	ID                       string     `json:"id"`
	WorkOrderNo              string     `json:"workOrderNo"`
	OrderID                  string     `json:"orderId"`
	OrderNo                  string     `json:"orderNo"`
	Status                   string     `json:"status"`
	CustomerAcceptanceStatus string     `json:"customerAcceptanceStatus"`
	Priority                 string     `json:"priority"`
	AssigneeID               string     `json:"assigneeId,omitempty"`
	AssigneeName             string     `json:"assigneeName,omitempty"`
	AppointmentAt            *time.Time `json:"appointmentAt,omitempty"`
	AppointmentSlot          string     `json:"appointmentSlot,omitempty"`
	AppointmentSlotLabel     string     `json:"appointmentSlotLabel,omitempty"`
	Version                  int        `json:"version"`
	CompletionOutcome        string     `json:"completionOutcome,omitempty"`
}

type WorkOrderPage struct {
	Items    []WorkOrderSummary `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

type AdminEvidence struct {
	ID              string    `json:"id"`
	MediaID         string    `json:"mediaId"`
	Stage           string    `json:"stage"`
	CustomerVisible bool      `json:"customerVisible"`
	MediaType       string    `json:"mediaType"`
	ContentType     string    `json:"contentType"`
	URL             string    `json:"url"`
	CreatedAt       time.Time `json:"createdAt"`
}

type AdminInternalReview struct {
	ID           string    `json:"id"`
	Level        string    `json:"level"`
	Decision     string    `json:"decision"`
	ReviewerID   string    `json:"reviewerId"`
	ReviewerName string    `json:"reviewerName"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AdminWorkOrderDetail struct {
	ID                       string                `json:"id"`
	OrderID                  string                `json:"orderId"`
	WorkOrderNo              string                `json:"workOrderNo"`
	OrderNo                  string                `json:"orderNo"`
	Status                   string                `json:"status"`
	Priority                 string                `json:"priority"`
	AssigneeID               string                `json:"assigneeId,omitempty"`
	AssigneeName             string                `json:"assigneeName,omitempty"`
	AppointmentAt            *time.Time            `json:"appointmentAt,omitempty"`
	AppointmentSlot          string                `json:"appointmentSlot,omitempty"`
	ServiceAddress           string                `json:"serviceAddress"`
	ContactName              string                `json:"contactName"`
	ContactMobile            string                `json:"contactMobile"`
	CompletionSummary        string                `json:"completionSummary,omitempty"`
	ReviewNote               string                `json:"reviewNote,omitempty"`
	Version                  int                   `json:"version"`
	AcceptedAt               *time.Time            `json:"acceptedAt,omitempty"`
	ArrivedAt                *time.Time            `json:"arrivedAt,omitempty"`
	StartedAt                *time.Time            `json:"startedAt,omitempty"`
	CompletionSubmittedAt    *time.Time            `json:"completionSubmittedAt,omitempty"`
	ReviewedAt               *time.Time            `json:"reviewedAt,omitempty"`
	FinishedAt               *time.Time            `json:"finishedAt,omitempty"`
	VisitStatus              string                `json:"visitStatus"`
	CustomerAcceptanceStatus string                `json:"customerAcceptanceStatus"`
	CustomerAcceptanceSource string                `json:"customerAcceptanceSource,omitempty"`
	CustomerAcceptanceAt     *time.Time            `json:"customerAcceptanceAt,omitempty"`
	InternalReviewStatus     string                `json:"internalReviewStatus"`
	ClosureStatus            string                `json:"closureStatus"`
	CompletionOutcome        string                `json:"completionOutcome,omitempty"`
	CompletionSubmissionAt   *time.Time            `json:"completionSubmissionAt,omitempty"`
	ClosedAt                 *time.Time            `json:"closedAt,omitempty"`
	Evidence                 []AdminEvidence       `json:"evidence"`
	InternalReviews          []AdminInternalReview `json:"internalReviews"`
}

func (s *Service) AdminWorkOrderDetail(ctx context.Context, p auth.Principal, id int64) (AdminWorkOrderDetail, error) {
	if p.Role != "ADMIN" {
		return AdminWorkOrderDetail{}, httpx.E("FORBIDDEN", "无权查看工单详情", 403)
	}
	var out AdminWorkOrderDetail
	var wid int64
	var aid sql.NullInt64
	var orderID int64
	if err := s.db.QueryRowContext(ctx, `SELECT w.id,w.order_id,w.work_order_no,o.order_no,w.status,w.priority,w.assignee_id,COALESCE(e.display_name,''),w.appointment_at,COALESCE(w.appointment_slot,''),o.service_address,o.contact_name,o.contact_mobile,COALESCE(w.completion_summary,''),COALESCE(w.review_note,''),w.version,w.accepted_at,w.arrived_at,w.started_at,w.completion_submitted_at,w.reviewed_at,w.finished_at,COALESCE(w.visit_status,''),COALESCE(w.customer_acceptance_status,''),COALESCE(w.customer_acceptance_source,''),w.customer_acceptance_at,COALESCE(w.internal_review_status,''),COALESCE(w.closure_status,''),COALESCE(w.completion_outcome,''),w.completion_submission_at,w.closed_at FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id LEFT JOIN employee_account e ON e.org_id=w.org_id AND e.id=w.assignee_id WHERE w.org_id=$1 AND w.id=$2`, p.OrgID, id).Scan(&wid, &orderID, &out.WorkOrderNo, &out.OrderNo, &out.Status, &out.Priority, &aid, &out.AssigneeName, &out.AppointmentAt, &out.AppointmentSlot, &out.ServiceAddress, &out.ContactName, &out.ContactMobile, &out.CompletionSummary, &out.ReviewNote, &out.Version, &out.AcceptedAt, &out.ArrivedAt, &out.StartedAt, &out.CompletionSubmittedAt, &out.ReviewedAt, &out.FinishedAt, &out.VisitStatus, &out.CustomerAcceptanceStatus, &out.CustomerAcceptanceSource, &out.CustomerAcceptanceAt, &out.InternalReviewStatus, &out.ClosureStatus, &out.CompletionOutcome, &out.CompletionSubmissionAt, &out.ClosedAt); err == sql.ErrNoRows {
		return out, httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return out, err
	}
	out.ID = fmt.Sprint(wid)
	out.OrderID = fmt.Sprint(orderID)
	if aid.Valid {
		out.AssigneeID = fmt.Sprint(aid.Int64)
	}
	out.Evidence = []AdminEvidence{}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.media_id,e.stage,e.customer_visible,m.media_type,m.content_type,e.created_at FROM work_order_evidence e JOIN media_asset m ON m.org_id=e.org_id AND m.id=e.media_id WHERE e.org_id=$1 AND e.work_order_id=$2 ORDER BY e.created_at,e.id`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var eid, mid int64
		var item AdminEvidence
		if err = rows.Scan(&eid, &mid, &item.Stage, &item.CustomerVisible, &item.MediaType, &item.ContentType, &item.CreatedAt); err != nil {
			return out, err
		}
		item.ID, item.MediaID = fmt.Sprint(eid), fmt.Sprint(mid)
		item.URL = "/api/v1/admin/media/" + item.MediaID + "/content"
		out.Evidence = append(out.Evidence, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	out.InternalReviews = []AdminInternalReview{}
	reviewRows, err := s.db.QueryContext(ctx, `SELECT r.id,r.level,r.decision,r.reviewer_id,COALESCE(e.display_name,''),COALESCE(r.note,''),r.created_at FROM internal_review r LEFT JOIN employee_account e ON e.org_id=r.org_id AND e.id=r.reviewer_id WHERE r.org_id=$1 AND r.work_order_id=$2 ORDER BY r.created_at,r.id`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	defer reviewRows.Close()
	for reviewRows.Next() {
		var rid, reviewerID int64
		var item AdminInternalReview
		if err = reviewRows.Scan(&rid, &item.Level, &item.Decision, &reviewerID, &item.ReviewerName, &item.Note, &item.CreatedAt); err != nil {
			return out, err
		}
		item.ID, item.ReviewerID = fmt.Sprint(rid), fmt.Sprint(reviewerID)
		out.InternalReviews = append(out.InternalReviews, item)
	}
	return out, reviewRows.Err()
}

type WorkerWorkOrderDetail struct {
	ID                   string             `json:"id"`
	WorkOrderNo          string             `json:"workOrderNo"`
	OrderNo              string             `json:"orderNo"`
	Status               string             `json:"status"`
	AssigneeName         string             `json:"assigneeName,omitempty"`
	AppointmentAt        *time.Time         `json:"appointmentAt,omitempty"`
	AppointmentSlot      string             `json:"appointmentSlot,omitempty"`
	AppointmentSlotLabel string             `json:"appointmentSlotLabel,omitempty"`
	CustomerName         string             `json:"customerName"`
	CustomerMobile       string             `json:"customerMobile"`
	ServiceAddress       string             `json:"serviceAddress"`
	Items                []WorkerOrderItem  `json:"items"`
	CompletionSummary    string             `json:"completionSummary,omitempty"`
	Version              int                `json:"version"`
	Evidence             []CustomerEvidence `json:"evidence"`
}

type WorkerOrderItem struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Unit          string             `json:"unit"`
	Quantity      int                `json:"quantity"`
	CustomerNote  string             `json:"customerNote,omitempty"`
	CustomerMedia []WorkerOrderMedia `json:"customerMedia"`
}

type WorkerOrderMedia struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	URL       string `json:"url"`
}

func (s *Service) WorkerWorkOrder(ctx context.Context, p auth.Principal, id int64) (WorkerWorkOrderDetail, error) {
	if p.Role != "WORKER" {
		return WorkerWorkOrderDetail{}, httpx.E("FORBIDDEN", "无权查看师傅工单", 403)
	}
	var out WorkerWorkOrderDetail
	var wid int64
	if err := s.db.QueryRowContext(ctx, `SELECT w.id,w.work_order_no,o.order_no,w.status,COALESCE(e.display_name,''),w.appointment_at,COALESCE(w.appointment_slot,''),o.contact_name,o.contact_mobile,o.service_address,COALESCE(w.completion_summary,''),w.version FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id LEFT JOIN employee_account e ON e.org_id=w.org_id AND e.id=w.assignee_id WHERE w.org_id=$1 AND w.id=$2 AND w.assignee_id=$3`, p.OrgID, id, p.SubjectID).Scan(&wid, &out.WorkOrderNo, &out.OrderNo, &out.Status, &out.AssigneeName, &out.AppointmentAt, &out.AppointmentSlot, &out.CustomerName, &out.CustomerMobile, &out.ServiceAddress, &out.CompletionSummary, &out.Version); err == sql.ErrNoRows {
		return out, httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	} else if err != nil {
		return out, err
	}
	out.AppointmentSlotLabel = appointmentSlotText(out.AppointmentSlot)
	out.ID = fmt.Sprint(wid)
	out.Items = []WorkerOrderItem{}
	itemRows, err := s.db.QueryContext(ctx, `SELECT oi.id,oi.sku_name_snapshot,oi.unit_snapshot,oi.quantity,COALESCE(oi.fault_description,'') FROM work_order_item wi JOIN order_item oi ON oi.org_id=wi.org_id AND oi.id=wi.order_item_id WHERE wi.org_id=$1 AND wi.work_order_id=$2 ORDER BY oi.id`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	for itemRows.Next() {
		var itemID int64
		var item WorkerOrderItem
		if err = itemRows.Scan(&itemID, &item.Name, &item.Unit, &item.Quantity, &item.CustomerNote); err != nil {
			itemRows.Close()
			return out, err
		}
		item.ID = fmt.Sprint(itemID)
		item.CustomerMedia = []WorkerOrderMedia{}
		mediaRows, mediaErr := s.db.QueryContext(ctx, `SELECT m.id,m.media_type FROM order_item_media om JOIN media_asset m ON m.org_id=om.org_id AND m.id=om.media_id AND m.status='READY' WHERE om.org_id=$1 AND om.order_item_id=$2 ORDER BY om.sort_order,om.id`, p.OrgID, itemID)
		if mediaErr != nil {
			itemRows.Close()
			return out, mediaErr
		}
		for mediaRows.Next() {
			var mediaID int64
			var mediaItem WorkerOrderMedia
			if err = mediaRows.Scan(&mediaID, &mediaItem.MediaType); err != nil {
				mediaRows.Close()
				itemRows.Close()
				return out, err
			}
			mediaItem.ID = fmt.Sprint(mediaID)
			mediaItem.URL = "/api/v1/worker/media/" + mediaItem.ID + "/content"
			item.CustomerMedia = append(item.CustomerMedia, mediaItem)
		}
		if err = mediaRows.Err(); err != nil {
			mediaRows.Close()
			itemRows.Close()
			return out, err
		}
		mediaRows.Close()
		out.Items = append(out.Items, item)
	}
	if err = itemRows.Err(); err != nil {
		itemRows.Close()
		return out, err
	}
	itemRows.Close()
	out.Evidence = []CustomerEvidence{}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.media_id,e.stage,e.created_at FROM work_order_evidence e WHERE e.org_id=$1 AND e.work_order_id=$2 ORDER BY e.created_at`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var eid, mid int64
		var ev CustomerEvidence
		if err = rows.Scan(&eid, &mid, &ev.Stage, &ev.CreatedAt); err != nil {
			return out, err
		}
		ev.ID = fmt.Sprint(eid)
		ev.MediaID = fmt.Sprint(mid)
		ev.URL = "/api/v1/worker/media/" + ev.MediaID + "/content"
		out.Evidence = append(out.Evidence, ev)
	}
	return out, rows.Err()
}

type AssignRequest struct {
	WorkerID        int64     `json:"workerId"`
	AppointmentAt   time.Time `json:"appointmentAt"`
	AppointmentSlot string    `json:"appointmentSlot"`
	Note            string    `json:"note"`
	Version         int       `json:"version"`
}
type ReassignRequest struct {
	WorkerID        int64     `json:"workerId"`
	AppointmentAt   time.Time `json:"appointmentAt"`
	AppointmentSlot string    `json:"appointmentSlot"`
	Reason          string    `json:"reason"`
	Version         int       `json:"version"`
}
type RescheduleRequest struct {
	AppointmentAt   time.Time `json:"appointmentAt"`
	AppointmentSlot string    `json:"appointmentSlot"`
	Version         int       `json:"version"`
}

func (s *Service) Workers(ctx context.Context, orgID int64, status string) ([]Worker, error) {
	if status == "" {
		status = "ACTIVE"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id,e.username,e.display_name,COALESCE(e.mobile,''),e.status,COUNT(w.id) FROM employee_account e LEFT JOIN work_order w ON w.org_id=e.org_id AND w.assignee_id=e.id AND w.status NOT IN ('FINISHED','CANCELLED') WHERE e.org_id=$1 AND e.role='WORKER' AND e.status=$2 GROUP BY e.id,e.username,e.display_name,e.mobile,e.status ORDER BY e.id`, orgID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Worker{}
	for rows.Next() {
		var id int64
		var x Worker
		if err = rows.Scan(&id, &x.Username, &x.DisplayName, &x.Mobile, &x.Status, &x.OpenWorkOrderCount); err != nil {
			return nil, err
		}
		x.ID = fmt.Sprint(id)
		if len(x.Mobile) > 7 {
			x.Mobile = x.Mobile[:3] + "****" + x.Mobile[len(x.Mobile)-4:]
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) CreateWorker(ctx context.Context, p auth.Principal, username, displayName, mobile string) (Worker, error) {
	if p.Role != "ADMIN" {
		return Worker{}, httpx.E("FORBIDDEN", "无权管理师傅", 403)
	}
	if strings.TrimSpace(displayName) == "" || !validWorkerMobileForFulfillment(mobile) {
		return Worker{}, httpx.E("VALIDATION_ERROR", "姓名或手机号不合法", 400)
	}
	initialPassword := "w" + strings.TrimSpace(mobile)
	hash, err := auth.HashPassword(initialPassword)
	if err != nil {
		return Worker{}, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES($1,$2,$3,$4,'ACTIVE','WORKER',$2,TRUE,1) RETURNING id`, p.OrgID, strings.TrimSpace(mobile), strings.TrimSpace(displayName), hash).Scan(&id)
	if err != nil {
		return Worker{}, httpx.E("WORKER_USERNAME_EXISTS", "师傅用户名已存在", 409)
	}
	return Worker{ID: fmt.Sprint(id), Username: strings.TrimSpace(mobile), DisplayName: strings.TrimSpace(displayName), Mobile: mobile, Status: "ACTIVE"}, nil
}

func validWorkerMobileForFulfillment(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 11 || value[0] != '1' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *Service) SetWorkerStatus(ctx context.Context, p auth.Principal, id int64, status string) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权管理师傅", 403)
	}
	if status != "ACTIVE" && status != "DISABLED" {
		return httpx.E("VALIDATION_ERROR", "师傅状态不合法", 400)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE employee_account SET status=$1,version=version+1 WHERE org_id=$2 AND id=$3 AND role='WORKER'`, status, p.OrgID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
	}
	return nil
}

func (s *Service) WorkOrders(ctx context.Context, orgID int64, status string, workerID int64, keyword string, outcome string, page, size int) (WorkOrderPage, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := "%" + strings.TrimSpace(keyword) + "%"
	where := `w.org_id=$1 AND ($2='' OR w.status=$2) AND ($3=0 OR w.assignee_id=$3) AND ($4='' OR COALESCE(w.completion_outcome,'')=$4) AND (w.work_order_no ILIKE $5 OR o.order_no ILIKE $5)`
	var out WorkOrderPage
	out.Page = page
	out.PageSize = size
	out.Items = []WorkOrderSummary{}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id WHERE `+where, orgID, status, workerID, outcome, q).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT w.id,w.work_order_no,w.order_id,o.order_no,w.status,COALESCE(w.customer_acceptance_status,''),w.priority,w.assignee_id,COALESCE(e.display_name,''),w.appointment_at,COALESCE(w.appointment_slot,''),w.version,COALESCE(w.completion_outcome,'') FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id LEFT JOIN employee_account e ON e.org_id=w.org_id AND e.id=w.assignee_id WHERE `+where+` ORDER BY w.created_at DESC LIMIT $6 OFFSET $7`, orgID, status, workerID, outcome, q, size, (page-1)*size)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, oid int64
		var x WorkOrderSummary
		var aid sql.NullInt64
		if err = rows.Scan(&id, &x.WorkOrderNo, &oid, &x.OrderNo, &x.Status, &x.CustomerAcceptanceStatus, &x.Priority, &aid, &x.AssigneeName, &x.AppointmentAt, &x.AppointmentSlot, &x.Version, &x.CompletionOutcome); err != nil {
			return out, err
		}
		x.ID = fmt.Sprint(id)
		x.OrderID = fmt.Sprint(oid)
		if aid.Valid {
			x.AssigneeID = fmt.Sprint(aid.Int64)
		}
		x.AppointmentSlotLabel = appointmentSlotText(x.AppointmentSlot)
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}

func validAppointmentSlot(slot string) bool {
	switch slot {
	case "08:00", "10:00", "12:00", "14:00", "16:00", "18:00", "20:00":
		return true
	}
	return false
}

func (s *Service) assign(ctx context.Context, p auth.Principal, id, workerID int64, appointment time.Time, slot, reason, event string, version int, reassign bool) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权派单", 403)
	}
	if appointment.Before(time.Now().UTC()) {
		return httpx.E("APPOINTMENT_REQUIRED", "预约时间必须晚于当前时间", 400)
	}
	if !validAppointmentSlot(slot) {
		return httpx.E("APPOINTMENT_SLOT_INVALID", "预约时间段必须为08:00至22:00每两小时一个时段", 400)
	}
	if reassign && strings.TrimSpace(reason) == "" {
		return httpx.E("REASON_REQUIRED", "改派原因必填", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldWorker sql.NullInt64
	var oldTime sql.NullTime
	var status string
	var oldVersion int
	if err = tx.QueryRowContext(ctx, `SELECT assignee_id,appointment_at,status,version FROM work_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, id).Scan(&oldWorker, &oldTime, &status, &oldVersion); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	}
	if err != nil {
		return err
	}
	if oldVersion != version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if reassign {
		if status != WorkOrderPendingAccept && status != WorkOrderPendingArrival && status != WorkOrderReworkRequired {
			return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能改派", 409)
		}
	} else if status != WorkOrderPendingDispatch {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能派单", 409)
	}
	var workerStatus string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM employee_account WHERE org_id=$1 AND id=$2 AND role='WORKER'`, p.OrgID, workerID).Scan(&workerStatus); err == sql.ErrNoRows {
		return httpx.E("WORKER_NOT_FOUND", "师傅不存在", 404)
	}
	if err != nil {
		return err
	}
	if workerStatus != "ACTIVE" {
		return httpx.E("WORKER_DISABLED", "师傅已禁用", 409)
	}
	var requiredSkills, matchedSkills int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT req.skill_id), COUNT(DISTINCT CASE WHEN wa.skill_id=req.skill_id THEN req.skill_id END)
        FROM work_order_item wi
        JOIN order_item oi ON oi.org_id=wi.org_id AND oi.id=wi.order_item_id
        LEFT JOIN service_sku_skill_requirement req ON req.org_id=oi.org_id AND req.sku_id=oi.sku_id
        LEFT JOIN worker_skill_assignment wa ON wa.org_id=wi.org_id AND wa.worker_id=$3 AND wa.skill_id=req.skill_id
        WHERE wi.org_id=$1 AND wi.work_order_id=$2`, p.OrgID, id, workerID).Scan(&requiredSkills, &matchedSkills); err != nil {
		return err
	}
	if requiredSkills > 0 && matchedSkills == 0 {
		return httpx.E("WORKER_SKILL_INVALID", "师傅不具备当前 SKU 所需技能", 409)
	}
	newStatus := WorkOrderPendingAccept
	var occupied int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM work_order WHERE org_id=$1 AND assignee_id=$2 AND appointment_at::date=$3::date AND appointment_slot=$4 AND status NOT IN ('FINISHED','CANCELLED') AND id<>$5 FOR UPDATE`, p.OrgID, workerID, appointment, slot, id).Scan(&occupied); err == nil {
		return httpx.E("APPOINTMENT_SLOT_OCCUPIED", "师傅该预约时间段已有工单", 409)
	} else if err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET assignee_id=$1,appointment_at=$2,appointment_slot=$3,status=$4,version=version+1 WHERE org_id=$5 AND id=$6 AND version=$7`, workerID, appointment, slot, newStatus, p.OrgID, id, version); err != nil {
		return err
	}
	code := event
	if code == "" {
		code = "ASSIGNED"
	}
	var oldID any
	if oldWorker.Valid {
		oldID = oldWorker.Int64
	}
	var prev any
	if oldTime.Valid {
		prev = oldTime.Time
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_assignment_history(org_id,work_order_id,from_assignee_id,to_assignee_id,from_appointment_at,to_appointment_at,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,$6,$7,'ADMIN',$8,$9,NULLIF($10,''))`, p.OrgID, id, oldID, workerID, prev, appointment, code, p.SubjectID, p.Name, reason); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,'ADMIN',$6,$7,NULLIF($8,''))`, p.OrgID, id, status, newStatus, code, p.SubjectID, p.Name, reason); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Assign(ctx context.Context, p auth.Principal, id int64, req AssignRequest) error {
	var at sql.NullTime
	var slot string
	if err := s.db.QueryRowContext(ctx, `SELECT appointment_at,COALESCE(appointment_slot,'') FROM work_order WHERE org_id=$1 AND id=$2`, p.OrgID, id).Scan(&at, &slot); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return err
	}
	if !at.Valid || !validAppointmentSlot(slot) {
		return httpx.E("APPOINTMENT_REQUIRED", "客户预约时间段未确定，不能派单", 409)
	}
	return s.assign(ctx, p, id, req.WorkerID, at.Time, slot, req.Note, "ASSIGNED", req.Version, false)
}
func (s *Service) Reassign(ctx context.Context, p auth.Principal, id int64, req ReassignRequest) error {
	return s.assign(ctx, p, id, req.WorkerID, req.AppointmentAt, req.AppointmentSlot, req.Reason, "REASSIGNED", req.Version, true)
}
func (s *Service) Reschedule(ctx context.Context, p auth.Principal, id int64, req RescheduleRequest) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权改期", 403)
	}
	if req.AppointmentAt.Before(time.Now().UTC()) {
		return httpx.E("APPOINTMENT_REQUIRED", "预约时间必须晚于当前时间", 400)
	}
	if !validAppointmentSlot(req.AppointmentSlot) {
		return httpx.E("APPOINTMENT_SLOT_INVALID", "预约时间段必须为08:00至22:00每两小时一个时段", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var worker sql.NullInt64
	var oldTime sql.NullTime
	var status string
	var v int
	if err = tx.QueryRowContext(ctx, `SELECT assignee_id,appointment_at,status,version FROM work_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, id).Scan(&worker, &oldTime, &status, &v); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	}
	if err != nil {
		return err
	}
	if v != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderPendingAccept && status != WorkOrderPendingArrival {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能改期", 409)
	}
	if worker.Valid {
		var occupied int
		if err = tx.QueryRowContext(ctx, `SELECT 1 FROM work_order WHERE org_id=$1 AND assignee_id=$2 AND appointment_at::date=$3::date AND appointment_slot=$4 AND status NOT IN ('FINISHED','CANCELLED') AND id<>$5 FOR UPDATE`, p.OrgID, worker.Int64, req.AppointmentAt, req.AppointmentSlot, id).Scan(&occupied); err == nil {
			return httpx.E("APPOINTMENT_SLOT_OCCUPIED", "师傅该预约时间段已有工单", 409)
		} else if err != sql.ErrNoRows {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET appointment_at=$1,appointment_slot=$2,version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5`, req.AppointmentAt, req.AppointmentSlot, p.OrgID, id, req.Version); err != nil {
		return err
	}
	var wid any
	if worker.Valid {
		wid = worker.Int64
	}
	var prev any
	if oldTime.Valid {
		prev = oldTime.Time
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_assignment_history(org_id,work_order_id,from_assignee_id,to_assignee_id,from_appointment_at,to_appointment_at,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$3,$4,$5,'RESCHEDULED','ADMIN',$6,$7)`, p.OrgID, id, wid, prev, req.AppointmentAt, p.SubjectID, p.Name); err != nil {
		return err
	}
	return tx.Commit()
}

type WorkerRescheduleRequest struct {
	AppointmentAt          time.Time `json:"appointmentAt"`
	AppointmentSlot        string    `json:"appointmentSlot"`
	Version                int       `json:"version"`
	CommunicationConfirmed bool      `json:"communicationConfirmed"`
}

func (s *Service) WorkerReschedule(ctx context.Context, p auth.Principal, id int64, req WorkerRescheduleRequest) error {
	if p.Role != "WORKER" {
		return httpx.E("FORBIDDEN", "无权发起改期", 403)
	}
	if !req.CommunicationConfirmed {
		return httpx.E("COMMUNICATION_CONFIRMATION_REQUIRED", "请确认已与客户沟通改期", 400)
	}
	if req.AppointmentAt.Before(time.Now().UTC()) || !validAppointmentSlot(req.AppointmentSlot) {
		return httpx.E("APPOINTMENT_SLOT_INVALID", "预约时间段不合法", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldTime sql.NullTime
	var oldSlot, status string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT appointment_at,COALESCE(appointment_slot,''),status,version FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3 FOR UPDATE`, p.OrgID, id, p.SubjectID).Scan(&oldTime, &oldSlot, &status, &version); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	} else if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderPendingAccept && status != WorkOrderPendingArrival {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能改期", 409)
	}
	var occupied int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM work_order WHERE org_id=$1 AND assignee_id=$2 AND appointment_at::date=$3::date AND appointment_slot=$4 AND status NOT IN ('FINISHED','CANCELLED') AND id<>$5 FOR UPDATE`, p.OrgID, p.SubjectID, req.AppointmentAt, req.AppointmentSlot, id).Scan(&occupied); err == nil {
		return httpx.E("APPOINTMENT_SLOT_OCCUPIED", "该时间段已有其他工单", 409)
	} else if err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET appointment_at=$1,appointment_slot=$2,version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5`, req.AppointmentAt, req.AppointmentSlot, p.OrgID, id, req.Version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_assignment_history(org_id,work_order_id,from_assignee_id,to_assignee_id,from_appointment_at,to_appointment_at,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$3,$4,$5,'WORKER_RESCHEDULE_REQUESTED','WORKER',$6,$7,'已与客户沟通')`, p.OrgID, id, p.SubjectID, oldTime, req.AppointmentAt, p.SubjectID, p.Name); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) WorkerReturn(ctx context.Context, p auth.Principal, id int64, version int, reason string) error {
	if p.Role != "WORKER" {
		return httpx.E("FORBIDDEN", "无权退回工单", 403)
	}
	if strings.TrimSpace(reason) == "" {
		return httpx.E("REASON_REQUIRED", "退回原因必填", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	var v int
	if err = tx.QueryRowContext(ctx, `SELECT status,version FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3 FOR UPDATE`, p.OrgID, id, p.SubjectID).Scan(&status, &v); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	} else if err != nil {
		return err
	}
	if v != version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderPendingAccept && status != WorkOrderPendingArrival {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能退回", 409)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,assignee_id=NULL,appointment_at=NULL,appointment_slot=NULL,version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`, WorkOrderPendingDispatch, p.OrgID, id, version); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,'WORKER_RETURNED','WORKER',$5,$6,$7)`, p.OrgID, id, status, WorkOrderPendingDispatch, p.SubjectID, p.Name, reason)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type RejectRequest struct {
	Reason  string `json:"reason"`
	Version int    `json:"version"`
}

func (s *Service) WorkerCommand(ctx context.Context, p auth.Principal, id int64, command string, req RejectRequest, key string) error {
	if p.Role != "WORKER" {
		return httpx.E("FORBIDDEN", "无权操作工单", 403)
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return httpx.E("VALIDATION_ERROR", "缺少有效 Idempotency-Key", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	body, _ := json.Marshal(struct {
		ID      int64
		Command string
		Request RejectRequest
	}{id, command, req})
	hash := sha256.Sum256(body)
	var idemID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'WORKER',$2,$3,$4,'PROCESSING',NULL,$5) ON CONFLICT (org_id,principal_type,principal_id,idempotency_key) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID, key, hash[:], time.Now().UTC().Add(24*time.Hour)).Scan(&idemID)
	if err == sql.ErrNoRows {
		var old, raw []byte
		if err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='WORKER' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&old, &raw); err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(old, hash[:]) != 1 {
			return httpx.E("IDEMPOTENCY_KEY_CONFLICT", "幂等键已用于不同请求", 409)
		}
		if len(raw) == 0 {
			return httpx.E("COMMAND_IN_PROGRESS", "命令正在处理", 409)
		}
		return nil
	}
	if err != nil {
		return err
	}
	var status string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT status,version FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3 FOR UPDATE`, p.OrgID, id, p.SubjectID).Scan(&status, &version); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	}
	if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	to, event := status, "WORK_ORDER_"+command
	switch command {
	case "ACCEPT":
		if status != WorkOrderPendingAccept {
			return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能接单", 409)
		}
		to = WorkOrderPendingArrival
	case "REJECT":
		if status != WorkOrderPendingAccept {
			return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能拒单", 409)
		}
		if strings.TrimSpace(req.Reason) == "" {
			return httpx.E("REASON_REQUIRED", "拒单原因必填", 400)
		}
		to = WorkOrderPendingDispatch
	case "ARRIVE":
		if status != WorkOrderPendingArrival {
			return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能标记到达", 409)
		}
		to = WorkOrderArrived
	case "START":
		if status != WorkOrderArrived {
			return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能开始服务", 409)
		}
		to = WorkOrderInService
	default:
		return httpx.E("VALIDATION_ERROR", "未知工单命令", 400)
	}
	var query string
	if command == "ACCEPT" {
		query = `UPDATE work_order SET status=$1,accepted_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`
	} else if command == "ARRIVE" {
		query = `UPDATE work_order SET status=$1,arrived_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`
	} else if command == "START" {
		query = `UPDATE work_order SET status=$1,started_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`
	} else {
		query = `UPDATE work_order SET status=$1,assignee_id=NULL,version=version+1 WHERE org_id=$2 AND id=$3 AND version=$4`
	}
	if _, err = tx.ExecContext(ctx, query, to, p.OrgID, id, req.Version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,'WORKER',$6,$7,NULLIF($8,''))`, p.OrgID, id, status, to, event, p.SubjectID, p.Name, req.Reason); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE idempotency_record SET response_code='OK',response_body=$1 WHERE id=$2`, []byte(`{"updated":true}`), idemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ConfirmOrder(ctx context.Context, p auth.Principal, key string, orderID int64, req ConfirmRequest) (ConfirmResult, error) {
	if p.Role != "ADMIN" {
		return ConfirmResult{}, httpx.E("FORBIDDEN", "无权确认订单", 403)
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return ConfirmResult{}, httpx.E("VALIDATION_ERROR", "缺少有效 Idempotency-Key", 400)
	}
	body, _ := json.Marshal(req)
	hash := sha256.Sum256(body)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ConfirmResult{}, err
	}
	defer tx.Rollback()
	var idemID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'ADMIN',$2,$3,$4,'PROCESSING',NULL,$5) ON CONFLICT (org_id,principal_type,principal_id,idempotency_key) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID, key, hash[:], time.Now().UTC().Add(24*time.Hour)).Scan(&idemID)
	if err == sql.ErrNoRows {
		var old []byte
		var raw []byte
		if err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='ADMIN' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&old, &raw); err != nil {
			return ConfirmResult{}, err
		}
		if len(old) != len(hash) || subtle.ConstantTimeCompare(old, hash[:]) != 1 {
			return ConfirmResult{}, httpx.E("ORDER_SUBMIT_DUPLICATED", "幂等键已用于不同请求", 409)
		}
		if len(raw) == 0 {
			return ConfirmResult{}, httpx.E("COMMAND_IN_PROGRESS", "命令正在处理", 409)
		}
		var out ConfirmResult
		if err = json.Unmarshal(raw, &out); err != nil {
			return ConfirmResult{}, err
		}
		return out, nil
	}
	if err != nil {
		return ConfirmResult{}, err
	}
	var status string
	var version int
	var appointment sql.NullTime
	var appointmentSlot string
	if err = tx.QueryRowContext(ctx, `SELECT status,version,appointment_at,COALESCE(appointment_slot,'') FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&status, &version, &appointment, &appointmentSlot); err == sql.ErrNoRows {
		return ConfirmResult{}, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	} else if err != nil {
		return ConfirmResult{}, err
	}
	if !canConfirm(status) {
		return ConfirmResult{}, httpx.E("ORDER_STATUS_CONFLICT", "订单当前状态不能确认", 409)
	}
	if version != req.Version {
		return ConfirmResult{}, httpx.E("RESOURCE_VERSION_CONFLICT", "订单已被其他操作修改", 409)
	}
	if !appointment.Valid || !validAppointmentSlot(appointmentSlot) {
		return ConfirmResult{}, httpx.E("APPOINTMENT_REQUIRED", "客户预约时间段未确定，不能生成工单", 409)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,quantity FROM order_item WHERE org_id=$1 AND order_id=$2 ORDER BY id FOR UPDATE`, p.OrgID, orderID)
	if err != nil {
		return ConfirmResult{}, err
	}
	type item struct {
		id       int64
		quantity int
	}
	items := []item{}
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.id, &x.quantity); err != nil {
			rows.Close()
			return ConfirmResult{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return ConfirmResult{}, err
	}
	if len(items) == 0 {
		return ConfirmResult{}, httpx.E("ORDER_NO_ITEMS", "订单没有订单项", 409)
	}
	out := ConfirmResult{OrderID: fmt.Sprint(orderID), OrderStatus: OrderFulfilling, WorkOrders: make([]WorkOrderResult, 0, len(items))}
	for _, item := range items {
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT work_order_id FROM work_order_item WHERE org_id=$1 AND order_item_id=$2`, p.OrgID, item.id).Scan(&existing)
		if err == nil {
			var no, st string
			if err = tx.QueryRowContext(ctx, `SELECT work_order_no,status FROM work_order WHERE org_id=$1 AND id=$2`, p.OrgID, existing).Scan(&no, &st); err != nil {
				return ConfirmResult{}, err
			}
			out.WorkOrders = append(out.WorkOrders, WorkOrderResult{ID: fmt.Sprint(existing), WorkOrderNo: no, Status: st})
			continue
		}
		if err != sql.ErrNoRows {
			return ConfirmResult{}, err
		}
		var workID int64
		var workNo string
		workNo = fmt.Sprintf("WO%s%06d", time.Now().UTC().Format("20060102150405"), item.id%1000000)
		if err = tx.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,status,priority,appointment_at,appointment_slot) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, p.OrgID, workNo, orderID, WorkOrderPendingDispatch, priority(req.Priority), appointment.Time, appointmentSlot).Scan(&workID); err != nil {
			return ConfirmResult{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_item(org_id,work_order_id,order_item_id,quantity) VALUES($1,$2,$3,$4)`, p.OrgID, workID, item.id, item.quantity); err != nil {
			return ConfirmResult{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_id,reason,operator_type,operator_name) VALUES($1,$2,NULL,$3,'WORK_ORDER_CREATED',$4,NULL,'ADMIN',$5)`, p.OrgID, workID, WorkOrderPendingDispatch, p.SubjectID, p.Name); err != nil {
			return ConfirmResult{}, err
		}
		out.WorkOrders = append(out.WorkOrders, WorkOrderResult{ID: fmt.Sprint(workID), WorkOrderNo: workNo, Status: WorkOrderPendingDispatch})
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `UPDATE customer_order SET status=$1,version=version+1,confirmed_at=$2 WHERE org_id=$3 AND id=$4 AND version=$5`, OrderFulfilling, now, p.OrgID, orderID, req.Version); err != nil {
		return ConfirmResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$4,'ORDER_CONFIRMED','ADMIN',$5,$6)`, p.OrgID, orderID, status, OrderFulfilling, p.SubjectID, p.Name); err != nil {
		return ConfirmResult{}, err
	}
	raw, _ := json.Marshal(out)
	if _, err = tx.ExecContext(ctx, `UPDATE idempotency_record SET response_code='OK',response_body=$1 WHERE id=$2`, raw, idemID); err != nil {
		return ConfirmResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return ConfirmResult{}, err
	}
	return out, nil
}

func (s *Service) RejectOrder(ctx context.Context, p auth.Principal, key string, orderID int64, req OrderRejectRequest) (RejectResult, error) {
	if p.Role != "ADMIN" {
		return RejectResult{}, httpx.E("FORBIDDEN", "无权打回订单", 403)
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return RejectResult{}, httpx.E("VALIDATION_ERROR", "缺少有效 Idempotency-Key", 400)
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return RejectResult{}, httpx.E("REASON_REQUIRED", "打回原因必填", 400)
	}
	if len([]rune(reason)) > 512 {
		return RejectResult{}, httpx.E("VALIDATION_ERROR", "打回原因不能超过512字", 400)
	}
	body, _ := json.Marshal(req)
	hash := sha256.Sum256(body)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return RejectResult{}, err
	}
	defer tx.Rollback()
	var idemID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'ADMIN',$2,$3,$4,'PROCESSING',NULL,$5) ON CONFLICT (org_id,principal_type,principal_id,idempotency_key) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID, key, hash[:], time.Now().UTC().Add(24*time.Hour)).Scan(&idemID)
	if err == sql.ErrNoRows {
		var old []byte
		var raw []byte
		if err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='ADMIN' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&old, &raw); err != nil {
			return RejectResult{}, err
		}
		if len(old) != len(hash) || subtle.ConstantTimeCompare(old, hash[:]) != 1 {
			return RejectResult{}, httpx.E("ORDER_SUBMIT_DUPLICATED", "幂等键已用于不同请求", 409)
		}
		if len(raw) == 0 {
			return RejectResult{}, httpx.E("COMMAND_IN_PROGRESS", "命令正在处理", 409)
		}
		var out RejectResult
		if err = json.Unmarshal(raw, &out); err != nil {
			return RejectResult{}, err
		}
		return out, nil
	}
	if err != nil {
		return RejectResult{}, err
	}
	var status string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT status,version FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&status, &version); err == sql.ErrNoRows {
		return RejectResult{}, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	} else if err != nil {
		return RejectResult{}, err
	}
	if status != OrderPendingConfirmation {
		return RejectResult{}, httpx.E("ORDER_STATUS_CONFLICT", "只有待确认订单可以打回", 409)
	}
	if version != req.Version {
		return RejectResult{}, httpx.E("RESOURCE_VERSION_CONFLICT", "订单已被其他操作修改", 409)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE customer_order SET status=$1,version=version+1,cancelled_at=CURRENT_TIMESTAMP(3),cancel_reason=$2 WHERE org_id=$3 AND id=$4 AND version=$5`, OrderCancelled, reason, p.OrgID, orderID, req.Version); err != nil {
		return RejectResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,'ORDER_REJECTED','ADMIN',$5,$6,$7)`, p.OrgID, orderID, status, OrderCancelled, p.SubjectID, p.Name, reason); err != nil {
		return RejectResult{}, err
	}
	out := RejectResult{OrderID: fmt.Sprint(orderID), OrderStatus: OrderCancelled, CancelReason: reason}
	raw, _ := json.Marshal(out)
	if _, err = tx.ExecContext(ctx, `UPDATE idempotency_record SET response_code='OK',response_body=$1 WHERE id=$2`, raw, idemID); err != nil {
		return RejectResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return RejectResult{}, err
	}
	return out, nil
}

func priority(value string) string {
	if value == "HIGH" || value == "URGENT" {
		return value
	}
	return "NORMAL"
}

type CompletionRequest struct {
	CompletionSummary string `json:"completionSummary"`
	Version           int    `json:"version"`
}
type EvidenceRequest struct {
	MediaID         int64  `json:"mediaId"`
	Stage           string `json:"stage"`
	CustomerVisible *bool  `json:"customerVisible"`
	Version         int    `json:"version"`
}
type ReviewRequest struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
	Version  int    `json:"version"`
}

type CustomerOrderSummary struct {
	ID                string    `json:"id"`
	OrderNo           string    `json:"orderNo"`
	Status            string    `json:"status"`
	StatusText        string    `json:"statusText"`
	CancelReason      string    `json:"cancelReason,omitempty"`
	TotalAmount       int64     `json:"totalAmount"`
	ItemCount         int       `json:"itemCount"`
	WorkOrderTotal    int       `json:"workOrderTotal"`
	WorkOrderFinished int       `json:"workOrderFinished"`
	CreatedAt         time.Time `json:"createdAt"`
}
type CustomerOrderPage struct {
	Items    []CustomerOrderSummary `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}
type CustomerWorkOrder struct {
	ID                       string             `json:"id"`
	WorkOrderNo              string             `json:"workOrderNo"`
	Status                   string             `json:"status"`
	StatusText               string             `json:"statusText"`
	CustomerAcceptanceStatus string             `json:"customerAcceptanceStatus"`
	AssigneeName             string             `json:"assigneeName,omitempty"`
	AppointmentAt            *time.Time         `json:"appointmentAt,omitempty"`
	AppointmentSlot          string             `json:"appointmentSlot,omitempty"`
	AppointmentSlotLabel     string             `json:"appointmentSlotLabel,omitempty"`
	CompletionSummary        string             `json:"completionSummary,omitempty"`
	Version                  int                `json:"version"`
	Evidence                 []CustomerEvidence `json:"evidence"`
}
type CustomerEvidence struct {
	ID        string    `json:"id"`
	MediaID   string    `json:"mediaId"`
	Stage     string    `json:"stage"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}
type CustomerOrderDetail struct {
	ID                   string              `json:"id"`
	OrderNo              string              `json:"orderNo"`
	Status               string              `json:"status"`
	StatusText           string              `json:"statusText"`
	CancelReason         string              `json:"cancelReason,omitempty"`
	ContactName          string              `json:"contactName"`
	ContactMobile        string              `json:"contactMobile"`
	ServiceAddress       string              `json:"serviceAddress"`
	TotalAmount          int64               `json:"totalAmount"`
	Version              int                 `json:"version"`
	CreatedAt            time.Time           `json:"createdAt"`
	AppointmentAt        *time.Time          `json:"appointmentAt,omitempty"`
	AppointmentSlot      string              `json:"appointmentSlot,omitempty"`
	AppointmentSlotLabel string              `json:"appointmentSlotLabel,omitempty"`
	WorkOrders           []CustomerWorkOrder `json:"workOrders"`
}

func (s *Service) CustomerOrders(ctx context.Context, p auth.Principal, status string, page, size int) (CustomerOrderPage, error) {
	if p.Role != "CUSTOMER" {
		return CustomerOrderPage{}, httpx.E("FORBIDDEN", "无权查看客户订单", 403)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	var out CustomerOrderPage
	out.Page = page
	out.PageSize = size
	out.Items = []CustomerOrderSummary{}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM customer_order WHERE org_id=$1 AND customer_id=$2 AND ($3='' OR status=$3)`, p.OrgID, p.SubjectID, status).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.id,o.order_no,o.status,COALESCE(o.cancel_reason,''),o.total_amount,o.item_count,o.created_at,COUNT(w.id),COUNT(w.id) FILTER (WHERE w.status='FINISHED'),COUNT(w.id) FILTER (WHERE COALESCE(w.customer_acceptance_status,'') IN ('MANUAL_ACCEPTED','AUTO_ACCEPTED')) FROM customer_order o LEFT JOIN work_order w ON w.org_id=o.org_id AND w.order_id=o.id WHERE o.org_id=$1 AND o.customer_id=$2 AND ($3='' OR o.status=$3) GROUP BY o.id ORDER BY o.created_at DESC LIMIT $4 OFFSET $5`, p.OrgID, p.SubjectID, status, size, (page-1)*size)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var acceptedWorkOrderTotal int
		var x CustomerOrderSummary
		if err = rows.Scan(&id, &x.OrderNo, &x.Status, &x.CancelReason, &x.TotalAmount, &x.ItemCount, &x.CreatedAt, &x.WorkOrderTotal, &x.WorkOrderFinished, &acceptedWorkOrderTotal); err != nil {
			return out, err
		}
		x.ID = fmt.Sprint(id)
		x.StatusText = statusText(x.Status)
		if x.Status == OrderCancelled && x.CancelReason != "" {
			x.StatusText = "商家已打回"
		}
		if x.WorkOrderTotal > 0 && acceptedWorkOrderTotal == x.WorkOrderTotal {
			x.StatusText = statusText(OrderCompleted)
			x.WorkOrderFinished = x.WorkOrderTotal
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}

func (s *Service) CustomerOrder(ctx context.Context, p auth.Principal, id int64) (CustomerOrderDetail, error) {
	if p.Role != "CUSTOMER" {
		return CustomerOrderDetail{}, httpx.E("FORBIDDEN", "无权查看客户订单", 403)
	}
	var out CustomerOrderDetail
	var oid int64
	var appointment sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT id,order_no,status,COALESCE(cancel_reason,''),contact_name,contact_mobile,service_address,total_amount,version,created_at,appointment_at,COALESCE(appointment_slot,'') FROM customer_order WHERE org_id=$1 AND customer_id=$2 AND id=$3`, p.OrgID, p.SubjectID, id).Scan(&oid, &out.OrderNo, &out.Status, &out.CancelReason, &out.ContactName, &out.ContactMobile, &out.ServiceAddress, &out.TotalAmount, &out.Version, &out.CreatedAt, &appointment, &out.AppointmentSlot); err == sql.ErrNoRows {
		return out, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	} else if err != nil {
		return out, err
	}
	out.ID = fmt.Sprint(oid)
	out.StatusText = statusText(out.Status)
	if out.Status == OrderCancelled && out.CancelReason != "" {
		out.StatusText = "商家已打回"
	}
	if appointment.Valid {
		out.AppointmentAt = &appointment.Time
	}
	out.AppointmentSlotLabel = appointmentSlotText(out.AppointmentSlot)
	out.WorkOrders = []CustomerWorkOrder{}
	rows, err := s.db.QueryContext(ctx, `SELECT w.id,w.work_order_no,w.status,COALESCE(w.customer_acceptance_status,''),COALESCE(e.display_name,''),w.appointment_at,COALESCE(w.appointment_slot,''),COALESCE(w.completion_summary,''),w.version FROM work_order w LEFT JOIN employee_account e ON e.org_id=w.org_id AND e.id=w.assignee_id WHERE w.org_id=$1 AND w.order_id=$2 ORDER BY w.id`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var wid int64
		var x CustomerWorkOrder
		if err = rows.Scan(&wid, &x.WorkOrderNo, &x.Status, &x.CustomerAcceptanceStatus, &x.AssigneeName, &x.AppointmentAt, &x.AppointmentSlot, &x.CompletionSummary, &x.Version); err != nil {
			return out, err
		}
		x.ID = fmt.Sprint(wid)
		x.AppointmentSlotLabel = appointmentSlotText(x.AppointmentSlot)
		x.StatusText = statusText(x.Status)
		x.Evidence = []CustomerEvidence{}
		ers, ee := s.db.QueryContext(ctx, `SELECT e.id,e.media_id,e.stage,e.created_at FROM work_order_evidence e WHERE e.org_id=$1 AND e.work_order_id=$2 AND e.customer_visible=true ORDER BY e.created_at`, p.OrgID, wid)
		if ee != nil {
			return out, ee
		}
		for ers.Next() {
			var eid, mid int64
			var ev CustomerEvidence
			if ee = ers.Scan(&eid, &mid, &ev.Stage, &ev.CreatedAt); ee != nil {
				ers.Close()
				return out, ee
			}
			ev.ID = fmt.Sprint(eid)
			ev.MediaID = fmt.Sprint(mid)
			ev.URL = "/api/v1/mini/media/" + ev.MediaID + "/content"
			x.Evidence = append(x.Evidence, ev)
		}
		ers.Close()
		out.WorkOrders = append(out.WorkOrders, x)
	}
	if len(out.WorkOrders) > 0 {
		allAccepted := true
		for _, work := range out.WorkOrders {
			if work.CustomerAcceptanceStatus != "MANUAL_ACCEPTED" && work.CustomerAcceptanceStatus != "AUTO_ACCEPTED" {
				allAccepted = false
				break
			}
		}
		if allAccepted {
			out.StatusText = statusText(OrderCompleted)
		}
	}
	return out, rows.Err()
}

func (s *Service) CustomerWorkOrder(ctx context.Context, p auth.Principal, id int64) (CustomerWorkOrder, error) {
	if p.Role != "CUSTOMER" {
		return CustomerWorkOrder{}, httpx.E("FORBIDDEN", "无权查看工单", 403)
	}
	var orderID int64
	if err := s.db.QueryRowContext(ctx, `SELECT w.order_id FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id WHERE w.org_id=$1 AND w.id=$2 AND o.customer_id=$3`, p.OrgID, id, p.SubjectID).Scan(&orderID); err == sql.ErrNoRows {
		return CustomerWorkOrder{}, httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return CustomerWorkOrder{}, err
	}
	detail, err := s.CustomerOrder(ctx, p, orderID)
	if err != nil {
		return CustomerWorkOrder{}, err
	}
	for _, work := range detail.WorkOrders {
		if work.ID == fmt.Sprint(id) {
			return work, nil
		}
	}
	return CustomerWorkOrder{}, httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
}

type AcceptanceRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Version  int    `json:"version"`
}

func (s *Service) CustomerAcceptance(ctx context.Context, p auth.Principal, id int64, req AcceptanceRequest) error {
	if p.Role != "CUSTOMER" {
		return httpx.E("FORBIDDEN", "无权验收工单", 403)
	}
	if req.Decision != "ACCEPT" && req.Decision != "REJECT" {
		return httpx.E("VALIDATION_ERROR", "验收决定不合法", 400)
	}
	if req.Decision == "REJECT" && strings.TrimSpace(req.Reason) == "" {
		return httpx.E("REASON_REQUIRED", "拒绝验收原因必填", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, internalReviewStatus, closureStatus, visitStatus, completionOutcome string
	var version int
	var orderID int64
	if err = tx.QueryRowContext(ctx, `SELECT w.status,w.internal_review_status,w.closure_status,w.visit_status,COALESCE(w.completion_outcome,''),w.version,w.order_id FROM work_order w JOIN customer_order o ON o.org_id=w.org_id AND o.id=w.order_id WHERE w.org_id=$1 AND w.id=$2 AND o.customer_id=$3 FOR UPDATE`, p.OrgID, id, p.SubjectID).Scan(&status, &internalReviewStatus, &closureStatus, &visitStatus, &completionOutcome, &version, &orderID); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_FOUND", "工单不存在", 404)
	} else if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderWaitingAcceptance && status != WorkOrderWaitingQAAudit && status != WorkOrderWaitingDirectorAudit {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能验收", 409)
	}
	to, event := status, "CUSTOMER_ACCEPTED"
	nextClosure, nextVisit, nextOutcome := closureStatus, visitStatus, completionOutcome
	finished := false
	if req.Decision == "REJECT" {
		to, event = WorkOrderReworkRequired, "CUSTOMER_REJECTED"
		nextClosure, nextVisit = "SECOND_VISIT_PENDING", "SECOND_VISIT_PENDING"
	} else if internalReviewStatus == "APPROVED" {
		to, nextClosure, nextVisit, nextOutcome, finished = WorkOrderFinished, "FINISHED", "FINISHED", "NORMAL", true
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,customer_acceptance_status=$2,customer_acceptance_source='MANUAL',customer_acceptance_at=CURRENT_TIMESTAMP(3),closure_status=$3,visit_status=$4,completion_outcome=NULLIF($5,''),closed_at=CASE WHEN $6::boolean THEN CURRENT_TIMESTAMP(3) ELSE closed_at END,finished_at=CASE WHEN $6::boolean THEN CURRENT_TIMESTAMP(3) ELSE finished_at END,version=version+1 WHERE org_id=$7 AND id=$8 AND version=$9`, to, map[bool]string{true: "REJECTED", false: "MANUAL_ACCEPTED"}[req.Decision == "REJECT"], nextClosure, nextVisit, nextOutcome, finished, p.OrgID, id, req.Version); err != nil {
		return err
	}
	var submissionID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM completion_submission WHERE org_id=$1 AND work_order_id=$2 ORDER BY attempt_no DESC LIMIT 1`, p.OrgID, id).Scan(&submissionID); err == nil {
		_, _ = tx.ExecContext(ctx, `INSERT INTO customer_acceptance(org_id,work_order_id,submission_id,customer_id,decision,source,reason) VALUES($1,$2,$3,$4,$5,'MANUAL',NULLIF($6,'')) ON CONFLICT (org_id,submission_id) DO NOTHING`, p.OrgID, id, submissionID, p.SubjectID, req.Decision, strings.TrimSpace(req.Reason))
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,'CUSTOMER',$6,$7,NULLIF($8,''))`, p.OrgID, id, status, to, event, p.SubjectID, p.Name, strings.TrimSpace(req.Reason)); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT status FROM work_order WHERE org_id=$1 AND order_id=$2`, p.OrgID, orderID)
	if err != nil {
		return err
	}
	statuses := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		statuses = append(statuses, v)
	}
	rows.Close()
	next := rollupOrder(statuses)
	var previousOrderStatus string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&previousOrderStatus); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE customer_order SET status=$1,version=version+1,completed_at=CASE WHEN $4::boolean THEN CURRENT_TIMESTAMP(3) ELSE completed_at END WHERE org_id=$2 AND id=$3`, next, p.OrgID, orderID, next == OrderCompleted); err != nil {
		return err
	}
	if previousOrderStatus != next {
		if _, err = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$4,'ORDER_ROLLED_UP','SYSTEM',0,'system')`, p.OrgID, orderID, previousOrderStatus, next); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Service) UploadEvidence(ctx context.Context, p auth.Principal, id int64, fh *multipart.FileHeader) (any, error) {
	if p.Role != "WORKER" {
		return nil, httpx.E("FORBIDDEN", "无权上传工单证据", 403)
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3`, p.OrgID, id, p.SubjectID).Scan(&status); err == sql.ErrNoRows {
		return nil, httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	} else if err != nil {
		return nil, err
	}
	if status != WorkOrderPendingArrival && status != WorkOrderArrived && status != WorkOrderInService {
		return nil, httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能上传证据", 409)
	}
	u, err := s.media.UploadWorkOrder(ctx, p, id, fh)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func parseID(v string) int64 { var n int64; fmt.Sscan(v, &n); return n }

func (s *Service) BindEvidence(ctx context.Context, p auth.Principal, id int64, req EvidenceRequest) error {
	if p.Role != "WORKER" {
		return httpx.E("FORBIDDEN", "无权绑定工单证据", 403)
	}
	if req.Stage != "BEFORE" && req.Stage != "DURING" && req.Stage != "AFTER" {
		return httpx.E("VALIDATION_ERROR", "证据节点不合法", 400)
	}
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3`, p.OrgID, id, p.SubjectID).Scan(&status); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	} else if err != nil {
		return err
	}
	if status != WorkOrderPendingArrival && status != WorkOrderArrived && status != WorkOrderInService {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能绑定证据", 409)
	}
	visible := true
	if req.CustomerVisible != nil {
		visible = *req.CustomerVisible
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO work_order_evidence(org_id,work_order_id,media_id,stage,customer_visible,uploaded_by) SELECT $1,$2,m.id,$3,$4,$5 FROM media_asset m WHERE m.org_id=$1 AND m.id=$6 AND m.owner_type='WORK_ORDER' AND m.owner_id=$2 AND m.purpose='WORK_ORDER_EVIDENCE' AND m.status='READY' ON CONFLICT (org_id,work_order_id,media_id) DO NOTHING`, p.OrgID, id, req.Stage, visible, p.SubjectID, req.MediaID)
	if err != nil {
		return err
	}
	var exists int
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_order_evidence WHERE org_id=$1 AND work_order_id=$2 AND media_id=$3`, p.OrgID, id, req.MediaID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return httpx.E("EVIDENCE_NOT_ACCESSIBLE", "媒体不存在或不属于当前工单", 403)
	}
	return nil
}

func (s *Service) SubmitCompletion(ctx context.Context, p auth.Principal, id int64, req CompletionRequest, key string) error {
	if p.Role != "WORKER" {
		return httpx.E("FORBIDDEN", "无权提交完工", 403)
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return httpx.E("VALIDATION_ERROR", "缺少有效 Idempotency-Key", 400)
	}
	summary := strings.TrimSpace(req.CompletionSummary)
	if len([]rune(summary)) < 5 || len([]rune(summary)) > 1000 {
		return httpx.E("COMPLETION_SUMMARY_REQUIRED", "完工说明需为 5-1000 字", 400)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	body, _ := json.Marshal(struct {
		ID      int64
		Request CompletionRequest
	}{id, req})
	hash := sha256.Sum256(body)
	var idemID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'WORKER',$2,$3,$4,'PROCESSING',NULL,$5) ON CONFLICT (org_id,principal_type,principal_id,idempotency_key) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID, key, hash[:], time.Now().UTC().Add(24*time.Hour)).Scan(&idemID)
	if err == sql.ErrNoRows {
		var old, raw []byte
		if err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='WORKER' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&old, &raw); err != nil {
			return err
		}
		if subtle.ConstantTimeCompare(old, hash[:]) != 1 {
			return httpx.E("IDEMPOTENCY_KEY_CONFLICT", "幂等键已用于不同请求", 409)
		}
		if len(raw) == 0 {
			return httpx.E("COMMAND_IN_PROGRESS", "命令正在处理", 409)
		}
		return nil
	}
	if err != nil {
		return err
	}
	var status string
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT status,version FROM work_order WHERE org_id=$1 AND id=$2 AND assignee_id=$3 FOR UPDATE`, p.OrgID, id, p.SubjectID).Scan(&status, &version); err == sql.ErrNoRows {
		return httpx.E("WORK_ORDER_NOT_ASSIGNED_TO_YOU", "工单不属于当前师傅", 403)
	}
	if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderInService {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能提交完工", 409)
	}
	var before, after int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE stage='BEFORE'),COUNT(*) FILTER (WHERE stage='AFTER') FROM work_order_evidence WHERE org_id=$1 AND work_order_id=$2`, p.OrgID, id).Scan(&before, &after); err != nil {
		return err
	}
	if before < 1 || after < 1 {
		return httpx.E("COMPLETION_EVIDENCE_INCOMPLETE", "缺少施工前或施工后图片", 409)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,visit_status='COMPLETION_SUBMITTED',customer_acceptance_status='PENDING',internal_review_status='PENDING_QA',closure_status='OPEN',completion_summary=$2,completion_submitted_at=CURRENT_TIMESTAMP(3),completion_submission_at=CURRENT_TIMESTAMP(3),auto_accept_due_at=CURRENT_TIMESTAMP(3)+INTERVAL '7 days',version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5`, WorkOrderWaitingQAAudit, summary, p.OrgID, id, req.Version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO completion_submission(org_id,work_order_id,attempt_no,worker_id,summary) SELECT $1,$2,COALESCE(MAX(attempt_no),0)+1,$3,$4 FROM completion_submission WHERE org_id=$1 AND work_order_id=$2`, p.OrgID, id, p.SubjectID, summary); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,$3,$4,'COMPLETION_SUBMITTED','WORKER',$5,$6)`, p.OrgID, id, status, WorkOrderWaitingQAAudit, p.SubjectID, p.Name); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_event(org_id,work_order_id,event_code,operator_type,operator_id,note) VALUES($1,$2,'COMPLETION_SUBMITTED','WORKER',$3,$4)`, p.OrgID, id, p.SubjectID, summary); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE idempotency_record SET response_code='OK',response_body=$1 WHERE id=$2`, []byte(`{"updated":true}`), idemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ReviewCompletion(ctx context.Context, p auth.Principal, id int64, req ReviewRequest) error {
	if p.Role != "ADMIN" {
		return httpx.E("FORBIDDEN", "无权审核完工", 403)
	}
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		return httpx.E("VALIDATION_ERROR", "审核决定不合法", 400)
	}
	if req.Decision == "REJECT" && strings.TrimSpace(req.Note) == "" {
		return httpx.E("REASON_REQUIRED", "驳回原因必填", 400)
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
	}
	if err != nil {
		return err
	}
	if version != req.Version {
		return httpx.E("RESOURCE_VERSION_CONFLICT", "工单已被修改", 409)
	}
	if status != WorkOrderWaitingCompletionReview {
		return httpx.E("WORK_ORDER_STATUS_CONFLICT", "当前状态不能审核", 409)
	}
	to, event := WorkOrderWaitingAcceptance, "COMPLETION_APPROVED"
	if req.Decision == "REJECT" {
		to, event = WorkOrderInService, "COMPLETION_REJECTED"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_order SET status=$1,review_note=$2,reviewed_at=CURRENT_TIMESTAMP(3),version=version+1 WHERE org_id=$3 AND id=$4 AND version=$5`, to, strings.TrimSpace(req.Note), p.OrgID, id, req.Version); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO work_order_status_history(org_id,work_order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name,reason) VALUES($1,$2,$3,$4,$5,'ADMIN',$6,$7,NULLIF($8,''))`, p.OrgID, id, status, to, event, p.SubjectID, p.Name, strings.TrimSpace(req.Note)); err != nil {
		return err
	}
	return tx.Commit()
}
