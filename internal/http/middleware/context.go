package middlewarex

import "context"

type ctxKey string

const (
	ctxTenantID ctxKey = "tenant_id"
)

func WithTenantID(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, ctxTenantID, tenantID)
}

func TenantID(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(ctxTenantID).(int64)
	return v, ok
}
