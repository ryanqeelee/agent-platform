package dto

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// TenantResponse is the viewer-safe tenant profile shape. Secret-bearing
// columns are omitted or redacted unless the caller has Admin+.
type TenantResponse struct {
	ID           uint64         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	Business     string         `json:"business"`
	StorageQuota int64          `json:"storage_quota"`
	StorageUsed  int64          `json:"storage_used"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at"`
}

// NewTenantResponse converts a stored tenant into its HTTP response shape.
func NewTenantResponse(ctx context.Context, tenant *types.Tenant) *TenantResponse {
	return NewTenantResponseWithRole(tenant, RoleFromContext(ctx))
}

// NewTenantResponseWithRole converts a stored tenant using an explicit role
// (for auth flows where tenant role is not yet in request context).
func NewTenantResponseWithRole(tenant *types.Tenant, role types.TenantRole) *TenantResponse {
	if tenant == nil {
		return nil
	}
	resp := &TenantResponse{
		ID:           tenant.ID,
		Name:         tenant.Name,
		Description:  tenant.Description,
		Status:       tenant.Status,
		Business:     tenant.Business,
		StorageQuota: tenant.StorageQuota,
		StorageUsed:  tenant.StorageUsed,
		CreatedAt:    tenant.CreatedAt,
		UpdatedAt:    tenant.UpdatedAt,
		DeletedAt:    tenant.DeletedAt,
	}
	_ = role
	return resp
}

// NewTenantResponses is the slice convenience wrapper.
func NewTenantResponses(ctx context.Context, tenants []*types.Tenant) []*TenantResponse {
	out := make([]*TenantResponse, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, NewTenantResponse(ctx, t))
	}
	return out
}

// NewTenantResponsesCrossTenant redacts every tenant as Viewer regardless of
// the caller's active-tenant role. Used by cross-tenant list/search endpoints
// where the caller's home-tenant role must not unlock other tenants' secrets.
func NewTenantResponsesCrossTenant(tenants []*types.Tenant) []*TenantResponse {
	out := make([]*TenantResponse, 0, len(tenants))
	for _, t := range tenants {
		out = append(out, NewTenantResponseWithRole(t, types.TenantRoleViewer))
	}
	return out
}
