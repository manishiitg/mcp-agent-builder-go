package common

import "context"

// IsWorkflowReadOnlyAccess reports whether the caller of ctx currently holds
// only read access to workflows (PLAT-262). See WorkflowReadOnlyAccessKey's
// doc comment for who sets this and why it lives here instead of in
// cmd/server, which owns the real WorkflowAccessLevel type.
func IsWorkflowReadOnlyAccess(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	readOnly, _ := ctx.Value(WorkflowReadOnlyAccessKey).(bool)
	return readOnly
}

// WithWorkflowReadOnlyAccess returns a copy of ctx carrying the caller's
// current workflow read-only status.
func WithWorkflowReadOnlyAccess(ctx context.Context, readOnly bool) context.Context {
	return context.WithValue(ctx, WorkflowReadOnlyAccessKey, readOnly)
}
