package middleware

type contextKey string

const (
	// ContextKeyUserID stores the authenticated user's UUID in the request context.
	ContextKeyUserID contextKey = "user_id"
	// ContextKeyEmail stores the authenticated user's email in the request context.
	ContextKeyEmail contextKey = "email"
	// ContextKeyOrgID stores the authenticated user's organization UUID in the request context.
	ContextKeyOrgID contextKey = "org_id"
	// ContextKeyRole stores the authenticated user's role in the request context.
	ContextKeyRole contextKey = "role"
)
