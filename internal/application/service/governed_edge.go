package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type governedEdgeResolver struct {
	db     *gorm.DB
	config *config.Config
}

func NewGovernedEdgeResolver(db *gorm.DB, cfg *config.Config) interfaces.GovernedEdgeResolver {
	return &governedEdgeResolver{db: db, config: cfg}
}

func (r *governedEdgeResolver) Resolve(ctx context.Context, tenantID uint64) (types.GovernedEdgeConnection, error) {
	var tenant types.Tenant
	if err := r.db.WithContext(ctx).Select("id", "status", "governed_edge_binding").First(&tenant, tenantID).Error; err != nil {
		return types.GovernedEdgeConnection{}, err
	}
	b := tenant.GovernedEdgeBinding
	if tenant.Status != types.TenantStatusActive || b == nil || !b.Enabled {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	if r.config.Agent == nil || r.config.Agent.GovernedData == nil {
		return types.GovernedEdgeConnection{}, fmt.Errorf("Edge connection is not configured")
	}
	// Read the private configuration on each use, so a credential rotation or
	// endpoint change cannot leave an old turn using a cached connection.
	raw, err := os.ReadFile(r.config.Agent.GovernedData.ConnectionsFile)
	if err != nil {
		return types.GovernedEdgeConnection{}, fmt.Errorf("Edge connection configuration is unavailable")
	}
	var nodes map[string]struct {
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return types.GovernedEdgeConnection{}, fmt.Errorf("invalid Edge connection configuration")
	}
	node, ok := nodes[b.EdgeNodeID]
	if !ok || node.BaseURL == "" || node.Token == "" {
		return types.GovernedEdgeConnection{}, fmt.Errorf("Edge connection is not configured")
	}
	return types.GovernedEdgeConnection{EnterpriseID: b.EnterpriseID, EdgeNodeID: b.EdgeNodeID, SourceID: b.SourceID, BindingID: b.BindingID, Revision: b.Revision, BaseURL: node.BaseURL, Token: node.Token}, nil
}

func (s *tenantService) ApplyGovernedEdgeBinding(ctx context.Context, tenantID uint64, binding types.GovernedEdgeBinding) error {
	if tenantID == 0 || binding.BindingID == "" || binding.EnterpriseID == "" || binding.Revision < 1 || (binding.Enabled && (binding.EdgeNodeID == "" || binding.SourceID == "")) {
		return fmt.Errorf("invalid governed Edge binding")
	}
	return s.repo.ApplyGovernedEdgeBinding(ctx, tenantID, binding)
}
