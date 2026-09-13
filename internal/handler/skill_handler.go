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
	usableSkills   usableSkillLister
	catalog        skillCatalogService
	employeeAgents interfaces.CustomAgentService
}

type skillCatalogService interface {
	ListCatalog(ctx context.Context) ([]service.SkillCatalogView, error)
	RegisterCatalogFromArchive(ctx context.Context, archive []byte) (*types.TenantSkillCatalogEntity, error)
	RegisterCatalogFromSource(ctx context.Context, source string) (*types.TenantSkillCatalogEntity, error)
	InstallCatalogToConfigs(ctx context.Context, catalogID string, configIDs []string) (*service.CatalogInstallResult, error)
	DeleteCatalog(ctx context.Context, catalogID string) error
	ListCatalogFiles(ctx context.Context, catalogID string) ([]service.SkillFileEntry, error)
	ReadCatalogFile(ctx context.Context, catalogID, relativePath string) (*service.SkillFileContent, error)
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
