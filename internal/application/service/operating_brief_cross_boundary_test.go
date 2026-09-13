package service

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The fixture is the real authenticated Python compute endpoint response over
// test_operating_brief_source's existing fixed publication fixture.
func TestOperatingBriefPythonProjectionPersistsAndReadsWithoutCompute(t *testing.T) {
	path := os.Getenv("WEKNORA_TEST_BRIEF_COMPUTE_RESPONSE")
	if path == "" {
		t.Skip("set WEKNORA_TEST_BRIEF_COMPUTE_RESPONSE to the Python endpoint response")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE tenants (id INTEGER PRIMARY KEY); INSERT INTO tenants VALUES (10001)").Error)
	migration, err := os.ReadFile("../../../migrations/sqlite/000027_operating_brief_native.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(migration)).Error)
	repo := apprepo.NewOperatingBriefRepository(db)
	members := &governedTestMembers{allowed: true}
	connection := types.GovernedEdgeConnection{SourceID: "source", BindingID: "binding", Revision: 1, DeploymentRevision: 1}
	service := NewOperatingBriefService(repo, members, governedTestTenants{}, &governedTestResolver{connection: connection}, nil, nil)
	ctx := operatingReadContext("user-a", 10001)
	now := time.Now().UTC()
	refresh := &types.OperatingBriefRefresh{ID: uuid.NewString(), TenantID: 10001, RequesterUserID: "user-a", ScopeKind: types.OperatingBriefScopeAll, Status: types.OperatingBriefRefreshQueued, CreatedAt: now, UpdatedAt: now}
	created, err := repo.CreateRefresh(ctx, refresh)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, repo.MarkRefreshRunning(ctx, refresh.ID))
	payload := types.OperatingBriefRefreshPayload{RefreshID: refresh.ID, TenantID: 10001, RequesterUserID: "user-a", Scope: types.OperatingBriefScope{Kind: types.OperatingBriefScopeAll}}
	require.NoError(t, service.persistProjection(ctx, payload, connection, map[string]any{"catalog": map[string]any{"version": "catalog", "freshness_token": "fresh"}}, result))
	service.resolver = nil // Historical read/handoff must not resolve a live connection.
	public, err := service.Read(ctx, "")
	require.NoError(t, err)
	require.NotEmpty(t, public["weeklyCore"].(map[string]any)["data"])
	snapshot, err := repo.LatestSnapshot(ctx, 10001, payload.Scope)
	require.NoError(t, err)
	var questions map[string]string
	require.NoError(t, json.Unmarshal(snapshot.HandoffQuestions, &questions))
	require.Len(t, questions, len(result["projection"].(map[string]any)["handoffQuestions"].(map[string]any)))
	require.NotEmpty(t, questions)
	for anchor, question := range questions {
		_, err := uuid.Parse(anchor)
		require.NoError(t, err)
		handoff, err := service.CreateHandoff(ctx, snapshot.SnapshotRef, anchor)
		require.NoError(t, err)
		require.Equal(t, question, handoff["question"])
		members.allowed = false
		_, err = service.Read(ctx, "")
		require.Error(t, err)
		_, err = service.CreateHandoff(ctx, snapshot.SnapshotRef, anchor)
		require.Error(t, err)
		break
	}
}
