package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/stretchr/testify/require"
)

func TestParserEngineConfigResolveChatParserEngine(t *testing.T) {
	config := &ParserEngineConfig{ChatParserEngineRules: []ParserEngineRule{
		{FileTypes: []string{"pdf", ".docx"}, Engine: "mineru"},
		{FileTypes: []string{"xlsx"}, Engine: "markitdown"},
	}}
	for input, expected := range map[string]string{
		"PDF": "mineru", ".docx": "mineru", "xlsx": "markitdown",
		"txt": "", "pptx": "markitdown", ".PPT": "markitdown",
	} {
		if actual := config.ResolveChatParserEngine(input); actual != expected {
			t.Fatalf("ResolveChatParserEngine(%q) = %q, want %q", input, actual, expected)
		}
	}
	var nilConfig *ParserEngineConfig
	if actual := nilConfig.ResolveChatParserEngine("pdf"); actual != "" {
		t.Fatalf("nil config resolved %q", actual)
	}
	if actual := nilConfig.ResolveChatParserEngine("pptx"); actual != "markitdown" {
		t.Fatalf("nil config pptx resolved %q, want markitdown", actual)
	}
}

func TestParserEngineConfigValueAndScanEncryptSecrets(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	original := &ParserEngineConfig{
		MinerUAPIKey:          "mineru-secret",
		PaddleOCRVLCloudToken: "paddle-secret",
		MinerUEndpoint:        "https://mineru.example.invalid",
	}
	value, err := original.Value()
	require.NoError(t, err)
	bytes, ok := value.([]byte)
	require.True(t, ok)
	require.NotContains(t, string(bytes), "mineru-secret")
	require.NotContains(t, string(bytes), "paddle-secret")

	var stored ParserEngineConfig
	require.NoError(t, json.Unmarshal(bytes, &stored))
	require.True(t, strings.HasPrefix(stored.MinerUAPIKey, utils.EncPrefix))
	require.True(t, strings.HasPrefix(stored.PaddleOCRVLCloudToken, utils.EncPrefix))

	var loaded ParserEngineConfig
	require.NoError(t, loaded.Scan(bytes))
	require.Equal(t, original, &loaded)
	require.Equal(t, "mineru-secret", original.MinerUAPIKey)
}

func TestParserEngineConfigValueRejectsSecretWithoutAESKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	_, err := (&ParserEngineConfig{MinerUAPIKey: "plaintext"}).Value()
	require.ErrorContains(t, err, "SYSTEM_AES_KEY")
}

func TestParserEngineConfigValueAllowsNonSecretConfigWithoutAESKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")
	_, err := (&ParserEngineConfig{MinerUEndpoint: "https://mineru.example.invalid"}).Value()
	require.NoError(t, err)
}

func TestParserEngineConfigScanRejectsPlaintextCredential(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	var loaded ParserEngineConfig
	err := loaded.Scan([]byte(`{"mineru_api_key":"legacy","chat_parser_engine_rules":[{"file_types":["pdf"],"engine":"mineru"}]}`))
	require.ErrorContains(t, err, "plaintext credential")
}

func TestParserEngineConfigScanRejectsEncryptedCredentialWithoutAESKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	stored, err := utils.EncryptAESGCM("secret", utils.GetAESKey())
	require.NoError(t, err)
	t.Setenv("SYSTEM_AES_KEY", "")

	var loaded ParserEngineConfig
	err = loaded.Scan([]byte(`{"mineru_api_key":"` + stored + `"}`))
	require.ErrorIs(t, err, utils.ErrEncryptedDataMissingKey)
}

func TestParserEngineConfigScanRejectsCorruptCiphertext(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	var loaded ParserEngineConfig
	err := loaded.Scan([]byte(`{"paddleocr_vl_cloud_token":"enc:v1:not-valid-ciphertext"}`))
	require.ErrorContains(t, err, "decrypt platform_parser_config.paddleocr_vl_cloud_token")
}

func TestParserEngineConfigScanRejectsUnsupportedSQLValue(t *testing.T) {
	var loaded ParserEngineConfig
	err := loaded.Scan(42)
	require.ErrorContains(t, err, "unsupported value type int")
}
