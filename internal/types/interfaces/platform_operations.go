package interfaces

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

type PlatformOperationsUpstreamError struct {
	StatusCode int
	Detail     string
}

func (e *PlatformOperationsUpstreamError) Error() string {
	return fmt.Sprintf("Center operations returned %d: %s", e.StatusCode, e.Detail)
}

type CenterEnterpriseEdgeSummary struct {
	ConnectionStatus string  `json:"connectionStatus"`
	NodeCount        int     `json:"nodeCount"`
	OnlineNodeCount  int     `json:"onlineNodeCount"`
	LastSeenAt       *string `json:"lastSeenAt"`
}

type CenterEnterpriseEdgeNode struct {
	EdgeNodeID        string         `json:"edgeNodeId"`
	DisplayName       string         `json:"displayName"`
	Version           string         `json:"version"`
	Status            string         `json:"status"`
	CatalogVersion    *string        `json:"catalogVersion"`
	DataServiceStatus map[string]any `json:"dataServiceStatus"`
	RegisteredAt      *string        `json:"registeredAt"`
	LastSeenAt        *string        `json:"lastSeenAt"`
	ControlRevision   int64          `json:"controlRevision"`
}

type CenterEnterpriseEdge struct {
	Schema       string                      `json:"schema"`
	EnterpriseID string                      `json:"enterpriseId"`
	Summary      CenterEnterpriseEdgeSummary `json:"summary"`
	Nodes        []CenterEnterpriseEdgeNode  `json:"nodes"`
}

type CenterEnterpriseEnrollment struct {
	Schema          string  `json:"schema"`
	EnterpriseID    string  `json:"enterpriseId"`
	EnrollmentToken string  `json:"enrollmentToken"`
	RotatedAt       *string `json:"rotatedAt"`
}

type CenterEdgeNodeDisable struct {
	Schema          string `json:"schema"`
	EnterpriseID    string `json:"enterpriseId"`
	EdgeNodeID      string `json:"edgeNodeId"`
	Status          string `json:"status"`
	ControlRevision int64  `json:"controlRevision"`
}

type PlatformOperationsBridge interface {
	DisableEnterpriseEdgeNode(ctx context.Context, enterpriseID, edgeNodeID, actorUserID string) (*CenterEdgeNodeDisable, error)
	GetEnterpriseEdge(ctx context.Context, enterpriseID, actorUserID string) (*CenterEnterpriseEdge, error)
	RotateEnterpriseEnrollmentToken(ctx context.Context, enterpriseID, actorUserID string) (*CenterEnterpriseEnrollment, error)
}

type PlatformOperationsTenantRepository interface {
	ValidateSystemAdministrator(ctx context.Context, actorUserID string) error
	UpdateForPlatformOperations(ctx context.Context, actorUserID string, id uint64, name, description, status string, analysisEnabled bool, seatsTotal *int, storageQuota int64) (*types.Tenant, int64, error)
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
