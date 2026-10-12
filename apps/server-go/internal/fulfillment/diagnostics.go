package fulfillment

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
)

func committed(ctx context.Context, tx *sql.Tx, event string, org, work, order int64, from, to string) error {
	if work > 0 && order == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT order_id FROM work_order WHERE org_id=$1 AND id=$2`, org, work).Scan(&order); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fields := []zap.Field{zap.String("orgId", strconv.FormatInt(org, 10))}
	if from != "" {
		fields = append(fields, zap.String("fromStatus", from))
	}
	if to != "" {
		fields = append(fields, zap.String("toStatus", to))
	}
	if work > 0 {
		fields = append(fields, zap.String("workOrderId", strconv.FormatInt(work, 10)))
	}
	if order > 0 {
		fields = append(fields, zap.String("orderId", strconv.FormatInt(order, 10)))
	}
	logging.Bind(ctx, fields...)
	logging.Event(ctx, "fulfillment", event, fields...)
	logging.Count(ctx, true)
	return nil
}
