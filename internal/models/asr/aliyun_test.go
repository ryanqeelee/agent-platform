package asr

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliyunASRTranscribeUsesQwenChatProtocol(t *testing.T) {
	withASRSSRFWhitelist(t, "127.0.0.1")
	audio := []byte("RIFFtest-audio")
	var request aliyunASRRequest
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		authorization = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" test transcript "}}]}`))
	}))
	defer server.Close()

	instance, err := NewASR(&Config{
		Provider: string(provider.ProviderAliyun), BaseURL: server.URL,
		ModelName: "qwen3-asr-flash", ModelID: "asr-1", APIKey: "dashscope-key", Language: "zh",
	})
	require.NoError(t, err)
	result, err := instance.Transcribe(t.Context(), audio, "sample.wav")
	require.NoError(t, err)

	assert.Equal(t, "test transcript", result.Text)
	assert.Empty(t, result.Segments)
	assert.Equal(t, "Bearer dashscope-key", authorization)
	assert.Equal(t, "qwen3-asr-flash", request.Model)
	assert.False(t, request.Stream)
	assert.Equal(t, "zh", request.ASROptions.Language)
	assert.False(t, request.ASROptions.EnableITN)
	require.Len(t, request.Messages, 1)
	require.Len(t, request.Messages[0].Content, 1)
	content := request.Messages[0].Content[0]
	assert.Equal(t, "input_audio", content.Type)
	require.NotNil(t, content.InputAudio)
	assert.Equal(t, "data:audio/wav;base64,"+base64.StdEncoding.EncodeToString(audio), content.InputAudio.Data)
}

func TestAliyunASRAllowsStructuredEmptySilenceResponse(t *testing.T) {
	withASRSSRFWhitelist(t, "127.0.0.1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer server.Close()

	instance, err := NewAliyunASR(&Config{BaseURL: server.URL, ModelName: "qwen3-asr-flash"})
	require.NoError(t, err)
	result, err := instance.Transcribe(t.Context(), []byte("RIFFsilence"), "silence.wav")
	require.NoError(t, err)
	assert.Empty(t, result.Text)
}

func TestAliyunASRAudioMIMETypes(t *testing.T) {
	tests := map[string]string{
		"recording.webm": "audio/webm",
		"recording.opus": "audio/opus",
		"recording.aac":  "audio/aac",
		"recording.mp3":  "audio/mpeg",
	}
	for fileName, expected := range tests {
		t.Run(fileName, func(t *testing.T) {
			mimeType, ok := audioMIMEType(DetectAudioFormat([]byte("audio"), fileName))
			require.True(t, ok)
			assert.Equal(t, expected, mimeType)
		})
	}

	_, ok := audioMIMEType(DetectAudioFormat([]byte("unknown"), "recording.bin"))
	assert.False(t, ok)
}

func TestNewASRPreservesGenericOpenAIAdapter(t *testing.T) {
	withASRSSRFWhitelist(t, "127.0.0.1")
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"generic transcript"}`))
	}))
	defer server.Close()

	instance, err := NewASR(&Config{Provider: "generic", BaseURL: server.URL, ModelName: "whisper-test"})
	require.NoError(t, err)
	result, err := instance.Transcribe(t.Context(), []byte("RIFFaudio"), "test.wav")
	require.NoError(t, err)
	assert.Equal(t, "/audio/transcriptions", path)
	assert.Equal(t, "generic transcript", result.Text)
}

func TestAliyunASRRejectsFailedAndMalformedResponses(t *testing.T) {
	withASRSSRFWhitelist(t, "127.0.0.1")
	tests := []struct {
		name string
		body string
		code int
		want string
	}{
		{name: "non-2xx", code: http.StatusUnauthorized, body: `{"message":"bad key"}`, want: "401"},
		{name: "malformed JSON", code: http.StatusOK, body: `{`, want: "decode"},
		{name: "missing content", code: http.StatusOK, body: `{"choices":[{"message":{}}]}`, want: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			instance, err := NewAliyunASR(&Config{BaseURL: server.URL, ModelName: "qwen3-asr-flash"})
			require.NoError(t, err)
			_, err = instance.Transcribe(t.Context(), []byte("RIFFaudio"), "test.wav")
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.want))
		})
	}
}
