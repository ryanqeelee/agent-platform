package service

import (
	"context"
	"fmt"

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
	client, admitted := tools.GovernedAnalysisClientFromContext(ctx)
	if !admitted || client.RunID() == "" {
		return fmt.Errorf("business analysis run was not admitted")
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
