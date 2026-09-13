package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOperatingBriefPostgresCurrentRefreshRejectsStaleAndDuplicateSave(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_operating_brief_")
	require.NoError(t, db.Exec(`CREATE TABLE tenants (id bigint PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id) VALUES (1)`).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000106_operating_brief_native.up.sql")).Error)
	repo := NewOperatingBriefRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	scope := types.OperatingBriefScope{Kind: types.OperatingBriefScopeAll}

	oldRefresh := briefRefreshForTest(uuid.NewString(), now)
	created, err := repo.CreateRefresh(ctx, oldRefresh)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, repo.MarkRefreshRunning(ctx, oldRefresh.ID))
	require.ErrorIs(t, repo.MarkRefreshRunning(ctx, oldRefresh.ID), ErrOperatingBriefSnapshotNotFound)
	require.NoError(t, repo.MarkRefreshFailed(ctx, oldRefresh.ID, "failed"))

	currentRefresh := briefRefreshForTest(uuid.NewString(), now.Add(time.Second))
	created, err = repo.CreateRefresh(ctx, currentRefresh)
	require.NoError(t, err)
	require.True(t, created)
	require.Error(t, repo.SaveSnapshot(ctx, oldRefresh.ID, briefSnapshotForTest(uuid.NewString(), now.Add(2*time.Second)), nil))

	require.NoError(t, repo.MarkRefreshRunning(ctx, currentRefresh.ID))
	currentSnapshot := briefSnapshotForTest(uuid.NewString(), now.Add(3*time.Second))
	require.NoError(t, repo.SaveSnapshot(ctx, currentRefresh.ID, currentSnapshot, nil))
	require.Error(t, repo.SaveSnapshot(ctx, currentRefresh.ID, briefSnapshotForTest(uuid.NewString(), now.Add(4*time.Second)), nil))
	require.NoError(t, repo.MarkRefreshFailed(ctx, currentRefresh.ID, "late_failure"))

	storedSnapshot, err := repo.LatestSnapshot(ctx, 1, scope)
	require.NoError(t, err)
	require.Equal(t, currentSnapshot.SnapshotRef, storedSnapshot.SnapshotRef)
	storedRefresh, err := repo.LatestRefresh(ctx, 1, scope)
	require.NoError(t, err)
	require.Equal(t, types.OperatingBriefRefreshSucceeded, storedRefresh.Status)
}

func briefRefreshForTest(id string, now time.Time) *types.OperatingBriefRefresh {
	return &types.OperatingBriefRefresh{
		ID: id, TenantID: 1, RequesterUserID: "member", ScopeKind: types.OperatingBriefScopeAll,
		Status: types.OperatingBriefRefreshQueued, CreatedAt: now, UpdatedAt: now,
	}
}

func briefSnapshotForTest(ref string, now time.Time) *types.OperatingBriefSnapshot {
	return &types.OperatingBriefSnapshot{
		SnapshotRef: ref, TenantID: 1, ScopeKind: types.OperatingBriefScopeAll,
		SourceID: "source", BindingID: "binding", BindingRevision: 1, DeploymentRevision: 1,
		CatalogVersion: "catalog", FreshnessToken: "freshness", State: "ready", Quality: "complete",
		Data: types.JSON(`{}`), HandoffQuestions: types.JSON(`{}`), SnapshotMetadata: types.JSON(`[]`),
		FixedSlots: types.JSON(`[]`), GeneratedAt: now,
	}
}
