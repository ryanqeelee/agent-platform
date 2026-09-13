package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// SystemInfo represents system version and configuration information
type SystemInfo struct {
	Version             string `json:"version"`
	Edition             string `json:"edition"`
	CommitID            string `json:"commit_id,omitempty"`
	BuildTime           string `json:"build_time,omitempty"`
	GoVersion           string `json:"go_version,omitempty"`
	KeywordIndexEngine  string `json:"keyword_index_engine,omitempty"`
	VectorStoreEngine   string `json:"vector_store_engine,omitempty"`
	GraphDatabaseEngine string `json:"graph_database_engine,omitempty"`
	MinioEnabled        bool   `json:"minio_enabled,omitempty"`
	DBVersion           string `json:"db_version,omitempty"`
	DBMigrationError    string `json:"db_migration_error,omitempty"`
	StartedAt           string `json:"started_at,omitempty"`
	UptimeSeconds       int64  `json:"uptime_seconds,omitempty"`
}

// ParserEngine represents a document parser engine
type ParserEngine struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
}

// ParserEngineConfig is the deployment-wide parser runtime configuration.
// Secret fields returned by the API contain "***" when configured.
type ParserEngineConfig struct {
	ChatParserEngineRules               []ParserEngineRule `json:"chat_parser_engine_rules,omitempty"`
	MinerUEndpoint                      string             `json:"mineru_endpoint"`
	MinerUAPIKey                        string             `json:"mineru_api_key"`
	MinerUModel                         string             `json:"mineru_model,omitempty"`
	MinerUVLMServerURL                  string             `json:"mineru_vlm_server_url,omitempty"`
	MinerUEnableFormula                 *bool              `json:"mineru_enable_formula,omitempty"`
	MinerUEnableTable                   *bool              `json:"mineru_enable_table,omitempty"`
	MinerUParseMethod                   string             `json:"mineru_parse_method,omitempty"`
	MinerUEnableOCR                     *bool              `json:"mineru_enable_ocr,omitempty"`
	MinerULanguage                      string             `json:"mineru_language,omitempty"`
	MinerUCloudModel                    string             `json:"mineru_cloud_model,omitempty"`
	MinerUCloudEnableFormula            *bool              `json:"mineru_cloud_enable_formula,omitempty"`
	MinerUCloudEnableTable              *bool              `json:"mineru_cloud_enable_table,omitempty"`
	MinerUCloudEnableOCR                *bool              `json:"mineru_cloud_enable_ocr,omitempty"`
	MinerUCloudLanguage                 string             `json:"mineru_cloud_language,omitempty"`
	ODLHybrid                           string             `json:"odl_hybrid,omitempty"`
	ODLHybridURL                        string             `json:"odl_hybrid_url,omitempty"`
	ODLHybridMode                       string             `json:"odl_hybrid_mode,omitempty"`
	ODLHybridFallback                   *bool              `json:"odl_hybrid_fallback,omitempty"`
	ODLMarkdownWithHTML                 *bool              `json:"odl_markdown_with_html,omitempty"`
	PaddleOCRVLEndpoint                 string             `json:"paddleocr_vl_endpoint,omitempty"`
	PaddleOCRVLUseSealRecognition       *bool              `json:"paddleocr_vl_use_seal_recognition,omitempty"`
	PaddleOCRVLUseChartRecognition      *bool              `json:"paddleocr_vl_use_chart_recognition,omitempty"`
	PaddleOCRVLCloudToken               string             `json:"paddleocr_vl_cloud_token,omitempty"`
	PaddleOCRVLCloudModel               string             `json:"paddleocr_vl_cloud_model,omitempty"`
	PaddleOCRVLCloudUseSealRecognition  *bool              `json:"paddleocr_vl_cloud_use_seal_recognition,omitempty"`
	PaddleOCRVLCloudUseChartRecognition *bool              `json:"paddleocr_vl_cloud_use_chart_recognition,omitempty"`
}

// StorageEngineStatusItem describes one storage engine's availability
type StorageEngineStatusItem struct {
	Name        string `json:"name"`
	Allowed     bool   `json:"allowed"`
	Available   bool   `json:"available"`
	Description string `json:"description"`
}

// StorageEngineStatusResponse is the response for storage engine status
type StorageEngineStatusResponse struct {
	Engines           []StorageEngineStatusItem `json:"engines"`
	AllowedProviders  []string                  `json:"allowed_providers"`
	MinioEnvAvailable bool                      `json:"minio_env_available"`
}

// StorageCheckRequest is the body for storage engine connectivity check
type StorageCheckRequest struct {
	Provider string          `json:"provider"`
	MinIO    json.RawMessage `json:"minio,omitempty"`
	COS      json.RawMessage `json:"cos,omitempty"`
	TOS      json.RawMessage `json:"tos,omitempty"`
	S3       json.RawMessage `json:"s3,omitempty"`
	OBS      json.RawMessage `json:"obs,omitempty"`
}

// StorageCheckResponse is the response for storage engine check
type StorageCheckResponse struct {
	OK            bool   `json:"ok"`
	Message       string `json:"message"`
	BucketCreated bool   `json:"bucket_created,omitempty"`
}

// DeploymentCapability describes whether a deployment exposes a feature route.
type DeploymentCapability struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

// DeploymentCapabilitiesData is the payload of GET /system/capabilities.
type DeploymentCapabilitiesData struct {
	Edition      string                          `json:"edition"`
	Capabilities map[string]DeploymentCapability `json:"capabilities"`
}

// GetSystemInfo gets system version and configuration information
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/system/info", nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int         `json:"code"`
		Data *SystemInfo `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetDeploymentCapabilities returns the deployment feature snapshot for SPA menu gating.
func (c *Client) GetDeploymentCapabilities(ctx context.Context) (*DeploymentCapabilitiesData, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/system/capabilities", nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int                         `json:"code"`
		Data *DeploymentCapabilitiesData `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// ListParserEngines lists available document parser engines
func (c *Client) ListParserEngines(ctx context.Context) ([]ParserEngine, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/system/parser-engines", nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code      int            `json:"code"`
		Data      []ParserEngine `json:"data"`
		Connected bool           `json:"connected"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// CheckParserEngines checks parser engine availability with given config overrides
func (c *Client) CheckParserEngines(ctx context.Context, config any) ([]ParserEngine, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/system/admin/parser-engines/check", config, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int            `json:"code"`
		Data []ParserEngine `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetParserEngineConfig returns the deployment-wide parser configuration with secrets masked.
func (c *Client) GetParserEngineConfig(ctx context.Context) (*ParserEngineConfig, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/system/admin/parser-engine-config", nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int                 `json:"code"`
		Data *ParserEngineConfig `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// UpdateParserEngineConfig replaces the deployment-wide parser configuration and returns its masked form.
func (c *Client) UpdateParserEngineConfig(ctx context.Context, config *ParserEngineConfig) (*ParserEngineConfig, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/api/v1/system/admin/parser-engine-config", config, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int                 `json:"code"`
		Data *ParserEngineConfig `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetStorageEngineStatus gets the availability status of all storage engines
func (c *Client) GetStorageEngineStatus(ctx context.Context) (*StorageEngineStatusResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/system/storage-engine-status", nil, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int                          `json:"code"`
		Data *StorageEngineStatusResponse `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// CheckStorageEngine tests connectivity for a storage engine
func (c *Client) CheckStorageEngine(ctx context.Context, req *StorageCheckRequest) (*StorageCheckResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/system/storage-engine-check", req, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Code int                   `json:"code"`
		Data *StorageCheckResponse `json:"data"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}
