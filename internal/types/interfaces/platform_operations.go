package interfaces

import (
	"context"
	"io"

	"github.com/Tencent/WeKnora/internal/types"
)

// PlatformOperationsResponse preserves the Center operations response while
// keeping its service credential out of the browser-facing layer.
type PlatformOperationsResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

type PlatformOperationsBridge interface {
	Do(ctx context.Context, method, path, actorUserID, idempotencyKey string, body io.Reader) (*PlatformOperationsResponse, error)
}

type PlatformOperationsTenantRepository interface {
	UpdateForPlatformOperations(ctx context.Context, actorUserID string, id uint64, name, description, status string, seatsTotal *int, storageQuota int64) (*types.Tenant, int64, error)
}

type PlatformOperationsIdentityRepository interface {
	CreateInitialAdministrator(ctx context.Context, actorUserID, commandID, requestSHA256, password string, user *types.User) (*types.PlatformInitialAdministratorReceipt, *types.User, bool, error)
	GetInitialAdministrator(ctx context.Context, actorUserID, commandID string) (*types.PlatformInitialAdministratorReceipt, *types.User, error)
	ResetEnterpriseMemberPassword(ctx context.Context, actorUserID string, tenantID uint64, targetUserID, passwordHash string) error
}

type PlatformOperationsIdentityService interface {
	CreateInitialAdministrator(ctx context.Context, actorUserID, commandID string, req *types.AdminCreateUserRequest) (*types.PlatformInitialAdministratorResult, error)
	GetInitialAdministrator(ctx context.Context, actorUserID, commandID string) (*types.PlatformInitialAdministratorResult, error)
	ResetEnterpriseMemberPassword(ctx context.Context, actorUserID string, tenantID uint64, targetUserID, newPassword string) error
}
