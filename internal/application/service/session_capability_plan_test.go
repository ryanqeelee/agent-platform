package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Tencent/WeKnora/internal/infrastructure/capabilityplan"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type capabilityPlanResolverStub struct {
	resolution *types.AICapabilityPlanResolution
	err        error
}

func (s capabilityPlanResolverStub) Resolve(context.Context, uint64) (*types.AICapabilityPlanResolution, error) {
	return s.resolution, s.err
}

type sessionPinRepository struct {
	interfaces.SessionRepository
	created *types.Session
}

func (r *sessionPinRepository) Create(_ context.Context, session *types.Session) (*types.Session, error) {
	r.created = session
	return session, nil
}

func TestCreateSessionPinsPlanBeforeRepositoryWrite(t *testing.T) {
	repo := &sessionPinRepository{}
	service := &sessionService{
		sessionRepo: repo,
		capabilityPlanResolver: capabilityPlanResolverStub{resolution: &types.AICapabilityPlanResolution{
			ContractVersion: "AICapabilityPlanV1",
			PlanVersionID:   "plan-v1",
		}},
	}

	created, err := service.CreateSession(context.Background(), &types.Session{TenantID: 7})
	require.NoError(t, err)
	require.Equal(t, "plan-v1", created.AICapabilityPlanVersionID)
	require.Same(t, created, repo.created)
}

func TestCreateSessionLeavesNoRowWhenPlanUnavailable(t *testing.T) {
	repo := &sessionPinRepository{}
	service := &sessionService{
		sessionRepo:            repo,
		capabilityPlanResolver: capabilityPlanResolverStub{err: interfaces.ErrAICapabilityUnavailable},
	}

	created, err := service.CreateSession(context.Background(), &types.Session{TenantID: 7})
	require.Nil(t, created)
	require.ErrorIs(t, err, interfaces.ErrAICapabilityUnavailable)
	require.Nil(t, repo.created)
}

func TestRingxunCapabilityPlanIntegrationPinsSession(t *testing.T) {
	baseURL := os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Skip("requires the Ringxun capability-plan integration fixture")
	}
	t.Setenv("RINGXUN_CAPABILITY_PLAN_BASE_URL", baseURL)
	t.Setenv(
		"RINGXUN_CAPABILITY_PLAN_SERVICE_TOKEN",
		os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_SERVICE_TOKEN"),
	)

	repo := &sessionPinRepository{}
	service := &sessionService{
		sessionRepo:            repo,
		capabilityPlanResolver: capabilityplan.NewClientFromEnv(),
	}
	created, err := service.CreateSession(context.Background(), &types.Session{TenantID: 7})
	require.NoError(t, err)
	require.Equal(t, os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_VERSION_ID"), created.AICapabilityPlanVersionID)
	require.Same(t, created, repo.created)
}

func TestRunnableSessionRejectsHistoricalMissingPlan(t *testing.T) {
	repo := &sessionPinRepository{}
	repo.SessionRepository = sessionGetRepository{session: &types.Session{ID: "session-old", TenantID: 7}}
	service := &sessionService{sessionRepo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	_, err := service.GetRunnableSession(ctx, "session-old")
	require.True(t, errors.Is(err, interfaces.ErrConversationPlanMissing))
}

type sessionGetRepository struct {
	interfaces.SessionRepository
	session *types.Session
}

func (r sessionGetRepository) Get(context.Context, uint64, string, string) (*types.Session, error) {
	return r.session, nil
}
