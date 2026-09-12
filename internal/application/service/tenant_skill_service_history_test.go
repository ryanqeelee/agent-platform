package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

type transcriptHistoryFixture struct {
	db      *gorm.DB
	service *TenantSkillService
}

func newTranscriptHistoryFixture(t *testing.T) *transcriptHistoryFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantSkillEntity{}, &types.Message{}))
	return &transcriptHistoryFixture{
		db: db,
		service: &TenantSkillService{
			skills:   repository.NewTenantSkillRepository(db),
			messages: repository.NewMessageRepository(db),
		},
	}
}

func (f *transcriptHistoryFixture) seedSkill(t *testing.T, skill *types.TenantSkillEntity) {
	t.Helper()
	require.NoError(t, f.db.Session(&gorm.Session{SkipHooks: true}).Create(skill).Error)
}

func (f *transcriptHistoryFixture) seedMessages(t *testing.T, messages ...*types.Message) {
	t.Helper()
	for _, message := range messages {
		require.NoError(t, f.db.Session(&gorm.Session{SkipHooks: true}).Create(message).Error)
	}
}

func requireTranscriptHistoryNotFound(t *testing.T, err error) {
	t.Helper()
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok, "error=%v", err)
	require.Equal(t, 404, appErr.HTTPCode)
}

func TestGetInstallTranscriptHistoryReturnsOnlyLocatedInstallMessages(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	now := time.Now()
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "skill-1", TenantID: 42, SandboxConfigID: "cfg-a", Name: "pdf",
		Status: types.SkillStatusReady, InstallSessionID: "session-1", InstallMessageID: "answer-1",
	})
	fixture.seedMessages(t,
		&types.Message{ID: "prompt-1", SessionID: "session-1", Role: "user", Content: "install pdf", CreatedAt: now},
		&types.Message{ID: "answer-1", SessionID: "session-1", Role: "assistant", Content: "installed", CreatedAt: now.Add(time.Millisecond)},
		&types.Message{ID: "other-answer", SessionID: "session-1", Role: "assistant", Content: "must not leak", CreatedAt: now.Add(2 * time.Millisecond)},
		&types.Message{ID: "other-session", SessionID: "session-2", Role: "assistant", Content: "other tenant data", CreatedAt: now},
	)

	history, err := fixture.service.GetInstallTranscriptHistory(
		context.Background(), 42, "cfg-a", "skill-1",
	)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, []string{"prompt-1", "answer-1"}, []string{history[0].ID, history[1].ID})
	require.Equal(t, []string{"user", "assistant"}, []string{history[0].Role, history[1].Role})
}

func TestGetInstallTranscriptHistoryEnforcesTenantConfigAndSkillScope(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "skill-1", TenantID: 42, SandboxConfigID: "cfg-a", Name: "pdf",
		Status: types.SkillStatusReady, InstallSessionID: "session-1", InstallMessageID: "answer-1",
	})

	for _, call := range []struct {
		name              string
		tenantID          uint64
		configID, skillID string
	}{
		{name: "other tenant", tenantID: 41, configID: "cfg-a", skillID: "skill-1"},
		{name: "other config", tenantID: 42, configID: "cfg-b", skillID: "skill-1"},
		{name: "other skill", tenantID: 42, configID: "cfg-a", skillID: "skill-2"},
	} {
		t.Run(call.name, func(t *testing.T) {
			_, err := fixture.service.GetInstallTranscriptHistory(
				context.Background(), call.tenantID, call.configID, call.skillID,
			)
			requireTranscriptHistoryNotFound(t, err)
		})
	}
}

func TestGetInstallTranscriptHistoryLifecycleWithoutDurableRows(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "installing", TenantID: 42, SandboxConfigID: "cfg-a", Name: "live",
		Status: types.SkillStatusInstalling,
	})
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "ready-no-locator", TenantID: 42, SandboxConfigID: "cfg-a", Name: "old",
		Status: types.SkillStatusReady,
	})
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "ready-no-message", TenantID: 42, SandboxConfigID: "cfg-a", Name: "missing",
		Status: types.SkillStatusReady, InstallSessionID: "gone", InstallMessageID: "gone",
	})

	history, err := fixture.service.GetInstallTranscriptHistory(
		context.Background(), 42, "cfg-a", "installing",
	)
	require.NoError(t, err)
	require.NotNil(t, history)
	require.Empty(t, history)

	_, err = fixture.service.GetInstallTranscriptHistory(
		context.Background(), 42, "cfg-a", "ready-no-locator",
	)
	requireTranscriptHistoryNotFound(t, err)

	_, err = fixture.service.GetInstallTranscriptHistory(
		context.Background(), 42, "cfg-a", "ready-no-message",
	)
	requireTranscriptHistoryNotFound(t, err)
}
