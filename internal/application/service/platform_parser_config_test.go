package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type parserConfigRepoStub struct {
	row *types.PlatformParserConfig
	err error
}

func (r *parserConfigRepoStub) Get(context.Context) (*types.PlatformParserConfig, error) {
	return r.row, r.err
}

func (r *parserConfigRepoStub) Upsert(
	_ context.Context, config *types.ParserEngineConfig, actorID string, now time.Time,
) (*types.PlatformParserConfig, error) {
	r.row = &types.PlatformParserConfig{
		ID: types.PlatformParserConfigSingletonID, Config: config,
		UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	return r.row, nil
}

func TestPlatformParserConfigServiceMissingRowUsesDocumentedDefaults(t *testing.T) {
	svc := NewPlatformParserConfigService(&parserConfigRepoStub{})
	cfg, err := svc.GetRuntime(context.Background())
	require.NoError(t, err)
	require.Empty(t, cfg.ToOverridesMap())
	require.Equal(t, types.DefaultParserEngine("pdf"), cfg.ResolveChatParserEngine("pdf"))
}

func TestPlatformParserConfigServiceUpdatePreservesMaskedSecretsAndRules(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	repo := &parserConfigRepoStub{row: &types.PlatformParserConfig{
		ID: types.PlatformParserConfigSingletonID,
		Config: &types.ParserEngineConfig{
			MinerUAPIKey:          "stored-secret",
			ChatParserEngineRules: []types.ParserEngineRule{{FileTypes: []string{"pdf"}, Engine: "mineru"}},
		},
	}}
	svc := NewPlatformParserConfigService(repo)
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-admin")
	response, err := svc.Update(ctx, &types.ParserEngineConfig{MinerUAPIKey: types.RedactedSecretPlaceholder})
	require.NoError(t, err)
	require.Equal(t, types.RedactedSecretPlaceholder, response.MinerUAPIKey)
	require.Equal(t, "stored-secret", repo.row.Config.MinerUAPIKey)
	require.Len(t, repo.row.Config.ChatParserEngineRules, 1)
	require.Equal(t, "platform-admin", repo.row.UpdatedBy)
}

func TestPlatformParserConfigServiceRejectsSecretsWithoutAESKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	svc := NewPlatformParserConfigService(&parserConfigRepoStub{})
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "platform-admin")
	_, err := svc.Update(ctx, &types.ParserEngineConfig{MinerUAPIKey: "secret"})
	require.ErrorContains(t, err, "SYSTEM_AES_KEY")
}

func TestPlatformParserConfigServiceRejectsMissingRepository(t *testing.T) {
	svc := NewPlatformParserConfigService(nil)
	_, err := svc.GetRuntime(context.Background())
	require.ErrorContains(t, err, "repository is not configured")
}

func TestKnowledgeParserOverridesUseSamePlatformConfigAcrossTenants(t *testing.T) {
	parserConfig := NewPlatformParserConfigService(&parserConfigRepoStub{row: &types.PlatformParserConfig{
		ID: types.PlatformParserConfigSingletonID,
		Config: &types.ParserEngineConfig{
			MinerUEndpoint: "https://shared-parser.example.invalid",
		},
	}})
	knowledge := &knowledgeService{parserConfig: parserConfig}
	ctxA := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(41))
	ctxB := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(42))

	overridesA, err := knowledge.getParserEngineOverrides(ctxA)
	require.NoError(t, err)
	overridesB, err := knowledge.getParserEngineOverrides(ctxB)
	require.NoError(t, err)
	require.Equal(t, overridesA, overridesB)
	require.Equal(t, "https://shared-parser.example.invalid", overridesA["mineru_endpoint"])
	tenantA, _ := types.TenantIDFromContext(ctxA)
	tenantB, _ := types.TenantIDFromContext(ctxB)
	require.Equal(t, uint64(41), tenantA)
	require.Equal(t, uint64(42), tenantB)
}

type temporaryDocumentReaderStub struct{}

func (temporaryDocumentReaderStub) Read(context.Context, *types.ReadRequest) (*types.ReadResult, error) {
	return nil, errors.New("unexpected read")
}

func (temporaryDocumentReaderStub) Reconnect(string) error { return nil }
func (temporaryDocumentReaderStub) IsConnected() bool      { return true }
func (temporaryDocumentReaderStub) ListEngines(context.Context, map[string]string) ([]types.ParserEngineInfo, error) {
	return []types.ParserEngineInfo{{Name: "custom", FileTypes: []string{"custom"}, Available: true}}, nil
}

func TestTemporaryDocumentCreatePropagatesPlatformParserConfigReadError(t *testing.T) {
	configErr := errors.New("parser config database unavailable")
	svc := &temporaryDocumentService{
		documentReader: temporaryDocumentReaderStub{},
		parserConfig:   NewPlatformParserConfigService(&parserConfigRepoStub{err: configErr}),
	}
	_, err := svc.Create(
		context.Background(), 41, "session-1", "attachment.custom", "application/octet-stream",
		1, strings.NewReader("x"), types.TemporaryDocumentCreateOptions{},
	)
	require.ErrorIs(t, err, configErr)
	require.ErrorContains(t, err, "load platform parser configuration")
}
