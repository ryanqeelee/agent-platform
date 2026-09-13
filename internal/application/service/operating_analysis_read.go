package service

import (
	"context"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func governedAnalysisSessionIDs(
	ctx context.Context,
	repo interfaces.MessageRepository,
	sessionIDs []string,
) (map[string]bool, error) {
	if repo == nil || len(sessionIDs) == 0 {
		return map[string]bool{}, nil
	}
	return repo.GovernedAnalysisSessionIDs(ctx, sessionIDs)
}

// OperatingAnalysisReadPermission is the shared product/member decision used
// by history reads, live admission, availability, and handoff consumption.
type OperatingAnalysisReadPermission struct {
	ProductEnabled bool
	MemberEnabled  bool
}

func (p OperatingAnalysisReadPermission) Allowed() bool {
	return p.ProductEnabled && p.MemberEnabled
}

// CurrentOperatingAnalysisReadPermission reads current Platform authority.
// It deliberately excludes the Edge binding so historical-result permission
// remains valid while a connection is disabled or unavailable.
func CurrentOperatingAnalysisReadPermission(
	ctx context.Context,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
) (OperatingAnalysisReadPermission, error) {
	permission := OperatingAnalysisReadPermission{}
	principal, ok := types.PrincipalFromContext(ctx)
	if !ok || principal.Type != types.PrincipalWebUser || members == nil || tenants == nil {
		return permission, nil
	}
	userID, hasUser := types.UserIDFromContext(ctx)
	tenantID, hasTenant := types.TenantIDFromContext(ctx)
	if !hasUser || !hasTenant || userID == "" || tenantID == 0 {
		return permission, nil
	}
	tenant, err := tenants.GetTenantByID(ctx, tenantID)
	if err != nil {
		return permission, err
	}
	permission.ProductEnabled = tenant != nil && tenant.ID == tenantID &&
		tenant.Status == types.TenantStatusActive && tenant.AnalysisEnabled
	if !permission.ProductEnabled {
		return permission, nil
	}
	membership, err := members.GetMembership(ctx, userID, tenantID)
	if err != nil {
		return permission, err
	}
	permission.MemberEnabled = membership != nil &&
		membership.UserID == userID &&
		membership.TenantID == tenantID &&
		membership.Status == types.TenantMemberStatusActive &&
		membership.OperatingAnalysisAccess
	return permission, nil

}

func currentMemberCanReadOperatingAnalysis(
	ctx context.Context,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
) (bool, error) {
	permission, err := CurrentOperatingAnalysisReadPermission(ctx, members, tenants)
	return permission.Allowed(), err
}

// authorizeGovernedAnalysisSessionRead runs after the existing tenant/owner
// lookup. Returning the same not-found error preserves that boundary while
// withholding governed history from a member whose live access was revoked.
func authorizeGovernedAnalysisSessionRead(
	ctx context.Context,
	repo interfaces.MessageRepository,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
	sessionID string,
) error {
	governed, err := governedAnalysisSessionIDs(ctx, repo, []string{sessionID})
	if err != nil {
		return err
	}
	if !governed[sessionID] {
		return nil
	}
	allowed, err := currentMemberCanReadOperatingAnalysis(ctx, members, tenants)
	if err != nil {
		return err
	}
	if !allowed {
		return apperrors.ErrSessionNotFound
	}
	return nil
}
