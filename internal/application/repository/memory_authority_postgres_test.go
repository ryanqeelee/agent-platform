package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newMemoryAuthorityPostgres(t *testing.T) (*gorm.DB, interfaces.MemoryRepository) {
	t.Helper()
	db := newIsolatedPostgresTestDatabase(t, "weknora_memory_authority_")
	// Legacy migration 91 expects uuid-ossp. Keep its UUID default scoped to
	// this disposable schema using PostgreSQL's equivalent built-in generator.
	require.NoError(t, db.Exec(`CREATE FUNCTION uuid_generate_v4() RETURNS uuid
		LANGUAGE sql VOLATILE AS $$ SELECT pg_catalog.gen_random_uuid() $$`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE tenants (id BIGINT PRIMARY KEY, deleted_at TIMESTAMPTZ); CREATE TABLE messages (id VARCHAR(36) PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000091_memory.up.sql")).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000101_unified_personal_memory.up.sql")).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id, memory_config) VALUES
		(1, '{"enabled":true,"write_mode":"auto"}'::jsonb)`).Error)
	return db, NewMemoryRepository(db)
}

func authorityCreate(
	content string,
) interfaces.MemoryAuthorityCallback {
	return func(ctx context.Context, repo interfaces.MemoryRepository, _ interfaces.MemoryAuthorityState) (*interfaces.MemoryAuthorityMutation, error) {
		id := uuid.NewString()
		err := repo.CreateItem(ctx, &types.MemoryItem{
			ID: id, TenantID: 1, SubjectID: "web_user:alice", Scope: types.MemoryScopeEmployee,
			Kind: types.MemoryKindFact, Content: content, NormalizedKey: id,
			Status: types.MemoryStatusActive, Origin: types.MemoryOriginExtracted, Importance: 3,
		})
		if err != nil {
			return nil, err
		}
		return &interfaces.MemoryAuthorityMutation{
			Mutated: true, BumpRevision: true, ItemIDs: []string{id},
		}, nil
	}
}

func TestPostgresAuthoritySerializesConcurrentFirstWriteAndReceiptReplay(t *testing.T) {
	_, repo := newMemoryAuthorityPostgres(t)
	scope := interfaces.MemoryScope{TenantID: 1, SubjectID: "web_user:alice"}
	request := interfaces.MemoryAuthorityRequest{
		Expected: &types.MemoryPolicyVersion{}, RequireEnabled: true, RequireAuto: true,
		OperationID: "same-operation", CommandHash: "same-hash",
	}

	const workers = 8
	start := make(chan struct{})
	receipts := make(chan *types.PersonalMemoryReceipt, workers)
	errs := make(chan error, workers)
	var callbacks atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			receipt, err := repo.WithAuthority(t.Context(), scope, request, func(
				ctx context.Context, txRepo interfaces.MemoryRepository, state interfaces.MemoryAuthorityState,
			) (*interfaces.MemoryAuthorityMutation, error) {
				callbacks.Add(1)
				return authorityCreate("one concurrent item")(ctx, txRepo, state)
			})
			receipts <- receipt
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for receipt := range receipts {
		require.NotNil(t, receipt)
		require.Equal(t, types.MemoryReceiptApplied, receipt.Status)
		require.Equal(t, int64(1), receipt.Revision)
	}
	require.Equal(t, int32(1), callbacks.Load())
	items, total, err := repo.ListItems(t.Context(), scope, types.MemoryStatusActive, 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
}

func TestPostgresAuthorityRejectsOneOfTwoConcurrentCommandsAtSameRevision(t *testing.T) {
	_, repo := newMemoryAuthorityPostgres(t)
	scope := interfaces.MemoryScope{TenantID: 1, SubjectID: "web_user:bob"}
	start := make(chan struct{})
	receipts := make(chan *types.PersonalMemoryReceipt, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index := range 2 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			request := interfaces.MemoryAuthorityRequest{
				Expected: &types.MemoryPolicyVersion{}, RequireEnabled: true,
				OperationID: "different-" + string(rune('a'+index)), CommandHash: "hash-" + string(rune('a'+index)),
			}
			receipt, err := repo.WithAuthority(t.Context(), scope, request,
				func(ctx context.Context, txRepo interfaces.MemoryRepository, _ interfaces.MemoryAuthorityState) (*interfaces.MemoryAuthorityMutation, error) {
					id := uuid.NewString()
					if err := txRepo.CreateItem(ctx, &types.MemoryItem{
						ID: id, TenantID: 1, SubjectID: scope.SubjectID, Scope: types.MemoryScopeEmployee,
						Kind: types.MemoryKindFact, Content: "concurrent", NormalizedKey: id,
						Status: types.MemoryStatusActive, Origin: types.MemoryOriginExtracted, Importance: 3,
					}); err != nil {
						return nil, err
					}
					return &interfaces.MemoryAuthorityMutation{Mutated: true, BumpRevision: true}, nil
				})
			receipts <- receipt
			errs <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(receipts)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	statuses := map[string]int{}
	reasons := map[string]int{}
	for receipt := range receipts {
		statuses[receipt.Status]++
		if receipt.ReasonCode != nil {
			reasons[*receipt.ReasonCode]++
		}
	}
	require.Equal(t, 1, statuses[types.MemoryReceiptApplied])
	require.Equal(t, 1, statuses[types.MemoryReceiptRejected])
	require.Equal(t, 1, reasons[types.MemoryReasonRevisionConflict])
}

func TestPostgresAuthorityRollsBackMutationAndReceiptTogether(t *testing.T) {
	db, repo := newMemoryAuthorityPostgres(t)
	scope := interfaces.MemoryScope{TenantID: 1, SubjectID: "web_user:rollback"}
	sentinel := errors.New("forced rollback")
	_, err := repo.WithAuthority(t.Context(), scope, interfaces.MemoryAuthorityRequest{
		Expected: &types.MemoryPolicyVersion{}, RequireEnabled: true,
		OperationID: "rollback", CommandHash: "rollback-hash",
	}, func(ctx context.Context, txRepo interfaces.MemoryRepository, _ interfaces.MemoryAuthorityState) (*interfaces.MemoryAuthorityMutation, error) {
		if err := txRepo.CreateItem(ctx, &types.MemoryItem{
			ID: uuid.NewString(), TenantID: 1, SubjectID: scope.SubjectID, Scope: types.MemoryScopeEmployee,
			Kind: types.MemoryKindFact, Content: "must roll back", NormalizedKey: "rollback",
			Status: types.MemoryStatusActive, Origin: types.MemoryOriginExtracted, Importance: 3,
		}); err != nil {
			return nil, err
		}
		return nil, sentinel
	})
	require.ErrorIs(t, err, sentinel)
	var itemCount, receiptCount int64
	require.NoError(t, db.Model(&types.MemoryItem{}).Where("subject_id = ?", scope.SubjectID).Count(&itemCount).Error)
	require.NoError(t, db.Model(&types.MemoryCommandReceipt{}).Where("subject_id = ?", scope.SubjectID).Count(&receiptCount).Error)
	require.Zero(t, itemCount)
	require.Zero(t, receiptCount)
}

func TestPostgresAuthorityPreservesOpaqueSourceIDs(t *testing.T) {
	_, repo := newMemoryAuthorityPostgres(t)
	ctx := t.Context()
	scope := interfaces.MemoryScope{TenantID: 1, SubjectID: "web_user:source-ids"}
	sourceID := strings.Repeat("s", 128)
	receipt, err := repo.WithAuthority(ctx, scope, interfaces.MemoryAuthorityRequest{RequireEnabled: true}, func(ctx context.Context, tx interfaces.MemoryRepository, _ interfaces.MemoryAuthorityState) (*interfaces.MemoryAuthorityMutation, error) {
		item := &types.MemoryItem{ID: uuid.NewString(), TenantID: scope.TenantID, SubjectID: scope.SubjectID, Scope: types.MemoryScopeShared, Kind: types.MemoryKindPreference, Content: "prefer concise answers", Status: types.MemoryStatusActive, Importance: 3, Origin: types.MemoryOriginManual, SourceSessionID: sourceID, SourceMessageID: sourceID}
		if err := tx.CreateItem(ctx, item); err != nil {
			return nil, err
		}
		if err := tx.AddTombstone(ctx, scope, "", types.MemoryFingerprint(item.Content), sourceID); err != nil {
			return nil, err
		}
		return &interfaces.MemoryAuthorityMutation{Mutated: true, BumpRevision: true, ItemIDs: []string{item.ID}}, nil
	})
	require.NoError(t, err)
	require.Equal(t, types.MemoryReceiptApplied, receipt.Status)
	item, err := repo.GetItem(ctx, scope, receipt.ItemIDs[0])
	require.NoError(t, err)
	require.Equal(t, sourceID, item.SourceSessionID)
	require.Equal(t, sourceID, item.SourceMessageID)
	found, err := repo.HasTombstoneForMessage(ctx, scope, sourceID, time.Hour)
	require.NoError(t, err)
	require.True(t, found)
}
