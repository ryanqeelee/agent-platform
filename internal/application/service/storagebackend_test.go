package service_test

import (
	"context"
	"io"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newStorageBackendTestService(t *testing.T) (*service.StorageBackendService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.StorageBackend{}, &types.KnowledgeBase{}, &types.StoredResource{},
		&types.TenantSkillCatalogEntity{}, &types.TenantSkillEntity{},
	))
	return service.NewStorageBackendService(repository.NewStorageBackendRepository(db), db, nil), db
}

type staleStorageBackendRepository struct {
	interfaces.StorageBackendRepository
	getByID     func(context.Context, string) (*types.StorageBackend, error)
	updateCalls int
}

func (r *staleStorageBackendRepository) GetByID(ctx context.Context, id string) (*types.StorageBackend, error) {
	return r.getByID(ctx, id)
}

func (r *staleStorageBackendRepository) Update(ctx context.Context, backend *types.StorageBackend) error {
	r.updateCalls++
	return r.StorageBackendRepository.Update(ctx, backend)
}

func TestStorageBackendUpdateDisabledRechecksCurrentDefaultState(t *testing.T) {
	t.Setenv("LOCAL_STORAGE_BASE_DIR", t.TempDir())
	_, db := newStorageBackendTestService(t)
	baseRepo := repository.NewStorageBackendRepository(db)
	backend := &types.StorageBackend{
		Name: "Concurrent Default", Provider: "local", Status: types.StorageBackendStatusDisabled,
	}
	require.NoError(t, baseRepo.Create(context.Background(), backend))

	repo := &staleStorageBackendRepository{StorageBackendRepository: baseRepo}
	repo.getByID = func(ctx context.Context, id string) (*types.StorageBackend, error) {
		stale, err := baseRepo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		require.NoError(t, db.Model(&types.StorageBackend{}).Where("id = ?", id).Updates(map[string]any{
			"status": types.StorageBackendStatusActive, "is_default": true,
		}).Error)
		return stale, nil
	}
	svc := service.NewStorageBackendService(repo, db, nil)

	err := svc.Update(context.Background(), &types.StorageBackend{
		ID: backend.ID, Name: backend.Name, Status: types.StorageBackendStatusDisabled,
	})

	require.ErrorContains(t, err, "default storage backend cannot be disabled")
	require.Zero(t, repo.updateCalls, "disabled updates must use the locked guard path")
	current, err := baseRepo.GetByID(context.Background(), backend.ID)
	require.NoError(t, err)
	require.Equal(t, types.StorageBackendStatusActive, current.Status)
	require.True(t, current.IsDefault)
}

func TestStorageBackendResolverUsesGlobalConfigAndTenantNamespaces(t *testing.T) {
	resolver, db := newStorageBackendTestService(t)
	backend := &types.StorageBackend{Name: "Local", Provider: "local", Status: types.StorageBackendStatusActive}
	require.NoError(t, db.Create(backend).Error)
	require.NoError(t, resolver.SetDefault(context.Background(), backend.ID))

	fileSvc, provider, err := resolver.ResolveFileService(context.Background(), "", t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "local", provider)
	a, err := fileSvc.SaveBytes(context.Background(), []byte("a"), 101, "a.txt", false)
	require.NoError(t, err)
	b, err := fileSvc.SaveBytes(context.Background(), []byte("b"), 202, "b.txt", false)
	require.NoError(t, err)
	require.Contains(t, a, "/101/exports/")
	require.Contains(t, b, "/202/exports/")
	require.NotEqual(t, a, b)
}

func TestStorageBackendResolverRejectsExplicitUnknownWithoutDefaultFallback(t *testing.T) {
	resolver, db := newStorageBackendTestService(t)
	backend := &types.StorageBackend{Name: "Default", Provider: "local", Status: types.StorageBackendStatusActive}
	require.NoError(t, db.Create(backend).Error)
	require.NoError(t, resolver.SetDefault(context.Background(), backend.ID))
	_, err := resolver.ResolveBackend(context.Background(), "missing-id")
	require.ErrorContains(t, err, "not found")
}

func TestStorageBackendResolverRequiresConfiguredDefault(t *testing.T) {
	resolver, _ := newStorageBackendTestService(t)
	_, err := resolver.ResolveBackend(context.Background(), "")
	require.ErrorContains(t, err, "default storage backend is not configured")
}

func TestStorageBackendDeleteGuardsReferencesAcrossTenants(t *testing.T) {
	resolver, db := newStorageBackendTestService(t)
	backend := &types.StorageBackend{Name: "Bound", Provider: "local", Status: types.StorageBackendStatusActive}
	require.NoError(t, db.Create(backend).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{ID: "kb-other", TenantID: 202, Name: "KB", StorageBackendID: &backend.ID}).Error)
	err := resolver.Delete(context.Background(), backend.ID)
	require.ErrorContains(t, err, "knowledge base")
}

func TestStorageBackendDeleteGuardsPlatformSkillArchiveReferences(t *testing.T) {
	resolver, db := newStorageBackendTestService(t)
	backend := &types.StorageBackend{Name: "Archive", Provider: "local", Status: types.StorageBackendStatusActive}
	require.NoError(t, db.Create(backend).Error)
	ref := types.BuildStorageBackendPath(backend.ID, "local://platform/skills/catalog/demo.zip")
	require.NoError(t, db.Create(&types.TenantSkillCatalogEntity{
		ID: "catalog-1", Name: "demo", BundleRef: ref,
	}).Error)
	err := resolver.Delete(context.Background(), backend.ID)
	require.ErrorContains(t, err, "platform skill archive")
}

func TestPlatformSkillArchiveUsesDedicatedNamespaceAndRejectsOtherRefs(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("LOCAL_STORAGE_BASE_DIR", baseDir)
	resolver, db := newStorageBackendTestService(t)
	backend := &types.StorageBackend{Name: "Archive", Provider: "local", Status: types.StorageBackendStatusActive, Config: types.StorageBackendConfig{PathPrefix: "objects"}}
	require.NoError(t, db.Create(backend).Error)
	require.NoError(t, resolver.SetDefault(context.Background(), backend.ID))
	store := service.NewPlatformSkillArchiveStore(resolver)

	ref, err := store.Put(context.Background(), "catalog/demo.zip", []byte("archive"))
	require.NoError(t, err)
	require.Contains(t, ref, "storage://"+backend.ID+"/local://platform/skills/catalog/demo.zip")
	reader, err := store.Open(context.Background(), ref)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "archive", string(data))

	for _, bad := range []string{
		types.BuildStorageBackendPath(backend.ID, "local://tenant/202/private.zip"),
		types.BuildStorageBackendPath(backend.ID, "local://platform/skills/../../tenant/private.zip"),
		types.BuildStorageBackendPath(backend.ID, "local://platform\\skills\\private.zip"),
		types.BuildStorageBackendPath(backend.ID, "local://platform/skills/%2e%2e/private.zip"),
	} {
		require.Error(t, store.Delete(context.Background(), bad), bad)
	}
}
