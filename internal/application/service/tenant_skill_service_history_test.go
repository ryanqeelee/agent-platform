package service

import (
	"context"
	"encoding/json"
	"testing"

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
	require.NoError(t, db.AutoMigrate(&types.TenantSkillEntity{}))
	return &transcriptHistoryFixture{
		db: db,
		service: &TenantSkillService{
			skills: repository.NewTenantSkillRepository(db),
		},
	}
}

func (f *transcriptHistoryFixture) seedSkill(t *testing.T, skill *types.TenantSkillEntity) {
	t.Helper()
	require.NoError(t, f.db.Session(&gorm.Session{SkipHooks: true}).Create(skill).Error)
}

func historyTranscriptMessages(t *testing.T, messages ...*types.Message) types.JSON {
	t.Helper()
	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	return types.JSON(raw)
}

func requireTranscriptHistoryNotFound(t *testing.T, err error) {
	t.Helper()
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok, "error=%v", err)
	require.Equal(t, 404, appErr.HTTPCode)
}

func TestGetInstallTranscriptHistoryReturnsMessageDTOs(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "skill-1", SandboxConfigID: "cfg-a", Name: "pdf",
		Status: types.SkillStatusReady, InstallRunID: "run-1",
		InstallTranscript: historyTranscriptMessages(t,
			&types.Message{ID: "prompt-1", SessionID: "run-1", Role: "user", Content: "install pdf"},
			&types.Message{ID: "answer-1", SessionID: "run-1", Role: "assistant", Content: "installed"},
		),
	})

	history, err := fixture.service.GetInstallTranscriptHistory(
		context.Background(), "cfg-a", "skill-1",
	)
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, []string{"prompt-1", "answer-1"}, []string{history[0].ID, history[1].ID})
}

func TestGetInstallTranscriptHistoryEnforcesConfigAndSkillScope(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "skill-1", SandboxConfigID: "cfg-a", Name: "pdf",
		Status: types.SkillStatusReady, InstallRunID: "run-1",
		InstallTranscript: historyTranscriptMessages(t,
			&types.Message{ID: "prompt-1", Role: "user"},
			&types.Message{ID: "answer-1", Role: "assistant"},
		),
	})

	for _, call := range []struct{ configID, skillID string }{
		{configID: "cfg-b", skillID: "skill-1"},
		{configID: "cfg-a", skillID: "skill-2"},
	} {
		_, err := fixture.service.GetInstallTranscriptHistory(
			context.Background(), call.configID, call.skillID,
		)
		requireTranscriptHistoryNotFound(t, err)
	}
}

func TestGetInstallTranscriptHistoryLifecycleWithoutDurableRun(t *testing.T) {
	fixture := newTranscriptHistoryFixture(t)
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "installing", SandboxConfigID: "cfg-a", Name: "live",
		Status: types.SkillStatusInstalling,
	})
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "ready-no-locator", SandboxConfigID: "cfg-a", Name: "old",
		Status: types.SkillStatusReady,
	})
	fixture.seedSkill(t, &types.TenantSkillEntity{
		ID: "ready-no-run", SandboxConfigID: "cfg-a", Name: "missing",
		Status: types.SkillStatusReady, InstallRunID: "gone",
	})

	history, err := fixture.service.GetInstallTranscriptHistory(
		context.Background(), "cfg-a", "installing",
	)
	require.NoError(t, err)
	require.NotNil(t, history)
	require.Empty(t, history)

	_, err = fixture.service.GetInstallTranscriptHistory(
		context.Background(), "cfg-a", "ready-no-locator",
	)
	requireTranscriptHistoryNotFound(t, err)

	_, err = fixture.service.GetInstallTranscriptHistory(
		context.Background(), "cfg-a", "ready-no-run",
	)
	requireTranscriptHistoryNotFound(t, err)
}
