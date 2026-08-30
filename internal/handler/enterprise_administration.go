package handler

import (
	"net/http"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type EnterpriseAdministrationHandler struct {
	service interfaces.EnterpriseAdministrationService
}

func NewEnterpriseAdministrationHandler(
	service interfaces.EnterpriseAdministrationService,
) *EnterpriseAdministrationHandler {
	return &EnterpriseAdministrationHandler{service: service}
}

func (h *EnterpriseAdministrationHandler) GetQueue(c *gin.Context) {
	queue, err := h.service.Resolve(c.Request.Context())
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError("enterprise administration status unavailable"))
		return
	}
	c.JSON(http.StatusOK, queue)
}
