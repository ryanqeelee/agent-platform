package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// These actual service outputs are also consumed by the Python contract check.
func TestPersonalMemoryWireFixtures(t *testing.T) {
	svc, db, tenants := newMemoryHarness(t)
	ctx := enabledCtx(t, tenants, 1, "alice")
	initial, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	importance := 3
	command := commandForSnapshot("wire-command-001", initial, types.PersonalMemoryChange{
		Op: types.MemoryChangeCreate, Scope: types.MemoryScopeShared,
		Kind: types.MemoryKindPreference, Topic: "output", Content: "Prefer concise answers", Importance: &importance,
	})
	command.Source.Runtime = types.MemoryConsumerAnalysis
	receipt, err := svc.ApplyCommand(ctx, command)
	require.NoError(t, err)
	require.Equal(t, types.MemoryReceiptApplied, receipt.Status)
	after, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	require.Len(t, after.Items, 1)
	// Legacy/UI items may have no topic. The shared wire must preserve that.
	require.NoError(t, db.Model(&types.MemoryItem{}).Where("id = ?", after.Items[0].ID).Update("topic", "").Error)
	blankTopic, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	require.Empty(t, blankTopic.Items[0].Topic)
	require.NoError(t, svc.SetEnabled(ctx, false))
	disabled, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	expression := &types.PersonalMemoryExpression{
		Schema: types.PersonalMemoryExpressionSchema, ExpressionID: "wire-expression-001",
		Runtime: types.MemoryConsumerAnalysis, SessionID: "wire-session", MessageID: "wire-message",
		Text: strings.Repeat("context ", 200) + "This was a quotation, not my preference.",
		ExpectedPolicy: &types.PersonalMemoryExpressionExpectedPolicy{
			WorkspaceGeneration: disabled.Policy.WorkspaceGeneration, SubjectGeneration: disabled.Policy.SubjectGeneration,
		},
	}
	rejected, err := svc.SubmitExpression(ctx, expression)
	require.NoError(t, err)
	require.Equal(t, "rejected", rejected.Status)
	replayed, err := svc.SubmitExpression(ctx, expression)
	require.NoError(t, err)
	require.Equal(t, "replayed", replayed.Status)
	fixtures := map[string]interface{}{
		"initial": initial, "command": command, "receipt": receipt, "snapshot": after,
		"blank_topic": blankTopic, "disabled": disabled, "expression": expression,
		"expression_rejected": rejected, "expression_replayed": replayed,
	}
	encoded, err := json.Marshal(fixtures)
	require.NoError(t, err)
	t.Logf("WIRE_FIXTURES=%s", encoded)
}

func TestSnapshotProjectionHasSharedWireLimit(t *testing.T) {
	svc, db, tenants := newMemoryHarness(t)
	ctx := enabledCtx(t, tenants, 1, "alice")
	for i := 0; i < 51; i++ {
		require.NoError(t, db.Create(&types.MemoryItem{
			ID: fmt.Sprintf("wire-item-%03d", i), TenantID: 1, SubjectID: "web_user:alice",
			Scope: types.MemoryScopeShared, Kind: types.MemoryKindPreference,
			Content: fmt.Sprintf("preference %03d", i), Importance: 3,
			Status: types.MemoryStatusActive, Origin: types.MemoryOriginManual,
		}).Error)
	}
	snapshot, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 50)
}

func TestAcceptedExpressionPreservesTrailingQualification(t *testing.T) {
	svc, db, tenants := newMemoryHarness(t)
	ctx := enabledCtx(t, tenants, 1, "alice")
	svc.enqueuer = nil
	text := strings.Repeat("quoted example ", 100) + "This was a quotation, not my preference."
	expression := &types.PersonalMemoryExpression{
		Schema: types.PersonalMemoryExpressionSchema, ExpressionID: "whole-expression-001",
		Runtime: types.MemoryConsumerAnalysis, SessionID: "wire-session", MessageID: "wire-message",
		Text: text, ExpectedPolicy: &types.PersonalMemoryExpressionExpectedPolicy{},
	}
	receipt, err := svc.SubmitExpression(ctx, expression)
	require.NoError(t, err)
	require.Equal(t, "accepted", receipt.Status)
	var stored types.MemoryExpression
	require.NoError(t, db.Where("expression_id = ?", expression.ExpressionID).First(&stored).Error)
	require.NotNil(t, stored.Text)
	require.Equal(t, text, *stored.Text)
	expression.ExpressionID = "overlong-expression-001"
	expression.Text = strings.Repeat("a", 32001)
	_, err = svc.SubmitExpression(ctx, expression)
	require.ErrorIs(t, err, ErrInvalidMemoryContract)
}

func TestCommandRejectsLossyItemInput(t *testing.T) {
	svc, _, tenants := newMemoryHarness(t)
	ctx := enabledCtx(t, tenants, 1, "alice")
	snapshot, err := svc.Snapshot(ctx, types.MemoryConsumerAnalysis)
	require.NoError(t, err)
	for _, change := range []types.PersonalMemoryChange{
		{Op: types.MemoryChangeCreate, Kind: types.MemoryKindPreference, Content: strings.Repeat("a", 301)},
		{Op: types.MemoryChangeCreate, Kind: types.MemoryKindPreference, Content: "concise", Topic: strings.Repeat("a", 81)},
		{Op: types.MemoryChangeDelete, ID: "item-1", Content: "silently ignored field"},
	} {
		_, err := svc.ApplyCommand(ctx, commandForSnapshot("invalid-input-001", snapshot, change))
		require.ErrorIs(t, err, ErrInvalidMemoryContract)
	}
}
