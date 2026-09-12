package middleware

import (
	"context"
	"strconv"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// BindSystemAdminTenantScope resolves the explicit :tenant_id of a platform
// control-plane request and exposes it on both context surfaces consumed by
// existing tenant-filtered handlers. RequireSystemAdmin must run before this
// middleware so ordinary users cannot use the lookup to probe tenant IDs.
//
// The tenant only needs to exist. Suspended and provisioning enterprises stay
// configurable by platform operators even though their business APIs cannot
// run. This request-local scope never changes the platform user's identity,
// tenant binding, or membership.
func BindSystemAdminTenantScope(tenantService interfaces.TenantService) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawTenantID := strings.TrimSpace(c.Param("tenant_id"))
		tenantID, err := strconv.ParseUint(rawTenantID, 10, 64)
		if err != nil || tenantID == 0 {
			_ = c.Error(apperrors.NewValidationError("workspace id must be a positive integer"))
			c.Abort()
			return
		}

		if rawHeader := strings.TrimSpace(c.GetHeader("X-Tenant-ID")); rawHeader != "" {
			headerTenantID, headerErr := strconv.ParseUint(rawHeader, 10, 64)
			if headerErr != nil || headerTenantID == 0 || headerTenantID != tenantID {
				_ = c.Error(apperrors.NewValidationError(
					"X-Tenant-ID must match the workspace id in the request path"))
				c.Abort()
				return
			}
		}

		if tenantService == nil {
			_ = c.Error(apperrors.NewInternalServerError("workspace service is unavailable"))
			c.Abort()
			return
		}
		tenant, lookupErr := tenantService.GetTenantByID(c.Request.Context(), tenantID)
		if lookupErr != nil || tenant == nil {
			logger.Warnf(c.Request.Context(),
				"[platform tenant scope] workspace lookup failed: tenant=%d err=%v", tenantID, lookupErr)
			_ = c.Error(apperrors.NewNotFoundError("workspace not found"))
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, tenantID)
		ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenant)
		c.Set(types.TenantIDContextKey.String(), tenantID)
		c.Set(types.TenantInfoContextKey.String(), tenant)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
