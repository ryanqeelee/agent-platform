package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KnowledgeGovernanceService is the single employee-knowledge authorization
// authority. Repository/system paths without a human principal remain
// deliberately outside this interface.
type KnowledgeGovernanceService interface {
	ListBusinessRoles(ctx context.Context, tenantID uint64) ([]*types.BusinessRole, error)
	CreateBusinessRole(ctx context.Context, tenantID uint64, name string) (*types.BusinessRole, error)
	UpdateBusinessRole(ctx context.Context, tenantID uint64, id, name string, enabled bool) (*types.BusinessRole, error)
	ReplaceMemberBusinessRoles(ctx context.Context, tenantID uint64, userID string, roleIDs []string) error
	ListMemberBusinessRoleIDs(ctx context.Context, tenantID uint64, userID string) ([]string, error)
	GetKnowledgeBaseRoleGrants(ctx context.Context, tenantID uint64, kbID string) ([]string, error)
	ReplaceKnowledgeBaseRoleGrants(ctx context.Context, tenantID uint64, kbID string, mode string, roleIDs []string) error
	CanAccessKnowledgeBase(ctx context.Context, tenantID uint64, userID string, tenantRole types.TenantRole, kbID string) (bool, error)
	FilterKnowledgeBases(ctx context.Context, tenantID uint64, userID string, tenantRole types.TenantRole, kbs []*types.KnowledgeBase) ([]*types.KnowledgeBase, error)
}
