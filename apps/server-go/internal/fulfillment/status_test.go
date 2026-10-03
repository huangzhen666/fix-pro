package fulfillment

import "testing"

func TestRollupOrder(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"in progress", []string{WorkOrderPendingDispatch, WorkOrderFinished}, OrderFulfilling},
		{"waiting acceptance", []string{WorkOrderWaitingAcceptance, WorkOrderFinished}, OrderWaitingAcceptance},
		{"completed", []string{WorkOrderFinished, WorkOrderFinished}, OrderCompleted},
		{"cancelled", []string{WorkOrderCancelled, WorkOrderCancelled}, OrderCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rollupOrder(tt.statuses); got != tt.want {
				t.Fatalf("rollupOrder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRollupOrderInProgressHasPriority(t *testing.T) {
	if got := rollupOrder([]string{WorkOrderWaitingAcceptance, WorkOrderInService}); got != OrderFulfilling {
		t.Fatalf("got %q", got)
	}
}

func TestRollupDoesNotCompleteBeforeInternalReviewFinishes(t *testing.T) {
	for _, status := range []string{WorkOrderWaitingQAAudit, WorkOrderWaitingDirectorAudit} {
		t.Run(status, func(t *testing.T) {
			got := rollupOrderStates([]workOrderRollupState{{status: status, customerAcceptanceStatus: "MANUAL_ACCEPTED"}})
			if got != OrderFulfilling {
				t.Fatalf("rollupOrderStates() = %q, want %q while internal review is pending", got, OrderFulfilling)
			}
		})
	}
}
