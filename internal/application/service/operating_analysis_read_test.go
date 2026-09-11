package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type operatingReadMembers struct {
	interfaces.TenantMemberService
	member *types.TenantMember
}

func (s operatingReadMembers) GetMembership(context.Context, string, uint64) (*types.TenantMember, error) {
	return s.member, nil
}

func operatingReadContext(userID string, tenantID uint64) context.Context {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, userID)
	ctx = context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	return types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: userID})
}

func newOperatingReadFixture(t *testing.T, allowed bool) (*sessionService, *messageService, context.Context, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Session{}, &types.Message{}))
	members := operatingReadMembers{member: &types.TenantMember{
		UserID: "employee", TenantID: 1, Status: types.TenantMemberStatusActive,
		OperatingAnalysisAccess: allowed,
	}}
	sessions := repository.NewSessionRepository(db)
	messages := repository.NewMessageRepository(db)
	return &sessionService{sessionRepo: sessions, messageRepo: messages, tenantMemberService: members},
		&messageService{sessionRepo: sessions, messageRepo: messages, tenantMemberService: members},
		operatingReadContext("employee", 1), db
}

func TestGovernedHistoryReadTracksLiveMemberAccess(t *testing.T) {
	sessionSvc, messageSvc, ctx, db := newOperatingReadFixture(t, false)
	governed := &types.Session{ID: "governed", TenantID: 1, UserID: "employee"}
	ordinary := &types.Session{ID: "ordinary", TenantID: 1, UserID: "employee"}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create([]*types.Session{governed, ordinary}).Error)
	governedAnswer := &types.Message{
		ID: "answer", SessionID: governed.ID, Role: "assistant", Content: "secret",
		AgentID: types.BuiltinOperatingAnalystID,
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(governedAnswer).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&types.Message{
		ID: "csv-answer", SessionID: ordinary.ID, Role: "assistant", Content: "csv result",
		AgentID: types.BuiltinEmployeeAssistantID,
	}).Error)

	_, err := sessionSvc.GetSession(ctx, governed.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)
	_, err = messageSvc.GetMessagesBySession(ctx, governed.ID, 1, 20)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)
	_, err = messageSvc.GetMessageForRead(ctx, governed.ID, governedAnswer.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)

	got, err := messageSvc.GetMessagesBySession(ctx, ordinary.ID, 1, 20)
	require.NoError(t, err)
	require.Len(t, got, 1)

	sessionSvc.tenantMemberService = operatingReadMembers{member: &types.TenantMember{
		UserID: "employee", TenantID: 1, Status: types.TenantMemberStatusActive,
		OperatingAnalysisAccess: true,
	}}
	messageSvc.tenantMemberService = sessionSvc.tenantMemberService
	got, err = messageSvc.GetMessagesBySession(ctx, governed.ID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, "secret", got[0].Content)
}

func TestGovernedHistoryClassifiesPersistedToolEvidenceAndPreservesOwnerBoundary(t *testing.T) {
	sessionSvc, messageSvc, ctx, db := newOperatingReadFixture(t, true)
	toolRun := &types.Session{ID: "tool-run", TenantID: 1, UserID: "employee"}
	ordinary := &types.Session{ID: "ordinary", TenantID: 1, UserID: "employee"}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create([]*types.Session{toolRun, ordinary}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&types.Message{
		ID: "answer", SessionID: toolRun.ID, Role: "assistant",
		AgentSteps: types.AgentSteps{{ToolCalls: []types.ToolCall{{Name: "governed_data_query"}}}},
	}).Error)
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create([]*types.Message{
		{ID: "follow-up-artifact", SessionID: toolRun.ID, Role: "assistant", AgentID: types.BuiltinEmployeeAssistantID},
		{ID: "ordinary-artifact", SessionID: ordinary.ID, Role: "assistant", AgentID: types.BuiltinEmployeeAssistantID},
	}).Error)

	governed, err := messageSvc.IsGovernedAnalysisMessage(ctx, "follow-up-artifact")
	require.NoError(t, err)
	require.True(t, governed)
	governed, err = messageSvc.IsGovernedAnalysisMessage(ctx, "ordinary-artifact")
	require.NoError(t, err)
	require.False(t, governed)

	_, err = sessionSvc.GetSession(operatingReadContext("other", 1), toolRun.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)
	_, err = sessionSvc.GetSession(ctx, toolRun.ID)
	require.NoError(t, err)

	sessionSvc.tenantMemberService = operatingReadMembers{member: &types.TenantMember{
		UserID: "employee", TenantID: 1, Status: types.TenantMemberStatusActive,
		OperatingAnalysisAccess: false,
	}}
	_, err = sessionSvc.GetSession(ctx, toolRun.ID)
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)
}

func TestGovernedArtifactReadUsesPersistedArtifactWithoutLifecycleBinding(t *testing.T) {
	_, messageSvc, ctx, db := newOperatingReadFixture(t, true)
	ref := "resource://AbCdEfGhIjKlMnOpQrStUv"
	sessions := []*types.Session{{ID: "business", TenantID: 1, UserID: "employee"}, {ID: "ordinary", TenantID: 1, UserID: "employee"}}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(sessions).Error)
	messages := []*types.Message{
		{ID: "evidence", SessionID: "business", Role: "assistant", AgentID: types.BuiltinOperatingAnalystID},
		{ID: "unbound", SessionID: "business", Role: "assistant", Artifacts: types.MessageArtifacts{{URL: ref}}},
		{ID: "ordinary", SessionID: "ordinary", Role: "assistant", Artifacts: types.MessageArtifacts{{URL: "resource://ordinary12345678901234"}}},
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(messages).Error)
	ids, err := messageSvc.GovernedArtifactMessageIDs(ctx, []string{ref})
	require.NoError(t, err)
	require.Equal(t, []string{"unbound"}, ids)
	ids, err = messageSvc.GovernedArtifactMessageIDs(ctx, []string{"resource://ordinary12345678901234"})
	require.NoError(t, err)
	require.Empty(t, ids)
	_, err = messageSvc.GetMessageForRead(ctx, "business", "unbound")
	require.NoError(t, err)
	messageSvc.tenantMemberService = operatingReadMembers{member: &types.TenantMember{UserID: "employee", TenantID: 1, Status: types.TenantMemberStatusActive, OperatingAnalysisAccess: false}}
	_, err = messageSvc.GetMessageForRead(ctx, "business", "unbound")
	require.ErrorIs(t, err, apperrors.ErrSessionNotFound)
	require.NoError(t, db.Delete(&types.Message{}, "session_id = ?", "business").Error)
	ids, err = messageSvc.GovernedArtifactMessageIDs(ctx, []string{ref})
	require.NoError(t, err)
	require.Equal(t, []string{"unbound"}, ids)
	_, err = messageSvc.GetMessageForRead(ctx, "business", "unbound")
	require.Error(t, err)
}
