package service

import (
	"context"
	"fmt"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
)

func (s *agentService) registerGovernedDataTools(ctx context.Context, registry *tools.ToolRegistry, cfg *types.AgentConfig, sessionID string) error {
	wanted := map[string]bool{}
	for _, name := range cfg.AllowedTools {
		if name == tools.ToolGovernedDataSchema || name == tools.ToolGovernedDataQuery {
			wanted[name] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	if !wanted[tools.ToolGovernedDataSchema] || !wanted[tools.ToolGovernedDataQuery] {
		return fmt.Errorf("select both business data schema and query tools")
	}
	connection, err := AuthorizeGovernedData(ctx, s.userService, s.tenantMemberService, s.governedEdgeResolver)
	if err != nil {
		return err
	}
	client, err := tools.NewGovernedDataClient(connection, func(callCtx context.Context) error {
		current, err := AuthorizeGovernedData(callCtx, s.userService, s.tenantMemberService, s.governedEdgeResolver)
		if err != nil {
			return err
		}
		if current != connection {
			return tools.ErrGovernedDataAccessDenied
		}
		return nil
	})
	if err != nil {
		return err
	}
	var files sandbox.SessionFileStore
	if !employeeSandboxDisabled(cfg) {
		manager, err := s.resolveWorkspaceSandbox(ctx, sessionID, cfg)
		if err != nil {
			return fmt.Errorf("resolve business analysis sandbox: %w", err)
		}
		if manager != nil {
			files = sessionSandboxFileStore(manager)
		}
	}
	for _, tool := range tools.NewGovernedDataTools(client, files, sessionID) {
		registry.RegisterTool(tool)
	}
	return nil
}

// AuthorizeGovernedData uses the existing live member grant and server-owned
// connection binding. No browser token is forwarded to the Edge.
func AuthorizeGovernedData(ctx context.Context, users interfaces.UserService, members interfaces.TenantMemberService, resolver interfaces.GovernedEdgeResolver) (types.GovernedEdgeConnection, error) {
	bearer, tenantID, authenticated := types.GovernedDataUserCredential(ctx)
	if !authenticated || users == nil {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	user, _, err := users.ValidateToken(ctx, bearer)
	actorID, _ := types.UserIDFromContext(ctx)
	if err != nil || user == nil || user.ID != actorID || !user.IsActive {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	allowed, err := currentMemberCanReadOperatingAnalysis(ctx, members)
	if err != nil {
		return types.GovernedEdgeConnection{}, err
	}
	if !allowed {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	if resolver == nil {
		return types.GovernedEdgeConnection{}, fmt.Errorf("business data connection is not configured")
	}
	return resolver.Resolve(ctx, tenantID)
}
