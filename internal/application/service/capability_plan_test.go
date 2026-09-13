package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type capabilityPlanServiceRepo struct {
	repository.CapabilityPlanRepository
	plan     *types.ResolvedAICapabilityPlan
	scenario *types.ResolvedAssistantScenarioCapabilities
	tenant   *types.Tenant
}

func (r capabilityPlanServiceRepo) ResolvePlan(context.Context, uint64) (*types.ResolvedAICapabilityPlan, error) {
	return r.plan, nil
}

func (r capabilityPlanServiceRepo) ResolveScenario(context.Context, uint64) (*types.ResolvedAssistantScenarioCapabilities, error) {
	return r.scenario, nil
}

func (r capabilityPlanServiceRepo) GetTenant(context.Context, uint64) (*types.Tenant, error) {
	return r.tenant, nil
}

func TestCapabilityPlanServicePreservesOpaqueRefsAcrossResolverShapes(t *testing.T) {
	svc := NewCapabilityPlanService(capabilityPlanServiceRepo{plan: &types.ResolvedAICapabilityPlan{
		Source: "tenant_assignment",
		Plan: types.AICapabilityPlanVersion{
			VersionID: "plan-v1", ContractVersion: types.AICapabilityPlanContractVersion,
			ServiceLevel: "standard", EmployeeAssistantRequestRuntimeRef: "provider/model:alias",
			OperatingAnalysisRequestRuntimeRef: "analysis/runtime:alias",
			EmbeddingRef:                       "embedding::opaque", RerankingRef: "reranking::opaque", ParsingRef: "parser::opaque",
			CreatedBy: "import", CreatedAt: time.Now(),
		},
	}})
	model, err := svc.ResolvePlatformModelRuntimeSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "provider/model:alias", model.RequestRuntimeRefs.EmployeeAssistantRequestRuntime)
	retrieval, err := svc.ResolvePlatformRetrievalProcessingSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "embedding::opaque", retrieval.CapabilityRefs.Embedding)
}

func TestCapabilityPlanServiceScenarioDefaultAbsentIsAllFalse(t *testing.T) {
	svc := NewCapabilityPlanService(capabilityPlanServiceRepo{scenario: &types.ResolvedAssistantScenarioCapabilities{
		Source: "platform_default", Capabilities: types.AssistantScenarioCapabilities{},
	}})
	settings, err := svc.ResolveAssistantScenarioCapabilities(context.Background(), 7)
	require.NoError(t, err)
	require.False(t, settings.Capabilities.ExternalSearch)
	require.False(t, settings.Capabilities.MCP)
	require.False(t, settings.Capabilities.Tools)
}

func TestCapabilityPlanServiceMissingTenantOrPlanIsUnavailable(t *testing.T) {
	svc := NewCapabilityPlanService(capabilityPlanServiceRepo{})
	_, err := svc.Resolve(context.Background(), 7)
	require.ErrorIs(t, err, interfaces.ErrAICapabilityUnavailable)
	_, err = svc.Resolve(context.Background(), 0)
	require.ErrorIs(t, err, interfaces.ErrAICapabilityUnavailable)
}

func TestCapabilityPlanServiceQueueUsesLocalTenantFactsWithoutInventingEdgeNodes(t *testing.T) {
	one := 1
	svc := NewCapabilityPlanService(capabilityPlanServiceRepo{
		plan: &types.ResolvedAICapabilityPlan{Source: "platform_default", Plan: types.AICapabilityPlanVersion{
			VersionID: "plan-v1", ContractVersion: types.AICapabilityPlanContractVersion, ServiceLevel: "standard",
			EmployeeAssistantRequestRuntimeRef: "a", OperatingAnalysisRequestRuntimeRef: "b",
			EmbeddingRef: "c", RerankingRef: "d", ParsingRef: "e", CreatedBy: "import", CreatedAt: time.Now(),
		}},
		tenant: &types.Tenant{ID: 7, Status: types.TenantStatusActive, SeatsTotal: &one},
	})
	projection, err := svc.ResolveEnterpriseAdministrationQueue(context.Background(), 7, types.EnterpriseAdministrationFacts{
		Role: "admin", ActiveMemberCount: 1, OperatingAnalysisMissingAccessCount: 1,
	})
	require.NoError(t, err)
	require.Nil(t, projection.EdgeNodes)
	require.Equal(t, "active", projection.Summary.Status)
	require.Equal(t, "unknown", projection.Summary.Health)
	require.Len(t, projection.Items, 1)
	require.Equal(t, "operating_analysis_access_gap", projection.Items[0].Code)
}
