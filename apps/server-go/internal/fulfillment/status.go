package fulfillment

import "fmt"

const (
	OrderPendingConfirmation = "PENDING_CONFIRMATION"
	OrderFulfilling          = "FULFILLING"
	OrderWaitingAcceptance   = "WAITING_ACCEPTANCE"
	OrderCompleted           = "COMPLETED"
	OrderCancelled           = "CANCELLED"

	WorkOrderPendingDispatch         = "PENDING_DISPATCH"
	WorkOrderPendingAccept           = "PENDING_ACCEPT"
	WorkOrderPendingArrival          = "PENDING_ARRIVAL"
	WorkOrderArrived                 = "ARRIVED"
	WorkOrderInService               = "IN_SERVICE"
	WorkOrderWaitingCompletionReview = "WAITING_COMPLETION_REVIEW"
	WorkOrderWaitingAcceptance       = "WAITING_ACCEPTANCE"
	WorkOrderReworkRequired          = "REWORK_REQUIRED"
	WorkOrderFinished                = "FINISHED"
	WorkOrderCancelled               = "CANCELLED"
	WorkOrderWaitingQAAudit          = "WAITING_QA_AUDIT"
	WorkOrderWaitingDirectorAudit    = "WAITING_DIRECTOR_AUDIT"
	WorkOrderWaitingCustomerService  = "WAITING_CUSTOMER_SERVICE_CONFIRMATION"
	WorkOrderSecondVisitPending      = "SECOND_VISIT_PENDING"
	WorkOrderFinishedReviewException = "FINISHED_WITH_REVIEW_EXCEPTION"
)

func canConfirm(status string) bool { return status == OrderPendingConfirmation }

func canCancelOrder(status string) bool {
	return status == OrderPendingConfirmation || status == OrderFulfilling
}

func rollupOrder(statuses []string) string {
	if len(statuses) == 0 {
		return OrderFulfilling
	}
	allFinished := true
	allCancelled := true
	waitingAcceptance := false
	for _, status := range statuses {
		if status != WorkOrderFinished {
			allFinished = false
		}
		if status != WorkOrderCancelled {
			allCancelled = false
		}
		if status == WorkOrderWaitingAcceptance {
			waitingAcceptance = true
		}
		switch status {
		case WorkOrderPendingDispatch, WorkOrderPendingAccept, WorkOrderPendingArrival, WorkOrderArrived, WorkOrderInService, WorkOrderWaitingCompletionReview, WorkOrderReworkRequired:
			return OrderFulfilling
		}
	}
	if allFinished {
		return OrderCompleted
	}
	if waitingAcceptance {
		return OrderWaitingAcceptance
	}
	if allCancelled {
		return OrderCancelled
	}
	return OrderFulfilling
}

func invalidTransition(code, message string) error { return fmt.Errorf("%s: %s", code, message) }

func statusText(status string) string {
	labels := map[string]string{"PENDING_PAYMENT": "待支付", "PENDING_CONFIRMATION": "待确认", "FULFILLING": "履约中", "WAITING_ACCEPTANCE": "待客户验收", "COMPLETED": "已完成", "CANCELLED": "已取消", "PENDING_DISPATCH": "待派单", "PENDING_ACCEPT": "待师傅接单", "PENDING_ARRIVAL": "待上门", "ARRIVED": "已到达", "IN_SERVICE": "服务中", "WAITING_COMPLETION_REVIEW": "待审核", "WAITING_QA_AUDIT": "待质检初审", "WAITING_DIRECTOR_AUDIT": "待总监审核", "WAITING_CUSTOMER_SERVICE_CONFIRMATION": "待客服确认", "SECOND_VISIT_PENDING": "待二次上门", "REWORK_REQUIRED": "待返工", "FINISHED": "已完成", "FINISHED_WITH_REVIEW_EXCEPTION": "审核异常已完结"}
	if v, ok := labels[status]; ok {
		return v
	}
	return status
}

func appointmentSlotText(slot string) string {
	if !validAppointmentSlot(slot) {
		return slot
	}
	hour := 0
	fmt.Sscanf(slot, "%d:00", &hour)
	return fmt.Sprintf("%02d:00-%02d:00", hour, hour+2)
}
