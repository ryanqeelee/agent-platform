package types

import "context"

type governedDataCredentialKey struct{}

// GovernedDataObservabilityContextKey marks a turn whose observability must be
// metadata-only. It carries no authority or business payload, so it is safe to
// retain when the same turn detaches work with logger.CloneContext.
const GovernedDataObservabilityContextKey ContextKey = "GovernedDataObservability"

// WithGovernedDataObservability makes the privacy restriction monotonic for
// the lifetime of a turn. It does not affect model input, persisted history,
// files, or tool execution.
func WithGovernedDataObservability(ctx context.Context) context.Context {
	if GovernedDataObservability(ctx) {
		return ctx
	}
	return context.WithValue(ctx, GovernedDataObservabilityContextKey, true)
}

// GovernedDataObservability reports whether logs and traces for this turn must
// omit business payloads while preserving operational metadata.
func GovernedDataObservability(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, _ := ctx.Value(GovernedDataObservabilityContextKey).(bool)
	return marked
}

// This credential lives only in a validated HTTP turn. It is intentionally
// absent from context cloning, task payloads, agent configuration and history.
type governedDataCredential struct {
	bearer   string
	userID   string
	tenantID uint64
}

func (governedDataCredential) String() string   { return "[redacted user credential]" }
func (governedDataCredential) GoString() string { return "[redacted user credential]" }

// CopyGovernedDataTurnCredential is reserved for detaching the same interactive
// QA turn. Generic workers and context cloning must not copy user credentials.
func CopyGovernedDataTurnCredential(dst, src context.Context) context.Context {
	if !GovernedDataObservability(dst) {
		return dst
	}
	bearer, tenantID, ok := GovernedDataUserCredential(src)
	sourceUser, _ := UserIDFromContext(src)
	destUser, _ := UserIDFromContext(dst)
	destTenant, _ := TenantIDFromContext(dst)
	if !ok || sourceUser != destUser || tenantID != destTenant {
		return dst
	}
	return WithGovernedDataUserCredential(dst, bearer)
}

// WithGovernedDataUserCredential is called only after JWT authentication and
// workspace membership resolution. Only this validated interactive identity can request a server-side Edge connection.
func WithGovernedDataUserCredential(ctx context.Context, bearer string) context.Context {
	userID, hasUser := UserIDFromContext(ctx)
	tenantID, hasTenant := TenantIDFromContext(ctx)
	if !hasUser || !hasTenant || tenantID == 0 || bearer == "" || IsSyntheticUserID(userID) {
		return ctx
	}
	if _, machine := TenantAPIKeyScopeFromContext(ctx); machine {
		return ctx
	}
	return context.WithValue(ctx, governedDataCredentialKey{}, governedDataCredential{bearer, userID, tenantID})
}

// GovernedDataUserCredential refuses a credential when shared-agent execution
// has changed the tenant or caller. Sharing an agent does not share data rights.
func GovernedDataUserCredential(ctx context.Context) (bearer string, tenantID uint64, ok bool) {
	if ctx == nil {
		return "", 0, false
	}
	credential, found := ctx.Value(governedDataCredentialKey{}).(governedDataCredential)
	userID, _ := UserIDFromContext(ctx)
	currentTenant, _ := TenantIDFromContext(ctx)
	_, machine := TenantAPIKeyScopeFromContext(ctx)
	if !found || machine || credential.bearer == "" || credential.userID != userID || credential.tenantID != currentTenant {
		return "", 0, false
	}
	return credential.bearer, credential.tenantID, true
}
