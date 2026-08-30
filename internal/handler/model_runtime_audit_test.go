package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type captureModelRuntimeAudit struct {
	interfaces.AuditLogService
	entry *types.AuditLog
}

func (a *captureModelRuntimeAudit) Log(_ context.Context, entry *types.AuditLog) error {
	a.entry = entry
	return nil
}

func TestModelRuntimeAuditRecordsScopeRevisionAndOnlyCredentialFieldNames(t *testing.T) {
	audit := &captureModelRuntimeAudit{}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-operator")
	model := &types.Model{ID: "model-1", UpdatedAt: time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)}

	emitPlatformConfigAudit(ctx, audit, types.AuditActionSystemModelRuntimeChanged,
		"credentials_updated", "model", model.ID, modelRuntimeScope(model), modelRuntimeRevision(model), []string{"api_key"})

	require.NotNil(t, audit.entry)
	require.Equal(t, "platform-operator", audit.entry.ActorUserID)
	require.Equal(t, "enterprise_assigned", modelRuntimeScope(model))
	require.Contains(t, string(audit.entry.Details), `"version":"2026-08-30T01:02:03Z"`)
	require.Contains(t, string(audit.entry.Details), `"changed_fields":["api_key"]`)
}

func TestRetrievalProcessingAuditRecordsScopeRevisionAndFieldNames(t *testing.T) {
	audit := &captureModelRuntimeAudit{}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-operator")

	emitPlatformConfigAudit(ctx, audit, types.AuditActionSystemRetrievalProcessingChanged,
		"update", "vector_store", "store-1", "enterprise_assigned", "2026-08-30T01:02:03Z", []string{"connection_config"})

	require.NotNil(t, audit.entry)
	require.Equal(t, types.AuditActionSystemRetrievalProcessingChanged, audit.entry.Action)
	require.Equal(t, "vector_store", audit.entry.TargetType)
	require.Contains(t, string(audit.entry.Details), `"changed_fields":["connection_config"]`)
}
