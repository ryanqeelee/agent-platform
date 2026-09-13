package rerank

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliyunRerankerQwen37NativeProtocol(t *testing.T) {
	withRerankSSRFWhitelist(t, "127.0.0.1")
	var request struct {
		Model      string            `json:"model"`
		Input      AliyunRerankInput `json:"input"`
		Parameters struct {
			ReturnDocuments *bool `json:"return_documents"`
			TopN            int   `json:"top_n"`
		} `json:"parameters"`
	}
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/services/rerank/text-rerank/text-rerank", r.URL.Path)
		authorization = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":{"results":[{"index":0,"relevance_score":0.97}]},
			"usage":{"total_tokens":2}
		}`))
	}))
	defer server.Close()

	reranker, err := NewAliyunReranker(&RerankerConfig{
		BaseURL:   server.URL + "/api/v1/services/rerank/text-rerank/text-rerank",
		ModelName: "qwen3.7-text-rerank", APIKey: "dashscope-key",
	})
	require.NoError(t, err)
	results, err := reranker.Rerank(t.Context(), "ping", []string{"pong"})
	require.NoError(t, err)

	assert.Equal(t, "Bearer dashscope-key", authorization)
	assert.Equal(t, "qwen3.7-text-rerank", request.Model)
	assert.Equal(t, "ping", request.Input.Query)
	assert.Equal(t, []string{"pong"}, request.Input.Documents)
	assert.Nil(t, request.Parameters.ReturnDocuments)
	assert.Equal(t, 1, request.Parameters.TopN)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].Index)
	assert.Equal(t, 0.97, results[0].RelevanceScore)
}

func TestAliyunRerankerOtherModelsRequestDocuments(t *testing.T) {
	withRerankSSRFWhitelist(t, "127.0.0.1")
	var returnDocuments *bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Parameters struct {
				ReturnDocuments *bool `json:"return_documents"`
			} `json:"parameters"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		returnDocuments = request.Parameters.ReturnDocuments
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"results":[]},"usage":{"total_tokens":2}}`))
	}))
	defer server.Close()

	reranker, err := NewAliyunReranker(&RerankerConfig{
		BaseURL: server.URL, ModelName: "gte-rerank-v2", APIKey: "dashscope-key",
	})
	require.NoError(t, err)
	_, err = reranker.Rerank(t.Context(), "ping", []string{"pong"})
	require.NoError(t, err)
	require.NotNil(t, returnDocuments)
	assert.True(t, *returnDocuments)
}
