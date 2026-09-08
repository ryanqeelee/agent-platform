package handler

import (
	"errors"
	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"strings"
	"unicode/utf8"
)

func (h *TenantMemberHandler) CreateEmployee(c *gin.Context) {
	tenantID, ok := parseTenantIDFromPath(c)
	if !ok {
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewValidationError("请填写用户名、有效邮箱和初始密码"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if n := utf8.RuneCountInString(req.Username); n < 2 || n > 50 {
		c.Error(apperrors.NewValidationError("用户名需为 2–50 个字符"))
		return
	}
	user, member, err := h.userService.CreateEnterpriseEmployee(c.Request.Context(), tenantID, &types.RegisterRequest{Username: req.Username, Email: req.Email, Password: req.Password})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserIdentityConflict):
			c.Error(apperrors.NewConflictError("用户名或邮箱已被使用，请检查现有成员或联系平台管理员"))
		case errors.Is(err, service.ErrMemberActionForbidden):
			c.Error(apperrors.NewForbiddenError("仅本企业管理员可以创建员工"))
		case errors.Is(err, service.ErrPasswordPolicy), errors.Is(err, service.ErrComplexPasswordPolicy):
			c.Error(apperrors.NewValidationError(err.Error()))
		default:
			c.Error(apperrors.NewInternalServerError("创建员工失败"))
		}
		return
	}
	writeAddMemberSuccess(c, user, member)
}
