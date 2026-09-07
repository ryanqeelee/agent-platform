package handler

import (
	"context"
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// usableSkillLister returns the installed skills a chat turn can actually
// invoke on one sandbox config. The @ picker and the agent editor both read
// this set so they cannot offer a skill the running image does not carry.
type usableSkillLister interface {
	ListUsableSkills(ctx context.Context, tenantID uint64, configID string) []*types.TenantSkillEntity
}

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	usableSkills       usableSkillLister
	catalog            skillCatalogService
	employeeAgents     interfaces.CustomAgentService
	employeeManagement employeeSkillManagement
}

type skillCatalogService interface {
	ListCatalog(ctx context.Context, tenantID uint64) ([]service.SkillCatalogView, error)
	RegisterCatalogFromArchive(ctx context.Context, tenantID uint64, archive []byte) (*types.TenantSkillCatalogEntity, error)
	RegisterCatalogFromSource(ctx context.Context, tenantID uint64, source string) (*types.TenantSkillCatalogEntity, error)
	InstallCatalogToConfigs(ctx context.Context, tenantID uint64, catalogID string, configIDs []string) (*service.CatalogInstallResult, error)
	DeleteCatalog(ctx context.Context, tenantID uint64, catalogID string) error
	ListCatalogFiles(ctx context.Context, tenantID uint64, catalogID string) ([]service.SkillFileEntry, error)
	ReadCatalogFile(ctx context.Context, tenantID uint64, catalogID, relativePath string) (*service.SkillFileContent, error)
}

// NewSkillHandler creates a new skill handler. catalog may be nil in tests
// that only exercise the chat picker.
func NewSkillHandler(usableSkills usableSkillLister, catalog skillCatalogService) *SkillHandler {
	return &SkillHandler{
		usableSkills: usableSkills,
		catalog:      catalog,
	}
}

// SkillInfoResponse represents the skill info returned to frontend
type SkillInfoResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListSkills godoc
// @Summary      获取当前沙箱配置上可执行的 Skills
// @Description  返回指定沙箱配置镜像内、智能体实际能调用的已安装技能（ready 且启用）。不传 sandbox_config_id 时列表为空。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        sandbox_config_id  query     string  false  "Sandbox config ID"
// @Success      200  {object}  map[string]interface{}  "Skills列表"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	configID := c.Query("sandbox_config_id")
	if configID == "" || h.usableSkills == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":          true,
			"data":             []SkillInfoResponse{},
			"skills_available": false,
		})
		return
	}

	rows := h.usableSkills.ListUsableSkills(
		c.Request.Context(), sandboxConfigTenantID(c), configID,
	)
	response := make([]SkillInfoResponse, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		response = append(response, SkillInfoResponse{
			Name:        row.Name,
			Description: row.Description,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             response,
		"skills_available": true,
	})
}

// WithEmployeeAgents binds the read-only employee picker to server-owned authority.
func (h *SkillHandler) WithEmployeeAgents(agents interfaces.CustomAgentService) *SkillHandler {
	h.employeeAgents = agents
	return h
}

func (h *SkillHandler) ListEmployeeSkills(c *gin.Context) {
	if h.employeeAgents == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	agent, err := h.employeeAgents.GetAgentByID(c.Request.Context(), types.BuiltinEmployeeAssistantID)
	if err != nil || agent == nil || agent.TenantID != sandboxConfigTenantID(c) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "employee assistant skills unavailable"})
		return
	}
	response := []SkillInfoResponse{}
	if agent.Config.SandboxConfigID != "" && agent.Config.SkillsSelectionMode == "all" && h.usableSkills != nil {
		rows := h.usableSkills.ListUsableSkills(c.Request.Context(), sandboxConfigTenantID(c), agent.Config.SandboxConfigID)
		for _, row := range rows {
			if row != nil {
				response = append(response, SkillInfoResponse{Name: row.Name, Description: row.Description})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response, "skills_available": len(response) > 0})
}

type employeeSkillManagement interface {
	ListSkills(context.Context, uint64, string) ([]*types.TenantSkillEntity, error)
	UpdateSkillAdmin(context.Context, uint64, string, string, service.SkillAdminUpdate) (*types.TenantSkillEntity, error)
}

func (h *SkillHandler) WithEmployeeManagement(manager employeeSkillManagement) *SkillHandler {
	h.employeeManagement = manager
	return h
}
func (h *SkillHandler) employeeSkillConfig(c *gin.Context) (string, bool) {
	if h.employeeAgents == nil || h.employeeManagement == nil {
		c.Status(503)
		return "", false
	}
	agent, err := h.employeeAgents.GetAgentByID(c.Request.Context(), types.BuiltinEmployeeAssistantID)
	if err != nil || agent == nil || agent.TenantID != sandboxConfigTenantID(c) {
		c.Status(503)
		return "", false
	}
	if agent.Config.SandboxConfigID == "" {
		c.JSON(409, gin.H{"error": "员工助理沙箱尚未启用，请联系平台管理员"})
		return "", false
	}
	return agent.Config.SandboxConfigID, true
}
func (h *SkillHandler) ListEnterpriseSkills(c *gin.Context) {
	config, ok := h.employeeSkillConfig(c)
	if !ok {
		return
	}
	rows, err := h.employeeManagement.ListSkills(c.Request.Context(), sandboxConfigTenantID(c), config)
	if err != nil {
		c.Status(503)
		return
	}
	response := []gin.H{}
	for _, row := range rows {
		if row != nil {
			response = append(response, gin.H{"id": row.ID, "name": row.Name, "description": row.Description, "status": row.Status, "enabled": row.Enabled})
		}
	}
	c.JSON(200, gin.H{"data": response})
}
func (h *SkillHandler) SetEnterpriseSkillEnabled(c *gin.Context) {
	config, ok := h.employeeSkillConfig(c)
	if !ok {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		c.Status(400)
		return
	}
	row, err := h.employeeManagement.UpdateSkillAdmin(c.Request.Context(), sandboxConfigTenantID(c), config, c.Param("id"), service.SkillAdminUpdate{Enabled: body.Enabled})
	if err != nil {
		c.Status(503)
		return
	}
	if row == nil {
		c.Status(404)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
