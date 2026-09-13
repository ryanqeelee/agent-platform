package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// WebSearchProviderCredentialsHandler handles credentials for web search
// providers via the dedicated /credentials subresource. Currently the only
// recognized field is "api_key" — every provider that needs credentials uses
// just one key (Bing / Google / Tavily / Ollama / Baidu), and DuckDuckGo /
// SearXNG don't need credentials at all.
type WebSearchProviderCredentialsHandler struct {
	repo interfaces.WebSearchProviderRepository
	svc  interfaces.WebSearchProviderService
}

func NewWebSearchProviderCredentialsHandler(
	repo interfaces.WebSearchProviderRepository,
	svc interfaces.WebSearchProviderService,
) *WebSearchProviderCredentialsHandler {
	return &WebSearchProviderCredentialsHandler{repo: repo, svc: svc}
}

type webSearchCredentialsPutRequest struct {
	APIKey *string `json:"api_key,omitempty"`
}

// Put godoc
// @Summary      设置网络搜索 Provider 凭据
// @Description  写入或替换平台级 Provider API Key；省略字段时返回当前配置状态
// @Tags         网络搜索
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "Provider ID"
// @Param        request  body      map[string]interface{}  true  "{api_key?: string}"
// @Success      200      {object}  map[string]interface{}  "凭据状态"
// @Security     Bearer
// @Router       /system/admin/web-search-providers/{id}/credentials [put]
func (h *WebSearchProviderCredentialsHandler) Put(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req webSearchCredentialsPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if req.APIKey == nil {
		provider, err := h.repo.GetByID(ctx, id)
		if err != nil || provider == nil {
			c.Error(errors.NewNotFoundError("web search provider not found"))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.CredentialsResponse{
			Fields: map[string]dto.CredentialFieldMetadata{
				"api_key": {Configured: provider.Parameters.APIKey != ""},
			},
		}})
		return
	}
	updated, err := h.svc.UpdateProviderCredentials(ctx, id, req.APIKey)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"provider_id": secutils.SanitizeForLog(id),
		})
		c.Error(errors.NewInternalServerError("failed to update credentials: " + err.Error()))
		return
	}
	resp := dto.CredentialsResponse{
		Fields: map[string]dto.CredentialFieldMetadata{
			"api_key": {Configured: updated.Parameters.APIKey != ""},
		},
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// DeleteField godoc
// @Summary      移除网络搜索 Provider 凭据
// @Description  删除平台级 Provider 的 API Key
// @Tags         网络搜索
// @Produce      json
// @Param        id     path      string  true  "Provider ID"
// @Param        field  path      string  true  "字段名（api_key）"
// @Success      204
// @Security     Bearer
// @Router       /system/admin/web-search-providers/{id}/credentials/{field} [delete]
func (h *WebSearchProviderCredentialsHandler) DeleteField(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	field := c.Param("field")
	if field != "api_key" {
		c.Error(errors.NewBadRequestError("unknown credential field: " + secutils.SanitizeForLog(field)))
		return
	}
	if err := h.svc.ClearProviderCredential(ctx, id, field); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"provider_id": secutils.SanitizeForLog(id),
			"field":       field,
		})
		c.Error(errors.NewInternalServerError("failed to clear credential: " + err.Error()))
		return
	}
	c.Status(http.StatusNoContent)
}
