package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type initializationModelService struct {
	interfaces.ModelService
	stored *types.Model
}

func (s *initializationModelService) GetModelByID(context.Context, string) (*types.Model, error) {
	return s.stored, nil
}

func tenantlessInitializationContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), types.SystemAdminContextKey, true))
	c.Request = request
	return c, recorder
}

func enableInitializationLoopback(t *testing.T) {
	t.Helper()
	t.Setenv("SSRF_WHITELIST", "127.0.0.1")
	utils.ResetSSRFWhitelistForTest()
	t.Cleanup(utils.ResetSSRFWhitelistForTest)
}

func requireAvailableModelTestResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, true, response.Data["available"])
	return response.Data
}

func TestCheckRemoteModel_TenantlessUsesStoredModelCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"qwen-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	h := &InitializationHandler{modelService: &initializationModelService{stored: &types.Model{
		Parameters: types.ModelParameters{APIKey: "stored-chat-key"},
	}}}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelId":"global-chat","modelName":"qwen-test","baseUrl":"`+server.URL+`","provider":"generic"
	}`)

	h.CheckRemoteModel(c)

	require.Empty(t, c.Errors)
	requireAvailableModelTestResponse(t, recorder)
	assert.Equal(t, "Bearer stored-chat-key", authorization)
}

func TestModelTestPNG_HasProviderSupportedDimensions(t *testing.T) {
	config, err := png.DecodeConfig(bytes.NewReader(modelTestPNG))
	require.NoError(t, err)
	assert.Equal(t, 64, config.Width)
	assert.Equal(t, 64, config.Height)
}

func TestCheckRemoteModel_TenantlessVLMUsesImageRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)

	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer visual-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-vlm","object":"chat.completion","created":1,"model":"qwen-vl-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"a white pixel"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	h := &InitializationHandler{}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelType":"VLLM","modelName":"qwen-vl-test","baseUrl":"`+server.URL+`","apiKey":"visual-key","provider":"aliyun"
	}`)

	h.CheckRemoteModel(c)

	require.Empty(t, c.Errors)
	requireAvailableModelTestResponse(t, recorder)
	messages := request["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	require.Len(t, content, 2)
	assert.Equal(t, "text", content[0].(map[string]any)["type"])
	imagePart := content[1].(map[string]any)
	assert.Equal(t, "image_url", imagePart["type"])
	imageURL := imagePart["image_url"].(map[string]any)["url"].(string)
	const imagePrefix = "data:image/png;base64,"
	require.True(t, strings.HasPrefix(imageURL, imagePrefix))
	sentImage, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageURL, imagePrefix))
	require.NoError(t, err)
	assert.Equal(t, modelTestPNG, sentImage)
	config, err := png.DecodeConfig(bytes.NewReader(sentImage))
	require.NoError(t, err)
	assert.Equal(t, 64, config.Width)
	assert.Equal(t, 64, config.Height)
}

func TestEmbeddingModel_TenantlessUsesProvidedCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/embeddings", r.URL.Path)
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3],"index":0}]}`))
	}))
	defer server.Close()

	h := &InitializationHandler{}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelName":"text-embedding-test","baseUrl":"`+server.URL+`","apiKey":"provided-embedding-key","provider":"generic"
	}`)

	h.TestEmbeddingModel(c)

	require.Empty(t, c.Errors)
	data := requireAvailableModelTestResponse(t, recorder)
	assert.Equal(t, float64(3), data["dimension"])
	assert.Equal(t, "Bearer provided-embedding-key", authorization)
}

func TestCheckRerankModel_TenantlessUsesProvidedCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rerank", r.URL.Path)
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"rerank-test","model":"rerank-test","usage":{"total_tokens":2},
			"results":[{"index":0,"document":{"text":"pong"},"relevance_score":0.9}]
		}`))
	}))
	defer server.Close()

	h := &InitializationHandler{}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelName":"rerank-test","baseUrl":"`+server.URL+`","apiKey":"provided-rerank-key","provider":"generic"
	}`)

	h.CheckRerankModel(c)

	require.Empty(t, c.Errors)
	requireAvailableModelTestResponse(t, recorder)
	assert.Equal(t, "Bearer provided-rerank-key", authorization)
}

func TestCheckASRModel_ProviderFailureIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/audio/transcriptions", r.URL.Path)
		http.Error(w, "unsupported audio model", http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	h := &InitializationHandler{}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelName":"speech-test","baseUrl":"`+server.URL+`","apiKey":"speech-key","provider":"generic"
	}`)

	h.CheckASRModel(c)

	require.Empty(t, c.Errors)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, false, response.Data["available"])
	assert.Contains(t, response.Data["message"], "422")
}

func TestCheckASRModel_TenantlessAliyunUsesAudioChatProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableInitializationLoopback(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer speech-key", r.Header.Get("Authorization"))
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		messages := request["messages"].([]any)
		content := messages[0].(map[string]any)["content"].([]any)
		audio := content[0].(map[string]any)["input_audio"].(map[string]any)["data"].(string)
		assert.True(t, strings.HasPrefix(audio, "data:audio/wav;base64,"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer server.Close()

	h := &InitializationHandler{}
	c, recorder := tenantlessInitializationContext(t, `{
		"modelName":"qwen3-asr-flash","baseUrl":"`+server.URL+`","apiKey":"speech-key","provider":"aliyun"
	}`)

	h.CheckASRModel(c)

	require.Empty(t, c.Errors)
	requireAvailableModelTestResponse(t, recorder)
}

func TestResolveTestModelCredentials_OnlyWeKnoraCloudFallsBackToTenant(t *testing.T) {
	tenant := &types.Tenant{Credentials: &types.CredentialsConfig{WeKnoraCloud: &types.WeKnoraCloudCredentials{
		AppID: "tenant-app-id", AppSecret: "tenant-app-secret",
	}}}
	ctx := context.WithValue(context.Background(), types.TenantInfoContextKey, tenant)
	h := &InitializationHandler{}

	generic := &types.Model{Parameters: types.ModelParameters{Provider: string(provider.ProviderGeneric)}}
	appID, appSecret := h.resolveTestModelCredentials(ctx, generic)
	assert.Empty(t, appID)
	assert.Empty(t, appSecret)

	weKnoraCloud := &types.Model{Parameters: types.ModelParameters{Provider: string(provider.ProviderWeKnoraCloud)}}
	appID, appSecret = h.resolveTestModelCredentials(ctx, weKnoraCloud)
	assert.Equal(t, "tenant-app-id", appID)
	assert.Equal(t, "tenant-app-secret", appSecret)

	modelOwned := &types.Model{Parameters: types.ModelParameters{
		Provider: string(provider.ProviderWeKnoraCloud), AppID: "model-app-id", AppSecret: "model-app-secret",
	}}
	appID, appSecret = h.resolveTestModelCredentials(ctx, modelOwned)
	assert.Equal(t, "model-app-id", appID)
	assert.Equal(t, "model-app-secret", appSecret)

	nilTenantCtx := context.WithValue(context.Background(), types.TenantInfoContextKey, (*types.Tenant)(nil))
	appID, appSecret = h.resolveTestModelCredentials(nilTenantCtx, weKnoraCloud)
	assert.Empty(t, appID)
	assert.Empty(t, appSecret)
}
