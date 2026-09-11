package service

import (
	"context"
	"errors"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
)

// CreateEnterpriseEmployee uses the existing password policy and membership authority.
// It never reuses an existing identity or returns credentials.
func (s *userService) CreateEnterpriseEmployee(ctx context.Context, tenantID uint64, req *types.RegisterRequest) (*types.User, *types.TenantMember, error) {
	if err := ValidatePasswordPolicy(req.Password, s.complexPasswordEnabled(ctx)); err != nil {
		return nil, nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, err
	}
	user := &types.User{ID: uuid.NewString(), Username: strings.TrimSpace(req.Username), Email: strings.TrimSpace(req.Email), PasswordHash: string(hash), TenantID: tenantID, IsActive: true}
	member, err := s.memberService.CreateEmployee(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, member, nil
}

func (s *tenantMemberService) CreateEmployee(ctx context.Context, user *types.User) (*types.TenantMember, error) {
	actorID, _ := types.UserIDFromContext(ctx)
	actor := managedActor(ctx)
	member := &types.TenantMember{UserID: user.ID, TenantID: user.TenantID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive, InvitedBy: &actorID, JoinedAt: time.Now()}
	if err := s.repo.CreateEmployee(ctx, actor, user, member); err != nil {
		if errors.Is(err, apprepo.ErrSeatLimitExceeded) {
			return nil, ErrSeatLimitExceeded
		}
		if errors.Is(err, apprepo.ErrEnterpriseNotActive) {
			return nil, ErrEnterpriseNotActive
		}
		if isDuplicateMembership(err) {
			return nil, ErrUserIdentityConflict
		}
		if errors.Is(err, apprepo.ErrMemberActionForbidden) {
			return nil, ErrMemberActionForbidden
		}
		return nil, err
	}
	s.emitAudit(ctx, &types.AuditLog{TenantID: user.TenantID, ActorUserID: actorID, ActorRole: auditActorRole(ctx), Action: types.AuditActionMemberAdded, TargetType: "tenant_member", TargetUserID: user.ID, Outcome: types.AuditOutcomeSuccess})
	return member, nil
}
