package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func modelRuntimeScope(model *types.Model) string {
	if model != nil && !model.IsBuiltin {
		return "enterprise_assigned"
	}
	return "platform_shared"
}

func modelRuntimeRevision(model *types.Model) string {
	if model == nil {
		return ""
	}
	revision := model.UpdatedAt
	if revision.IsZero() {
		revision = model.CreatedAt
	}
	if revision.IsZero() {
		return ""
	}
	return revision.UTC().Format(time.RFC3339Nano)
}

func emitModelRuntimeAudit(
	ctx context.Context,
	audit interfaces.AuditLogService,
	action types.AuditAction,
	operation, targetType, targetID, scope, revision string,
	changedFields []string,
) {
	if audit == nil {
		return
	}
	actorID, _ := types.UserIDFromContext(ctx)
	details, _ := json.Marshal(map[string]any{
		"operation":      operation,
		"scope":          scope,
		"version":        revision,
		"changed_fields": changedFields,
	})
	_ = audit.Log(ctx, &types.AuditLog{
		TenantID: 0, ActorUserID: actorID, ActorRole: systemAuditActorRole(ctx),
		Action: action, TargetType: targetType, TargetID: targetID,
		Outcome: types.AuditOutcomeSuccess, Details: types.JSON(details),
	})
}
