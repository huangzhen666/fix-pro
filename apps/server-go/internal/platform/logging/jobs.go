package logging

import (
	"context"
	"go.uber.org/zap"
	"time"
)

func RunJob(parent context.Context, l *zap.Logger, c Config, name string, fn func(context.Context) error) {
	ctx, cancel := context.WithTimeout(parent, c.JobSlow*2)
	defer cancel()
	ctx = Context(ctx, l, c, "")
	Bind(ctx, zap.String("jobName", name), zap.String("jobRunId", ID()), zap.String("principalType", "SYSTEM"))
	start := time.Now()
	Event(ctx, "jobs", "job.started")
	defer func() {
		if x := recover(); x != nil {
			Count(ctx, false)
			FromContext(ctx).Error("job panic", zap.String("module", "jobs"), zap.String("event", "job.panic"), zap.String("errorType", "panic"), zap.String("stack", Stack()))
			p, f := Counts(ctx)
			FromContext(ctx).Error("job failed", zap.String("module", "jobs"), zap.String("event", "job.failed"), zap.Int("processedCount", p), zap.Int("failedCount", f), zap.Int64("durationMs", time.Since(start).Milliseconds()))
		}
	}()
	err := fn(ctx)
	if err == nil {
		err = ctx.Err()
	}
	p, f := Counts(ctx)
	fields := []zap.Field{zap.String("module", "jobs"), zap.Int64("durationMs", time.Since(start).Milliseconds()), zap.Int("processedCount", p), zap.Int("failedCount", f)}
	if err != nil {
		if f == 0 {
			f = 1
		}
		fields[3] = zap.Int("failedCount", f)
		fields = append(fields, SafeError(err)...)
		fields = append(fields, zap.String("event", "job.failed"), zap.String("result", "failure"), zap.String("stack", Stack()))
		FromContext(ctx).Error("job failed", fields...)
	} else {
		fields = append(fields, zap.String("event", "job.completed"), zap.String("result", "success"))
		if time.Since(start) >= c.JobSlow {
			FromContext(ctx).Warn("job completed", fields...)
		} else {
			FromContext(ctx).Info("job completed", fields...)
		}
	}
}
