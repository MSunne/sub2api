package handler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func bindUsageRecordContext(source context.Context, task service.UsageRecordTask) service.UsageRecordTask {
	if task == nil {
		return nil
	}
	clientRequestID, _ := source.Value(ctxkey.ClientRequestID).(string)
	localRequestID, _ := source.Value(ctxkey.RequestID).(string)
	clientRequestID = strings.TrimSpace(clientRequestID)
	localRequestID = strings.TrimSpace(localRequestID)
	return func(ctx context.Context) {
		if clientRequestID != "" {
			ctx = context.WithValue(ctx, ctxkey.ClientRequestID, clientRequestID)
		}
		if localRequestID != "" {
			ctx = context.WithValue(ctx, ctxkey.RequestID, localRequestID)
		}
		task(ctx)
	}
}
