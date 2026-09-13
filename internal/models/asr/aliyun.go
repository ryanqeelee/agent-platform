package asr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/provider"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

type AliyunASR struct {
	modelName string
	modelID   string
	apiKey    string
	baseURL   string
	language  string
	client    *http.Client
	headers   map[string]string
}

type aliyunASRRequest struct {
	Model      string             `json:"model"`
	Messages   []aliyunASRMessage `json:"messages"`
	Stream     bool               `json:"stream"`
	ASROptions aliyunASROptions   `json:"asr_options"`
}

type aliyunASRMessage struct {
	Role    string             `json:"role"`
	Content []aliyunASRContent `json:"content"`
}

type aliyunASRContent struct {
	Type       string               `json:"type"`
	InputAudio *aliyunASRInputAudio `json:"input_audio,omitempty"`
}

type aliyunASRInputAudio struct {
	Data string `json:"data"`
}

type aliyunASROptions struct {
	Language  string `json:"language,omitempty"`
	EnableITN bool   `json:"enable_itn"`
}

type aliyunASRResponse struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewAliyunASR(config *Config) (*AliyunASR, error) {
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = provider.AliyunChatBaseURL
	}
	if err := validateASRBaseURL(baseURL); err != nil {
		return nil, err
	}
	return &AliyunASR{
		modelName: config.ModelName,
		modelID:   config.ModelID,
		apiKey:    config.APIKey,
		baseURL:   baseURL,
		language:  config.Language,
		client:    newASRHTTPClient(asrDefaultTimeout),
		headers:   config.CustomHeaders,
	}, nil
}

func (s *AliyunASR) Transcribe(ctx context.Context, audioBytes []byte, fileName string) (*TranscriptionResult, error) {
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("audio bytes are empty")
	}
	extension := DetectAudioFormat(audioBytes, fileName)
	mimeType, ok := audioMIMEType(extension)
	if !ok {
		return nil, fmt.Errorf("unsupported Aliyun ASR audio format %q", extension)
	}
	payload := aliyunASRRequest{
		Model: s.modelName,
		Messages: []aliyunASRMessage{{
			Role: "user",
			Content: []aliyunASRContent{{
				Type: "input_audio",
				InputAudio: &aliyunASRInputAudio{Data: fmt.Sprintf(
					"data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(audioBytes),
				)},
			}},
		}},
		Stream:     false,
		ASROptions: aliyunASROptions{Language: s.language, EnableITN: false},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal Aliyun ASR request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Aliyun ASR request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	secutils.ApplyCustomHeaders(request, s.headers)

	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Aliyun ASR request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read Aliyun ASR response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Aliyun ASR API error: HTTP %s: %s", response.Status, string(responseBody))
	}
	var decoded aliyunASRResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode Aliyun ASR response: %w", err)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == nil {
		return nil, fmt.Errorf("Aliyun ASR response missing choices[0].message.content")
	}
	return &TranscriptionResult{Text: strings.TrimSpace(*decoded.Choices[0].Message.Content)}, nil
}

func audioMIMEType(extension string) (string, bool) {
	switch strings.ToLower(extension) {
	case ".wav":
		return "audio/wav", true
	case ".flac":
		return "audio/flac", true
	case ".ogg":
		return "audio/ogg", true
	case ".m4a":
		return "audio/mp4", true
	case ".mp3":
		return "audio/mpeg", true
	case ".aac":
		return "audio/aac", true
	case ".webm":
		return "audio/webm", true
	case ".opus":
		return "audio/opus", true
	}
	return "", false
}

func (s *AliyunASR) GetModelName() string { return s.modelName }
func (s *AliyunASR) GetModelID() string   { return s.modelID }
