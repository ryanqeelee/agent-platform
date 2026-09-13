package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

type catalogArchiveStore struct {
	objects map[string][]byte
	deleted []string
}

func (s *catalogArchiveStore) Put(_ context.Context, key string, data []byte) (string, error) {
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	ref := "storage://platform/skills/" + key
	s.objects[ref] = append([]byte(nil), data...)
	return ref, nil
}

func (s *catalogArchiveStore) Open(_ context.Context, ref string) (io.ReadCloser, error) {
	data, ok := s.objects[ref]
	if !ok {
		return nil, errors.New("archive not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *catalogArchiveStore) Delete(_ context.Context, ref string) error {
	delete(s.objects, ref)
	s.deleted = append(s.deleted, ref)
	return nil
}

func newCatalogService(t *testing.T) (*TenantSkillService, repository.TenantSkillRepository, *catalogArchiveStore) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.TenantSandboxConfigEntity{}, &types.TenantSkillEntity{},
		&types.TenantSkillCatalogEntity{}, &types.TenantSkillSnapshotEntity{},
		&types.TenantUserEnvVar{},
	))
	skills := repository.NewTenantSkillRepository(db)
	archives := &catalogArchiveStore{}
	svc := NewTenantSkillService(
		skills, repository.NewTenantSandboxConfigRepository(db), archives,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return svc, skills, archives
}

func TestInstallCatalogPreservesTheSuppliedCatalogID(t *testing.T) {
	svc, repo, archives := newCatalogService(t)
	ctx := context.Background()
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD})
	catalog, err := svc.RegisterCatalogFromArchive(ctx, archive)
	require.NoError(t, err)
	require.NoError(t, svc.configs.Create(ctx, &types.TenantSandboxConfigEntity{
		ID: "cfg-a", Name: "sandbox", SandboxType: "e2b",
	}))

	result, err := svc.InstallCatalogToConfigs(ctx, catalog.ID, []string{"cfg-a"})
	require.NoError(t, err)
	installed, err := repo.GetSkill(ctx, "cfg-a", result.Installs["cfg-a"])
	require.NoError(t, err)
	require.Equal(t, catalog.ID, installed.CatalogID)
	catalogs, err := repo.ListCatalogs(ctx)
	require.NoError(t, err)
	require.Len(t, catalogs, 1, "installing an existing definition must not create a duplicate")
	require.Len(t, archives.objects, 1)
}

func TestListCatalogGroupsPlatformInstallsByDefinition(t *testing.T) {
	svc, repo, _ := newCatalogService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateCatalog(ctx, &types.TenantSkillCatalogEntity{
		ID: "cat-pdf", Name: "pdf", Description: "extract",
	}))
	require.NoError(t, repo.CreateSkill(ctx, &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-a", CatalogID: "cat-pdf",
		Name: "pdf", Status: types.SkillStatusReady, Enabled: true,
	}))
	require.NoError(t, repo.CreateSkill(ctx, &types.TenantSkillEntity{
		ID: "sk-2", SandboxConfigID: "cfg-b", CatalogID: "cat-pdf",
		Name: "pdf", Status: types.SkillStatusInstalling, Enabled: true,
	}))

	list, err := svc.ListCatalog(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Len(t, list[0].Installations, 2)
}

func TestRegisterCatalogPreservesDuplicateNamesAsDistinctIDs(t *testing.T) {
	svc, repo, archives := newCatalogService(t)
	archive := zipBundle(t, map[string]string{"SKILL.md": validSkillMD})

	first, err := svc.RegisterCatalogFromArchive(context.Background(), archive)
	require.NoError(t, err)
	second, err := svc.RegisterCatalogFromArchive(context.Background(), archive)
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, first.Name, second.Name)
	require.NotEqual(t, first.BundleRef, second.BundleRef)
	require.Len(t, archives.objects, 2)
	rows, err := repo.ListCatalogs(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestDeleteCatalogRefusesWhileAnyInstallationReferencesIt(t *testing.T) {
	svc, repo, _ := newCatalogService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateCatalog(ctx, &types.TenantSkillCatalogEntity{
		ID: "cat-pdf", Name: "pdf",
	}))
	require.NoError(t, repo.CreateSkill(ctx, &types.TenantSkillEntity{
		ID: "sk-1", SandboxConfigID: "cfg-a", CatalogID: "cat-pdf",
		Name: "pdf", Status: types.SkillStatusReady, Enabled: true,
	}))

	err := svc.DeleteCatalog(ctx, "cat-pdf")
	appErr, ok := apperrors.IsAppError(err)
	require.True(t, ok, "error=%v", err)
	require.Equal(t, 409, appErr.HTTPCode)
}
