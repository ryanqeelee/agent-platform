package dto

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantResponse_ViewerOmitsSecrets(t *testing.T) {
	tenant := sampleSecretTenant()
	body, err := json.Marshal(NewTenantResponse(viewerContext(), tenant))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "tenant-api-key-123")
	assert.NotContains(t, s, "legacy-search-secret-999")
	assert.NotContains(t, s, "wk-app-secret-def")
	assert.NotContains(t, s, "parser-secret-123")
	assert.NotContains(t, s, "minio-secret-789")
	assert.NotContains(t, s, "web_search_config")
	assert.NotContains(t, s, "parser_engine_config")
	assert.NotContains(t, s, "storage_engine_config")
	assert.NotContains(t, s, "chat_history_config")
	assert.NotContains(t, s, "credentials")
}

func TestTenantResponse_OwnerOmitsLegacyTenantAPIKey(t *testing.T) {
	tenant := sampleSecretTenant()
	body, err := json.Marshal(NewTenantResponse(ownerContext(), tenant))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, `"api_key"`)
	assert.NotContains(t, s, "legacy-search-secret-999")
	assert.NotContains(t, s, "parser-secret-123")
	assert.NotContains(t, s, "web_search_config")
	assert.NotContains(t, s, "retriever_engines")
	assert.NotContains(t, s, "retrieval_config")
}

func TestTenantResponse_AdminOmitsPlatformRuntimeConfigs(t *testing.T) {
	tenant := sampleSecretTenant()
	body, err := json.Marshal(NewTenantResponse(adminContext(), tenant))
	require.NoError(t, err)
	s := string(body)
	for _, field := range []string{
		"web_search_config", "parser_engine_config", "storage_engine_config",
		"credentials", "retriever_engines", "retrieval_config", "context_config",
		"chat_history_config",
	} {
		assert.NotContains(t, s, field)
	}
}

func TestTenantResponsesCrossTenant_RedactsEvenForOwnerContext(t *testing.T) {
	tenant := sampleSecretTenant()
	body, err := json.Marshal(NewTenantResponsesCrossTenant([]*types.Tenant{tenant}))
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "tenant-api-key-123")
	assert.NotContains(t, s, "parser-secret-123")
}

func sampleSecretTenant() *types.Tenant {
	return &types.Tenant{
		ID:   42,
		Name: "tenant",
		WebSearchConfig: &types.WebSearchConfig{
			APIKey:   "legacy-search-secret-999",
			ProxyURL: "http://proxy.internal:8080",
		},
		Credentials: &types.CredentialsConfig{
			WeKnoraCloud: &types.WeKnoraCloudCredentials{
				AppID:     "wk-app-id-abc",
				AppSecret: "wk-app-secret-def",
			},
		},
		ParserEngineConfig: &types.ParserEngineConfig{
			MinerUAPIKey:          "parser-secret-123",
			PaddleOCRVLCloudToken: "paddle-secret-456",
		},
		StorageEngineConfig: &types.StorageEngineConfig{
			DefaultProvider: "minio",
			MinIO: &types.MinIOEngineConfig{
				AccessKeyID:     "minio-access-id",
				SecretAccessKey: "minio-secret-789",
			},
		},
	}
}
