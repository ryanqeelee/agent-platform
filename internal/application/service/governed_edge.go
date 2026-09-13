package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type governedEdgeResolver struct {
	db     *gorm.DB
	config *config.Config
}

type governedEdgeBindingService struct {
	tenantRepo interfaces.TenantRepository
	tenants    interfaces.TenantService
	operators  interfaces.PlatformOperationsTenantRepository
	center     interfaces.PlatformOperationsBridge
	resolver   interfaces.GovernedEdgeResolver
}

func NewGovernedEdgeResolver(db *gorm.DB, cfg *config.Config) interfaces.GovernedEdgeResolver {
	return &governedEdgeResolver{db: db, config: cfg}
}

func NewGovernedEdgeBindingService(
	tenantRepo interfaces.TenantRepository,
	tenants interfaces.TenantService,
	operators interfaces.PlatformOperationsTenantRepository,
	center interfaces.PlatformOperationsBridge,
	resolver interfaces.GovernedEdgeResolver,
) interfaces.GovernedEdgeBindingService {
	return &governedEdgeBindingService{
		tenantRepo: tenantRepo, tenants: tenants, operators: operators,
		center: center, resolver: resolver,
	}
}

func (r *governedEdgeResolver) loadPrivateConnection(binding types.GovernedEdgeBinding) (types.GovernedEdgeConnection, error) {
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
	node, ok := nodes[binding.EdgeNodeID]
	if !ok || node.BaseURL == "" || node.Token == "" {
		return types.GovernedEdgeConnection{}, fmt.Errorf("Edge connection is not configured")
	}
	return types.GovernedEdgeConnection{
		EnterpriseID: binding.EnterpriseID, EdgeNodeID: binding.EdgeNodeID,
		SourceID: binding.SourceID, BindingID: binding.BindingID,
		Revision: binding.Revision, DeploymentRevision: binding.DeploymentRevision,
		BaseURL: node.BaseURL, Token: node.Token,
	}, nil
}

func (r *governedEdgeResolver) Resolve(ctx context.Context, tenantID uint64) (types.GovernedEdgeConnection, error) {
	var tenant types.Tenant
	if err := r.db.WithContext(ctx).Select("id", "status", "analysis_enabled", "governed_edge_binding").First(&tenant, tenantID).Error; err != nil {
		return types.GovernedEdgeConnection{}, err
	}
	b := tenant.GovernedEdgeBinding
	if tenant.Status != types.TenantStatusActive || !tenant.AnalysisEnabled || b == nil || !b.Enabled {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	return r.loadPrivateConnection(*b)
}

func (r *governedEdgeResolver) VerifyCandidate(ctx context.Context, binding types.GovernedEdgeBinding) error {
	connection, err := r.loadPrivateConnection(binding)
	if err != nil {
		return err
	}
	return tools.VerifyGovernedDataCandidate(ctx, connection)
}

func validateGovernedEdgeCommand(tenantID uint64, actorUserID string, expectedRevision int64, edgeNodeID, sourceID string, deploymentRevision int64) error {
	if tenantID == 0 || actorUserID == "" || expectedRevision < 1 || edgeNodeID == "" || sourceID == "" || deploymentRevision < 1 {
		return fmt.Errorf("invalid governed Edge binding")
	}
	return nil
}

func candidateBinding(tenant *types.Tenant, edgeNodeID, sourceID string, deploymentRevision int64) (*types.GovernedEdgeBinding, error) {
	if tenant == nil || tenant.Status != types.TenantStatusActive || !tenant.AnalysisEnabled ||
		tenant.GovernedEnterpriseID == nil || tenant.GovernedEdgeBinding == nil ||
		tenant.GovernedEdgeBinding.BindingID == "" ||
		tenant.GovernedEdgeBinding.EnterpriseID != *tenant.GovernedEnterpriseID {
		return nil, apprepo.ErrGovernedEdgeBindingConflict
	}
	candidate := *tenant.GovernedEdgeBinding
	candidate.EdgeNodeID = edgeNodeID
	candidate.SourceID = sourceID
	candidate.DeploymentRevision = deploymentRevision
	candidate.Enabled = false
	return &candidate, nil
}

func centerObservedCandidate(observation *interfaces.CenterEnterpriseEdge, candidate *types.GovernedEdgeBinding) bool {
	if observation == nil || candidate == nil || observation.EnterpriseID != candidate.EnterpriseID {
		return false
	}
	for _, node := range observation.Nodes {
		if node.EdgeNodeID == candidate.EdgeNodeID && node.ControlRevision == candidate.DeploymentRevision && node.Status != "disabled" {
			return true
		}
	}
	return false
}

func (s *governedEdgeBindingService) Prepare(ctx context.Context, command interfaces.GovernedEdgeBindingPrepareCommand) (*types.GovernedEdgeBinding, error) {
	if err := validateGovernedEdgeCommand(command.TenantID, command.ActorUserID, command.ExpectedRevision, command.EdgeNodeID, command.SourceID, command.DeploymentRevision); err != nil {
		return nil, err
	}
	if err := s.operators.ValidateSystemAdministrator(ctx, command.ActorUserID); err != nil {
		return nil, err
	}
	tenant, err := s.tenants.GetTenantByID(ctx, command.TenantID)
	if err != nil {
		return nil, err
	}
	candidate, err := candidateBinding(tenant, command.EdgeNodeID, command.SourceID, command.DeploymentRevision)
	if err != nil {
		return nil, err
	}
	current := tenant.GovernedEdgeBinding
	if !current.Enabled && current.Revision == command.ExpectedRevision+1 &&
		current.EdgeNodeID == command.EdgeNodeID && current.SourceID == command.SourceID &&
		current.DeploymentRevision == command.DeploymentRevision {
		return s.tenantRepo.PrepareGovernedEdgeBinding(ctx, command)
	}
	if current.Revision != command.ExpectedRevision ||
		(current.EdgeNodeID == command.EdgeNodeID && current.DeploymentRevision > command.DeploymentRevision) {
		return nil, apprepo.ErrGovernedEdgeBindingConflict
	}
	observation, err := s.center.GetEnterpriseEdge(ctx, candidate.EnterpriseID, command.ActorUserID)
	if err != nil {
		return nil, err
	}
	if !centerObservedCandidate(observation, candidate) {
		return nil, apprepo.ErrGovernedEdgeBindingConflict
	}
	return s.tenantRepo.PrepareGovernedEdgeBinding(ctx, command)
}

func (s *governedEdgeBindingService) Confirm(ctx context.Context, command interfaces.GovernedEdgeBindingConfirmCommand) (*types.GovernedEdgeBinding, error) {
	if err := validateGovernedEdgeCommand(command.TenantID, command.ActorUserID, command.ExpectedRevision, command.EdgeNodeID, command.SourceID, command.DeploymentRevision); err != nil {
		return nil, err
	}
	if err := s.operators.ValidateSystemAdministrator(ctx, command.ActorUserID); err != nil {
		return nil, err
	}
	tenant, err := s.tenants.GetTenantByID(ctx, command.TenantID)
	if err != nil {
		return nil, err
	}
	candidate, err := candidateBinding(tenant, command.EdgeNodeID, command.SourceID, command.DeploymentRevision)
	if err != nil {
		return nil, err
	}
	current := tenant.GovernedEdgeBinding
	if current.Enabled && current.Revision == command.ExpectedRevision+1 &&
		current.EdgeNodeID == command.EdgeNodeID && current.SourceID == command.SourceID &&
		current.DeploymentRevision == command.DeploymentRevision {
		return s.tenantRepo.ConfirmGovernedEdgeBinding(ctx, command)
	}
	if current.Enabled || current.Revision != command.ExpectedRevision ||
		current.EdgeNodeID != command.EdgeNodeID || current.SourceID != command.SourceID ||
		current.DeploymentRevision != command.DeploymentRevision {
		return nil, apprepo.ErrGovernedEdgeBindingConflict
	}
	if err := s.resolver.VerifyCandidate(ctx, *candidate); err != nil {
		return nil, err
	}
	observation, err := s.center.GetEnterpriseEdge(ctx, candidate.EnterpriseID, command.ActorUserID)
	if err != nil {
		return nil, err
	}
	if !centerObservedCandidate(observation, candidate) {
		return nil, apprepo.ErrGovernedEdgeBindingConflict
	}
	return s.tenantRepo.ConfirmGovernedEdgeBinding(ctx, command)
}

func (s *governedEdgeBindingService) Revoke(ctx context.Context, enterpriseID, edgeNodeID string, controlRevision int64) (*interfaces.EdgeNodeRevocationReceipt, error) {
	if enterpriseID == "" || edgeNodeID == "" || controlRevision < 1 {
		return nil, fmt.Errorf("invalid governed Edge revocation")
	}
	return s.tenantRepo.RevokeGovernedEdgeBinding(ctx, enterpriseID, edgeNodeID, controlRevision)
}
