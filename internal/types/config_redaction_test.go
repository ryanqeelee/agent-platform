package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeParserEngineConfigForUpdate_PreservesRedactedSecrets(t *testing.T) {
	existing := &ParserEngineConfig{
		MinerUAPIKey:          "mineru-secret",
		PaddleOCRVLCloudToken: "paddle-secret",
		MinerUEndpoint:        "http://mineru",
		ChatParserEngineRules: []ParserEngineRule{{FileTypes: []string{"pdf"}, Engine: "mineru"}},
	}
	incoming := &ParserEngineConfig{
		MinerUAPIKey:          RedactedSecretPlaceholder,
		PaddleOCRVLCloudToken: RedactedSecretPlaceholder,
		MinerUEndpoint:        "http://mineru-new",
	}
	merged := MergeParserEngineConfigForUpdate(incoming, existing)
	require.NotNil(t, merged)
	assert.Equal(t, "mineru-secret", merged.MinerUAPIKey)
	assert.Equal(t, "paddle-secret", merged.PaddleOCRVLCloudToken)
	assert.Equal(t, "http://mineru-new", merged.MinerUEndpoint)
	assert.Equal(t, existing.ChatParserEngineRules, merged.ChatParserEngineRules)
}

func TestMergeParserEngineConfigForUpdate_ExplicitlyClearsGlobalChatRules(t *testing.T) {
	existing := &ParserEngineConfig{
		ChatParserEngineRules: []ParserEngineRule{{FileTypes: []string{"pdf"}, Engine: "mineru"}},
	}
	merged := MergeParserEngineConfigForUpdate(&ParserEngineConfig{ChatParserEngineRules: []ParserEngineRule{}}, existing)
	require.NotNil(t, merged)
	assert.NotNil(t, merged.ChatParserEngineRules)
	assert.Empty(t, merged.ChatParserEngineRules)
}

func TestParserEngineConfigForResponse_MasksSecrets(t *testing.T) {
	cfg := &ParserEngineConfig{MinerUAPIKey: "mineru-secret", PaddleOCRVLCloudToken: "paddle-secret"}
	resp := ParserEngineConfigForResponse(cfg, true)
	require.NotNil(t, resp)
	assert.Equal(t, RedactedSecretPlaceholder, resp.MinerUAPIKey)
	assert.Equal(t, RedactedSecretPlaceholder, resp.PaddleOCRVLCloudToken)
	assert.Equal(t, "mineru-secret", cfg.MinerUAPIKey)
}

func TestMergeStorageEngineConfigForUpdate_PreservesRedactedSecrets(t *testing.T) {
	existing := &StorageEngineConfig{
		DefaultProvider: "minio",
		MinIO: &MinIOEngineConfig{
			AccessKeyID:     "access-id",
			SecretAccessKey: "secret-key",
			BucketName:      "bucket",
		},
	}
	incoming := &StorageEngineConfig{
		DefaultProvider: "minio",
		MinIO: &MinIOEngineConfig{
			AccessKeyID:     RedactedSecretPlaceholder,
			SecretAccessKey: RedactedSecretPlaceholder,
			BucketName:      "bucket-new",
		},
	}
	merged := MergeStorageEngineConfigForUpdate(incoming, existing)
	require.NotNil(t, merged)
	require.NotNil(t, merged.MinIO)
	assert.Equal(t, "access-id", merged.MinIO.AccessKeyID)
	assert.Equal(t, "secret-key", merged.MinIO.SecretAccessKey)
	assert.Equal(t, "bucket-new", merged.MinIO.BucketName)
}

func TestMergeStorageEngineConfigForUpdate_ClearsS3Credentials(t *testing.T) {
	existing := &StorageEngineConfig{
		DefaultProvider: "s3",
		S3: &S3EngineConfig{
			AccessKey:  "stored-access-key",
			SecretKey:  "stored-secret-key",
			Region:     "us-east-1",
			BucketName: "bucket",
		},
	}

	t.Run("empty credentials enable the default credential chain", func(t *testing.T) {
		incoming := &StorageEngineConfig{
			DefaultProvider: "s3",
			S3: &S3EngineConfig{
				AccessKey:  "",
				SecretKey:  "",
				Region:     "us-east-1",
				BucketName: "bucket",
			},
		}
		merged := MergeStorageEngineConfigForUpdate(incoming, existing)
		require.NotNil(t, merged)
		require.NotNil(t, merged.S3)
		assert.Empty(t, merged.S3.AccessKey)
		assert.Empty(t, merged.S3.SecretKey)
	})

	t.Run("redacted placeholders preserve stored credentials", func(t *testing.T) {
		incoming := &StorageEngineConfig{
			DefaultProvider: "s3",
			S3: &S3EngineConfig{
				AccessKey:  RedactedSecretPlaceholder,
				SecretKey:  RedactedSecretPlaceholder,
				Region:     "us-east-1",
				BucketName: "bucket",
			},
		}
		merged := MergeStorageEngineConfigForUpdate(incoming, existing)
		require.NotNil(t, merged)
		require.NotNil(t, merged.S3)
		assert.Equal(t, "stored-access-key", merged.S3.AccessKey)
		assert.Equal(t, "stored-secret-key", merged.S3.SecretKey)
	})
}

func TestParserEngineConfigForResponse_NilSafe(t *testing.T) {
	assert.Nil(t, ParserEngineConfigForResponse(nil, true))
}

func TestStorageEngineConfigForResponse_NilSafe(t *testing.T) {
	assert.Nil(t, StorageEngineConfigForResponse(nil, true))
}

func TestSandboxEditorPreservesSessionPreparation(t *testing.T) {
	existing := &TenantSandboxConfig{SandboxType: "e2b", SkillPreparation: "session", E2B: &E2BSandboxConfig{OnTimeout: "kill"}}
	merged := MergeSandboxConfigForUpdate(&TenantSandboxConfig{SandboxType: "e2b", E2B: &E2BSandboxConfig{}}, existing)
	require.Equal(t, "session", merged.SkillPreparation)
	require.Equal(t, "kill", merged.E2B.OnTimeout)
}
