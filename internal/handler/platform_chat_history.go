package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type PlatformChatHistoryHandler struct {
	service *service.PlatformChatHistoryService
}

func NewPlatformChatHistoryHandler(service *service.PlatformChatHistoryService) *PlatformChatHistoryHandler {
	return &PlatformChatHistoryHandler{service: service}
}

// GetConfig godoc
// @Summary      获取平台消息索引配置
// @Description  获取部署级消息索引开关和 Embedding 模型；配置不隶属任何企业
// @Tags         系统管理
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      403  {object}  errors.AppError
// @Security     Bearer
// @Router       /system/admin/chat-history-config [get]
func (h *PlatformChatHistoryHandler) GetConfig(c *gin.Context) {
	config, err := h.service.RuntimeConfig(c.Request.Context())
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		_ = c.Error(errors.NewInternalServerError("Failed to load message index configuration").WithDetails(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

// UpdateConfig godoc
// @Summary      更新平台消息索引配置
// @Description  保存部署级消息索引开关和 Embedding 模型；启用时模型必填
// @Tags         系统管理
// @Accept       json
// @Produce      json
// @Param        request  body      types.PlatformChatHistoryConfig  true  "消息索引配置"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Failure      403      {object}  errors.AppError
// @Security     Bearer
// @Router       /system/admin/chat-history-config [put]
func (h *PlatformChatHistoryHandler) UpdateConfig(c *gin.Context) {
	var request types.PlatformChatHistoryConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	updated, err := h.service.Update(c.Request.Context(), &request)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			_ = c.Error(appErr)
		} else {
			logger.ErrorWithFields(c.Request.Context(), err, nil)
			_ = c.Error(errors.NewInternalServerError("Failed to update message index configuration").WithDetails(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

// GetStats godoc
// @Summary      获取平台消息索引统计
// @Description  汇总各企业私有消息索引知识库及已索引消息数量
// @Tags         系统管理
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Failure      403  {object}  errors.AppError
// @Security     Bearer
// @Router       /system/admin/chat-history-stats [get]
func (h *PlatformChatHistoryHandler) GetStats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		_ = c.Error(errors.NewInternalServerError("Failed to load message index statistics").WithDetails(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}
