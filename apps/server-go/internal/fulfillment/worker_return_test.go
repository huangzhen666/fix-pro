package fulfillment

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func workerReturnDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("FIXPRO_INTEGRATION") != "1" {
		t.Skip("set FIXPRO_INTEGRATION=1 to run against PostgreSQL")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWorkerReturnKeepsAppointment(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	appointment := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)

	var orderID, workOrderID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'退回测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("RETURN-ORDER-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,999999,'PENDING_ACCEPT','NORMAL',$3,'14:00') RETURNING id`, fmt.Sprintf("RETURN-WORK-%d", suffix), orderID, appointment).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	})

	service := New(db, nil)
	principal := auth.Principal{OrgID: 1, SubjectID: 999999, Role: "WORKER", Name: "退回测试师傅"}
	if err := service.WorkerReturn(ctx, principal, workOrderID, 0, "无法按时服务"); err != nil {
		t.Fatal(err)
	}

	var status string
	var appointmentAt sql.NullTime
	var appointmentSlot sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT status,appointment_at,appointment_slot FROM work_order WHERE id=$1`, workOrderID).Scan(&status, &appointmentAt, &appointmentSlot); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderPendingDispatch {
		t.Fatalf("status = %q, want %q", status, WorkOrderPendingDispatch)
	}
	if !appointmentAt.Valid || !appointmentAt.Time.Equal(appointment) {
		t.Fatalf("appointment_at = %v, want %s", appointmentAt, appointment)
	}
	if !appointmentSlot.Valid || appointmentSlot.String != "14:00" {
		t.Fatalf("appointment_slot = %q, want 14:00", appointmentSlot.String)
	}
}

func TestWorkOrdersFallsBackToOrderAppointment(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	appointment := time.Date(2026, 8, 30, 6, 0, 0, 0, time.UTC)

	var orderID, workOrderID int64
	orderNo := fmt.Sprintf("FO-%d", suffix)
	workOrderNo := fmt.Sprintf("FW-%d", suffix)
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count,appointment_at,appointment_slot) VALUES(1,$1,1,'回退测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1,$2,'14:00') RETURNING id`, orderNo, appointment).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,status,priority) VALUES(1,$1,$2,'PENDING_DISPATCH','NORMAL') RETURNING id`, workOrderNo, orderID).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	})

	result, err := New(db, nil).WorkOrders(ctx, 1, "", 0, "", "", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		if item.WorkOrderNo != workOrderNo {
			continue
		}
		if item.AppointmentAt == nil || !item.AppointmentAt.Equal(appointment) {
			t.Fatalf("appointment_at = %v, want %s", item.AppointmentAt, appointment)
		}
		if item.AppointmentSlot != "14:00" {
			t.Fatalf("appointment_slot = %q, want 14:00", item.AppointmentSlot)
		}
		return
	}
	t.Fatalf("work order %q not found", workOrderNo)
}

func TestAssignFallsBackToOrderAppointment(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	appointment := time.Date(2099, 8, 30, 6, 0, 0, 0, time.UTC)

	var orderID, workOrderID, workerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count,appointment_at,appointment_slot) VALUES(1,$1,1,'派单回退测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1,$2,'14:00') RETURNING id`, fmt.Sprintf("AO-%d", suffix), appointment).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,status,priority) VALUES(1,$1,$2,'PENDING_DISPATCH','NORMAL') RETURNING id`, fmt.Sprintf("AW-%d", suffix), orderID).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'派单测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("139%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	admin := auth.Principal{OrgID: 1, SubjectID: 1, Role: "ADMIN", Name: "派单测试管理员"}
	if err := New(db, nil).Assign(ctx, admin, workOrderID, AssignRequest{WorkerID: workerID, Version: 0}); err != nil {
		t.Fatal(err)
	}

	var status, appointmentSlot string
	var appointmentAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT status,appointment_at,appointment_slot FROM work_order WHERE id=$1`, workOrderID).Scan(&status, &appointmentAt, &appointmentSlot); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderPendingAccept {
		t.Fatalf("status = %q, want %q", status, WorkOrderPendingAccept)
	}
	if !appointmentAt.Equal(appointment) {
		t.Fatalf("appointment_at = %s, want %s", appointmentAt, appointment)
	}
	if appointmentSlot != "14:00" {
		t.Fatalf("appointment_slot = %q, want 14:00", appointmentSlot)
	}
}

func TestWorkerAcceptRejectsExpiredAppointment(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	appointment := time.Now().UTC().Add(-24 * time.Hour)
	key := fmt.Sprintf("expired-accept-%d", suffix)

	var orderID, workOrderID, workerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'超时接单测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("EAO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'超时测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("137%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,$3,'PENDING_ACCEPT','NORMAL',$4,'08:00') RETURNING id`, fmt.Sprintf("EAW-%d", suffix), orderID, workerID, appointment).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM idempotency_record WHERE org_id=1 AND principal_type='WORKER' AND principal_id=$1 AND idempotency_key=$2`, workerID, key)
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	err := New(db, nil).WorkerCommand(ctx, auth.Principal{OrgID: 1, SubjectID: workerID, Role: "WORKER", Name: "超时测试师傅"}, workOrderID, "ACCEPT", RejectRequest{Version: 0}, key)
	if err == nil || !strings.Contains(err.Error(), "WORK_ORDER_ACCEPTANCE_EXPIRED") {
		t.Fatalf("err = %v, want WORK_ORDER_ACCEPTANCE_EXPIRED", err)
	}
	var status string
	if err = db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE id=$1`, workOrderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderPendingAccept {
		t.Fatalf("status = %q, want %q", status, WorkOrderPendingAccept)
	}
}

func TestAdminRecallReturnsWorkOrderToPendingDispatch(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	appointment := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Millisecond)

	var orderID, workOrderID, workerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'收回派单测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("ARO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'收回测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("136%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,$3,'PENDING_ACCEPT','NORMAL',$4,'14:00') RETURNING id`, fmt.Sprintf("ARW-%d", suffix), orderID, workerID, appointment).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	if err := New(db, nil).Recall(ctx, auth.Principal{OrgID: 1, SubjectID: 1, Role: "ADMIN", Name: "收回测试管理员"}, workOrderID, RecallRequest{Version: 0, Reason: "师傅长时间未接单"}); err != nil {
		t.Fatal(err)
	}
	var status, appointmentSlot string
	var assignee sql.NullInt64
	var appointmentAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT status,assignee_id,appointment_at,appointment_slot FROM work_order WHERE id=$1`, workOrderID).Scan(&status, &assignee, &appointmentAt, &appointmentSlot); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderPendingDispatch || assignee.Valid {
		t.Fatalf("status = %q, assignee = %v; want pending dispatch without assignee", status, assignee)
	}
	if !appointmentAt.Equal(appointment) || appointmentSlot != "14:00" {
		t.Fatalf("appointment = %s %q, want %s %q", appointmentAt, appointmentSlot, appointment, "14:00")
	}
	var reason string
	if err := db.QueryRowContext(ctx, `SELECT reason FROM work_order_status_history WHERE work_order_id=$1 AND event_code='ADMIN_RECALLED'`, workOrderID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "师傅长时间未接单" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestSubmitCompletionRequiresEvidencePairForEveryPurchasedUnit(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	key := fmt.Sprintf("evidence-pairs-%d", suffix)

	var orderID, workOrderID, orderItemID, workOrderItemID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'凭证配对测试','13800138000','测试地址','REPAIR','FULFILLING',200,0,2) RETURNING id`, fmt.Sprintf("EPO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,fault_description,unit_price,quantity,subtotal_amount) VALUES(1,$1,1,1,'PAIR','配对服务','次','测试范围','无','无','',100,2,200) RETURNING id`, orderID).Scan(&orderItemID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority) VALUES(1,$1,$2,999998,'IN_SERVICE','NORMAL') RETURNING id`, fmt.Sprintf("EPW-%d", suffix), orderID).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order_item(org_id,work_order_id,order_item_id,quantity) VALUES(1,$1,$2,2) RETURNING id`, workOrderID, orderItemID).Scan(&workOrderItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO work_order_evidence(org_id,work_order_id,media_id,stage,work_order_item_id,unit_no,customer_visible,uploaded_by) VALUES(1,$1,$2,'BEFORE',$3,1,TRUE,999998),(1,$1,$4,'AFTER',$3,1,TRUE,999998)`, workOrderID, suffix+1, workOrderItemID, suffix+2); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM idempotency_record WHERE org_id=1 AND principal_type='WORKER' AND principal_id=999998 AND idempotency_key=$1`, key)
		db.ExecContext(ctx, `DELETE FROM work_order_event WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM completion_submission WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_evidence WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_item WHERE id=$1`, workOrderItemID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM order_item WHERE id=$1`, orderItemID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	})

	worker := auth.Principal{OrgID: 1, SubjectID: 999998, Role: "WORKER", Name: "凭证配对测试师傅"}
	err := New(db, nil).SubmitCompletion(ctx, worker, workOrderID, CompletionRequest{Version: 0, CompletionSummary: "服务已完成并完成检查"}, key)
	if err == nil || !strings.Contains(err.Error(), "COMPLETION_EVIDENCE_INCOMPLETE") {
		t.Fatalf("err = %v, want COMPLETION_EVIDENCE_INCOMPLETE", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO work_order_evidence(org_id,work_order_id,media_id,stage,work_order_item_id,unit_no,customer_visible,uploaded_by) VALUES(1,$1,$2,'BEFORE',$3,2,TRUE,999998),(1,$1,$4,'AFTER',$3,2,TRUE,999998)`, workOrderID, suffix+3, workOrderItemID, suffix+4); err != nil {
		t.Fatal(err)
	}
	if err = New(db, nil).SubmitCompletion(ctx, worker, workOrderID, CompletionRequest{Version: 0, CompletionSummary: "服务已完成并完成检查"}, key); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE id=$1`, workOrderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderWaitingQAAudit {
		t.Fatalf("status = %q, want %q", status, WorkOrderWaitingQAAudit)
	}
}

func TestWorkerEvidenceBindsUsingDetailProjectID(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	var orderID, workOrderID, orderItemID, workOrderItemID, workerID, mediaID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'上传映射测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("EMO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'上传映射测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("135%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,fault_description,unit_price,quantity,subtotal_amount) VALUES(1,$1,1,1,'MAP','上传映射服务','次','测试范围','无','无','',100,1,100) RETURNING id`, orderID).Scan(&orderItemID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority) VALUES(1,$1,$2,$3,'IN_SERVICE','NORMAL') RETURNING id`, fmt.Sprintf("EMW-%d", suffix), orderID, workerID).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order_item(org_id,work_order_id,order_item_id,quantity) VALUES(1,$1,$2,1) RETURNING id`, workOrderID, orderItemID).Scan(&workOrderItemID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'WORK_ORDER',$1,'WORK_ORDER_EVIDENCE','IMAGE','test.png',$2,'image/png',1,'READY') RETURNING id`, workOrderID, fmt.Sprintf("test/evidence-map-%d.png", suffix)).Scan(&mediaID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order_evidence WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM media_asset WHERE id=$1`, mediaID)
		db.ExecContext(ctx, `DELETE FROM work_order_item WHERE id=$1`, workOrderItemID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM order_item WHERE id=$1`, orderItemID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	worker := auth.Principal{OrgID: 1, SubjectID: workerID, Role: "WORKER", Name: "上传映射测试师傅"}
	detail, err := New(db, nil).WorkerWorkOrder(ctx, worker, workOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(detail.Items))
	}
	var detailProjectID int64
	if _, err = fmt.Sscan(detail.Items[0].ID, &detailProjectID); err != nil {
		t.Fatal(err)
	}
	if err = New(db, nil).BindEvidence(ctx, worker, workOrderID, EvidenceRequest{MediaID: mediaID, Stage: "BEFORE", WorkOrderItemID: detailProjectID, UnitNo: 1}); err != nil {
		t.Fatal(err)
	}
	var storedProjectID int64
	if err = db.QueryRowContext(ctx, `SELECT work_order_item_id FROM work_order_evidence WHERE work_order_id=$1 AND media_id=$2`, workOrderID, mediaID).Scan(&storedProjectID); err != nil {
		t.Fatal(err)
	}
	if storedProjectID != workOrderItemID {
		t.Fatalf("work_order_item_id = %d, want %d", storedProjectID, workOrderItemID)
	}
}

func TestRescheduleAllowsExpiredPendingDispatchWorkOrder(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	expiredAt := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	newAppointment := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Millisecond)

	var orderID, workOrderID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'重新预约测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("RSO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,'PENDING_DISPATCH','NORMAL',$3,'14:00') RETURNING id`, fmt.Sprintf("RSW-%d", suffix), orderID, expiredAt).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	})

	admin := auth.Principal{OrgID: 1, SubjectID: 1, Role: "ADMIN", Name: "重新预约测试管理员"}
	if err := New(db, nil).Reschedule(ctx, admin, workOrderID, RescheduleRequest{AppointmentAt: newAppointment, AppointmentSlot: "14:00", Version: 0}); err != nil {
		t.Fatal(err)
	}
	var status, slot string
	var appointment time.Time
	if err := db.QueryRowContext(ctx, `SELECT status,appointment_at,appointment_slot FROM work_order WHERE id=$1`, workOrderID).Scan(&status, &appointment, &slot); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderPendingDispatch || !appointment.Equal(newAppointment) || slot != "14:00" {
		t.Fatalf("status=%q appointment=%s slot=%q", status, appointment, slot)
	}
}

func TestAutoRecallUnstartedDueReturnsExpiredAcceptedWorkOrders(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	expiredAppointment := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)

	var orderID, workerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'自动回收测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("ARSO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'自动回收测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("132%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	createWorkOrder := func(prefix, status string) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,$3,$4,'NORMAL',$5,'08:00') RETURNING id`, fmt.Sprintf("%s-%d", prefix, suffix), orderID, workerID, status, expiredAppointment).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	pendingArrivalID := createWorkOrder("ARSPA", WorkOrderPendingArrival)
	arrivedID := createWorkOrder("ARSA", WorkOrderArrived)
	inServiceID := createWorkOrder("ARSS", WorkOrderInService)
	t.Cleanup(func() {
		for _, id := range []int64{pendingArrivalID, arrivedID, inServiceID} {
			db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, id)
			db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, id)
			db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, id)
		}
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	if err := New(db, nil).AutoRecallUnstartedDue(ctx); err != nil {
		t.Fatal(err)
	}
	assertRecalled := func(id int64) {
		var status, slot string
		var assignee sql.NullInt64
		var appointment time.Time
		if err := db.QueryRowContext(ctx, `SELECT status,assignee_id,appointment_at,appointment_slot FROM work_order WHERE id=$1`, id).Scan(&status, &assignee, &appointment, &slot); err != nil {
			t.Fatal(err)
		}
		if status != WorkOrderPendingDispatch || assignee.Valid {
			t.Fatalf("work order %d status=%q assignee=%v, want pending dispatch with no assignee", id, status, assignee)
		}
		if !appointment.Equal(expiredAppointment) || slot != "08:00" {
			t.Fatalf("work order %d appointment=%s %q, want %s %q", id, appointment, slot, expiredAppointment, "08:00")
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_order_status_history WHERE work_order_id=$1 AND event_code='SYSTEM_RECALLED_NO_SERVICE'`, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("work order %d recall history count=%d, want 1", id, count)
		}
	}
	assertRecalled(pendingArrivalID)
	assertRecalled(arrivedID)

	var inServiceStatus string
	var inServiceAssignee sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT status,assignee_id FROM work_order WHERE id=$1`, inServiceID).Scan(&inServiceStatus, &inServiceAssignee); err != nil {
		t.Fatal(err)
	}
	if inServiceStatus != WorkOrderInService || !inServiceAssignee.Valid || inServiceAssignee.Int64 != workerID {
		t.Fatalf("in-service work order status=%q assignee=%v, want unchanged in-service assignment", inServiceStatus, inServiceAssignee)
	}
}

func TestWorkerCannotStartExpiredAcceptedWorkOrder(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	expiredAppointment := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)
	key := fmt.Sprintf("expired-start-%d", suffix)

	var orderID, workerID, workOrderID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'超时开工测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("ESOO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'超时开工测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("133%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,$3,'ARRIVED','NORMAL',$4,'08:00') RETURNING id`, fmt.Sprintf("ESOW-%d", suffix), orderID, workerID, expiredAppointment).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM idempotency_record WHERE org_id=1 AND principal_type='WORKER' AND principal_id=$1 AND idempotency_key=$2`, workerID, key)
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	worker := auth.Principal{OrgID: 1, SubjectID: workerID, Role: "WORKER", Name: "超时开工测试师傅"}
	err := New(db, nil).WorkerCommand(ctx, worker, workOrderID, "START", RejectRequest{Version: 0}, key)
	if err == nil || !strings.Contains(err.Error(), "WORK_ORDER_SERVICE_EXPIRED") {
		t.Fatalf("start err = %v, want WORK_ORDER_SERVICE_EXPIRED", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE id=$1`, workOrderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderArrived {
		t.Fatalf("status = %q, want %q before the automatic recall runs", status, WorkOrderArrived)
	}
}

func TestExpiredInServiceWorkOrderCannotBeRecalledAndCanComplete(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	key := fmt.Sprintf("expired-in-service-%d", suffix)
	expiredAppointment := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Millisecond)

	var orderID, workOrderID, orderItemID, workOrderItemID, workerID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'超时服务测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("EISO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO employee_account(org_id,username,display_name,password_hash,status,role,mobile,must_change_password,password_version) VALUES(1,$1,'超时服务测试师傅','test','ACTIVE','WORKER',$1,FALSE,1) RETURNING id`, fmt.Sprintf("134%08d", suffix%100000000)).Scan(&workerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,fault_description,unit_price,quantity,subtotal_amount) VALUES(1,$1,1,1,'OVERDUE','超时服务','次','测试范围','无','无','',100,1,100) RETURNING id`, orderID).Scan(&orderItemID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority,appointment_at,appointment_slot) VALUES(1,$1,$2,$3,'IN_SERVICE','NORMAL',$4,'08:00') RETURNING id`, fmt.Sprintf("EISW-%d", suffix), orderID, workerID, expiredAppointment).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order_item(org_id,work_order_id,order_item_id,quantity) VALUES(1,$1,$2,1) RETURNING id`, workOrderID, orderItemID).Scan(&workOrderItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO work_order_evidence(org_id,work_order_id,media_id,stage,work_order_item_id,unit_no,customer_visible,uploaded_by) VALUES(1,$1,$2,'BEFORE',$3,1,TRUE,$4),(1,$1,$5,'AFTER',$3,1,TRUE,$4)`, workOrderID, suffix+1, workOrderItemID, workerID, suffix+2); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM idempotency_record WHERE org_id=1 AND principal_type='WORKER' AND principal_id=$1 AND idempotency_key=$2`, workerID, key)
		db.ExecContext(ctx, `DELETE FROM work_order_event WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM completion_submission WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_evidence WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_item WHERE id=$1`, workOrderItemID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM order_item WHERE id=$1`, orderItemID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM employee_account WHERE id=$1`, workerID)
	})

	service := New(db, nil)
	admin := auth.Principal{OrgID: 1, SubjectID: 1, Role: "ADMIN", Name: "超时服务测试管理员"}
	if err := service.Recall(ctx, admin, workOrderID, RecallRequest{Version: 0, Reason: "预约已结束"}); err == nil || !strings.Contains(err.Error(), "WORK_ORDER_STATUS_CONFLICT") {
		t.Fatalf("recall err = %v, want WORK_ORDER_STATUS_CONFLICT", err)
	}
	worker := auth.Principal{OrgID: 1, SubjectID: workerID, Role: "WORKER", Name: "超时服务测试师傅"}
	if err := service.SubmitCompletion(ctx, worker, workOrderID, CompletionRequest{Version: 0, CompletionSummary: "预约结束后仍已完成服务"}, key); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM work_order WHERE id=$1`, workOrderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != WorkOrderWaitingQAAudit {
		t.Fatalf("status = %q, want %q", status, WorkOrderWaitingQAAudit)
	}
}

func TestCustomerAcceptanceRollsUpOrderAndCustomerSummaryStatus(t *testing.T) {
	db := workerReturnDB(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	var orderID, workOrderID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'订单汇总测试','13800138000','测试地址','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("CAO-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO work_order(org_id,work_order_no,order_id,status,priority,visit_status,customer_acceptance_status,internal_review_status,closure_status) VALUES(1,$1,$2,'WAITING_QA_AUDIT','NORMAL','COMPLETION_SUBMITTED','PENDING','PENDING_QA','OPEN') RETURNING id`, fmt.Sprintf("CAW-%d", suffix), orderID).Scan(&workOrderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM customer_acceptance WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_event WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_assignment_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM work_order_status_history WHERE work_order_id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM order_status_history WHERE order_id=$1`, orderID)
		db.ExecContext(ctx, `DELETE FROM work_order WHERE id=$1`, workOrderID)
		db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	})

	customer := auth.Principal{OrgID: 1, SubjectID: 1, Role: "CUSTOMER", Name: "订单汇总测试客户"}
	service := New(db, nil)
	if err := service.CustomerAcceptance(ctx, customer, workOrderID, AcceptanceRequest{Decision: "ACCEPT", Version: 0}); err != nil {
		t.Fatal(err)
	}

	var orderStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE id=$1`, orderID).Scan(&orderStatus); err != nil {
		t.Fatal(err)
	}
	if orderStatus != OrderCompleted {
		t.Fatalf("stored order status = %q, want %q", orderStatus, OrderCompleted)
	}
	page, err := service.CustomerOrders(ctx, customer, "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == fmt.Sprint(orderID) {
			if item.Status != OrderCompleted {
				t.Fatalf("summary status = %q, want %q", item.Status, OrderCompleted)
			}
			return
		}
	}
	t.Fatalf("customer order %d missing from summary", orderID)
}
