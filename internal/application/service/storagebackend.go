package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StorageBackendService struct {
	repo            interfaces.StorageBackendRepository
	db              *gorm.DB
	resourceCatalog interfaces.ResourceCatalog
}

func NewStorageBackendService(
	repo interfaces.StorageBackendRepository,
	db *gorm.DB,
	catalog interfaces.ResourceCatalog,
) *StorageBackendService {
	return &StorageBackendService{repo: repo, db: db, resourceCatalog: catalog}
}

func (s *StorageBackendService) Create(ctx context.Context, backend *types.StorageBackend) error {
	if err := backend.Validate(); err != nil {
		return err
	}
	if err := validateStorageBackendEndpoint(backend); err != nil {
		return err
	}
	if err := s.Test(ctx, backend); err != nil {
		return apperrors.NewBadRequestError("storage connection test failed").WithDetails(secutils.SanitizeStorageConnectivityError(err))
	}
	backend.CreatedAt, backend.UpdatedAt = time.Now(), time.Now()
	if err := s.repo.Create(ctx, backend); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return apperrors.NewConflictError("a storage backend with this name already exists")
		}
		return err
	}
	return nil
}

func (s *StorageBackendService) Update(ctx context.Context, incoming *types.StorageBackend) error {
	existing, err := s.repo.GetByID(ctx, incoming.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperrors.NewNotFoundError("storage backend not found")
	}
	incoming.Provider = existing.Provider
	incoming.Config = incoming.Config.MergeSecrets(existing.Config)
	if incoming.Config.LocationKey(existing.Provider) != existing.Config.LocationKey(existing.Provider) {
		return apperrors.NewBadRequestError("endpoint, region, bucket and path prefix are immutable; use storage migration instead")
	}
	if incoming.Status == "" {
		incoming.Status = existing.Status
	}
	if err := incoming.Validate(); err != nil {
		return err
	}
	if err := validateStorageBackendEndpoint(incoming); err != nil {
		return err
	}
	if err := s.Test(ctx, incoming); err != nil {
		return apperrors.NewBadRequestError("storage connection test failed").WithDetails(secutils.SanitizeStorageConnectivityError(err))
	}
	incoming.UpdatedAt = time.Now()
	if incoming.Status == types.StorageBackendStatusDisabled {
		return s.disable(ctx, incoming)
	}
	return s.repo.Update(ctx, incoming)
}

func (s *StorageBackendService) disable(ctx context.Context, incoming *types.StorageBackend) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStorageDefaultChange(tx); err != nil {
			return err
		}
		var current types.StorageBackend
		query := tx.Where("id = ?", incoming.ID)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("storage backend not found")
			}
			return err
		}
		if current.IsDefault {
			return apperrors.NewBadRequestError("a default storage backend cannot be disabled")
		}
		var references int64
		if err := tx.Model(&types.KnowledgeBase{}).Where("storage_backend_id = ?", incoming.ID).Count(&references).Error; err != nil {
			return err
		}
		if references == 0 {
			if err := tx.Model(&types.StoredResource{}).
				Where("storage_backend_id = ? AND state = ?", incoming.ID, types.ResourceStateActive).
				Count(&references).Error; err != nil {
				return err
			}
		}
		if references == 0 {
			var err error
			references, err = countStorageBackendSkillRefs(tx, incoming.ID)
			if err != nil {
				return err
			}
		}
		if references > 0 {
			return apperrors.NewBadRequestError("a bound storage backend cannot be disabled")
		}
		return tx.Model(&types.StorageBackend{}).Where("id = ?", incoming.ID).
			Select("name", "config", "status", "updated_at").Updates(incoming).Error
	})
}

func (s *StorageBackendService) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStorageDefaultChange(tx); err != nil {
			return err
		}
		var backend types.StorageBackend
		query := tx.Where("id = ?", id)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&backend).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("storage backend not found")
			}
			return err
		}
		if backend.IsDefault {
			return apperrors.NewBadRequestError("default storage backend cannot be deleted")
		}
		var kbCount int64
		if err := tx.Model(&types.KnowledgeBase{}).Where("storage_backend_id = ?", id).Count(&kbCount).Error; err != nil {
			return err
		}
		if kbCount > 0 {
			return apperrors.NewBadRequestError(fmt.Sprintf("storage backend still has %d knowledge base(s) bound to it", kbCount))
		}
		var resourceCount int64
		if err := tx.Model(&types.StoredResource{}).
			Where("storage_backend_id = ? AND state = ?", id, types.ResourceStateActive).
			Count(&resourceCount).Error; err != nil {
			return err
		}
		if resourceCount > 0 {
			return apperrors.NewBadRequestError(fmt.Sprintf("storage backend still has %d active resource(s)", resourceCount))
		}
		skillRefCount, err := countStorageBackendSkillRefs(tx, id)
		if err != nil {
			return err
		}
		if skillRefCount > 0 {
			return apperrors.NewBadRequestError(fmt.Sprintf("storage backend still has %d platform skill archive reference(s)", skillRefCount))
		}
		return tx.Delete(&backend).Error
	})
}

func countStorageBackendSkillRefs(tx *gorm.DB, backendID string) (int64, error) {
	prefix := types.BuildStorageBackendPath(backendID, "") + "%"
	var catalogCount int64
	if err := tx.Unscoped().Model(&types.TenantSkillCatalogEntity{}).Where("bundle_ref LIKE ?", prefix).Count(&catalogCount).Error; err != nil {
		return 0, err
	}
	var installCount int64
	if err := tx.Unscoped().Model(&types.TenantSkillEntity{}).Where("bundle_ref LIKE ?", prefix).Count(&installCount).Error; err != nil {
		return 0, err
	}
	return catalogCount + installCount, nil
}

func (s *StorageBackendService) SetDefault(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStorageDefaultChange(tx); err != nil {
			return err
		}
		var backend types.StorageBackend
		q := tx.Where("id = ?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&backend).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NewNotFoundError("storage backend not found")
			}
			return err
		}
		if backend.Status != types.StorageBackendStatusActive {
			return apperrors.NewBadRequestError("only an active storage backend can be the default")
		}
		if err := tx.Model(&types.StorageBackend{}).Where("is_default = ? AND id <> ?", true, id).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&types.StorageBackend{}).Where("id = ?", id).Update("is_default", true).Error
	})
}

func (s *StorageBackendService) Test(ctx context.Context, backend *types.StorageBackend) error {
	if err := backend.Validate(); err != nil {
		return err
	}
	if err := validateStorageBackendEndpoint(backend); err != nil {
		return err
	}
	if backend.Provider == "local" {
		baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
		if baseDir == "" {
			baseDir = "/data/files"
		}
		candidate := filepath.Join(baseDir, strings.Trim(strings.TrimSpace(backend.Config.PathPrefix), "/\\"))
		safeDir, err := secutils.SafePathUnderBase(baseDir, candidate)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(safeDir, 0o755); err != nil {
			return fmt.Errorf("create local storage directory: %w", err)
		}
	}
	c := backend.Config
	switch backend.Provider {
	case "local":
		fileService, _, err := filesvc.NewFileServiceFromStorageConfig("local", backend.ToStorageEngineConfig(), "")
		if err != nil {
			return err
		}
		return fileService.CheckConnectivity(ctx)
	case "minio":
		if c.Mode == "docker" {
			c.Endpoint = os.Getenv("MINIO_ENDPOINT")
			c.AccessKeyID = os.Getenv("MINIO_ACCESS_KEY_ID")
			c.SecretAccessKey = os.Getenv("MINIO_SECRET_ACCESS_KEY")
			if c.BucketName == "" {
				c.BucketName = os.Getenv("MINIO_BUCKET_NAME")
			}
		}
		return filesvc.CheckMinioConnectivity(ctx, c.Endpoint, c.AccessKeyID, c.SecretAccessKey, c.BucketName, c.UseSSL)
	case "cos":
		return filesvc.CheckCosConnectivity(ctx, c.BucketName, c.Region, c.AccessKeyID, c.SecretAccessKey)
	case "tos":
		return filesvc.CheckTosConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "s3":
		return filesvc.CheckS3ConnectivityWithOptions(ctx, c.Endpoint, c.AccessKeyID, c.SecretAccessKey, c.BucketName, c.Region, c.ForcePathStyle)
	case "oss":
		return filesvc.CheckOssConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "ks3":
		return filesvc.CheckKS3Connectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	case "obs":
		return filesvc.CheckObsConnectivity(ctx, c.Endpoint, c.Region, c.AccessKeyID, c.SecretAccessKey, c.BucketName)
	default:
		return fmt.Errorf("unsupported storage provider: %s", backend.Provider)
	}
}

func (s *StorageBackendService) ResolveBackend(ctx context.Context, backendID string) (*types.StorageBackend, error) {
	backendID = strings.TrimSpace(backendID)
	if backendID == "" {
		backend, err := s.repo.GetDefault(ctx)
		if err != nil {
			return nil, err
		}
		if backend == nil {
			return nil, fmt.Errorf("platform default storage backend is not configured")
		}
		if backend.Status != types.StorageBackendStatusActive {
			return nil, fmt.Errorf("storage backend is not active")
		}
		return backend, nil
	}
	backend, err := s.repo.GetByID(ctx, backendID)
	if err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, fmt.Errorf("storage backend not found")
	}
	if backend.Status != types.StorageBackendStatusActive {
		return nil, fmt.Errorf("storage backend is not active")
	}
	return backend, nil
}

const storageDefaultAdvisoryLockKey int64 = 0x53544f52414745

func lockStorageDefaultChange(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", storageDefaultAdvisoryLockKey).Error
}

func (s *StorageBackendService) ResolveFileService(ctx context.Context, backendID, localBaseDir string) (interfaces.FileService, string, error) {
	backend, err := s.ResolveBackend(ctx, backendID)
	if err != nil {
		return nil, "", err
	}
	inner, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(backend.Provider, backend.ToStorageEngineConfig(), localBaseDir)
	if err != nil {
		return nil, resolvedProvider, err
	}
	scoped := filesvc.NewBackendScopedFileService(backend.ID, inner)
	return filesvc.NewResourceCatalogFileService(scoped, s.resourceCatalog), resolvedProvider, nil
}

type platformSkillArchiveStore struct {
	storage *StorageBackendService
}

func NewPlatformSkillArchiveStore(storage *StorageBackendService) interfaces.PlatformSkillArchiveStore {
	return &platformSkillArchiveStore{storage: storage}
}

func (s *platformSkillArchiveStore) Put(ctx context.Context, key string, data []byte) (string, error) {
	objectKey, err := platformSkillObjectKey(key)
	if err != nil {
		return "", err
	}
	backend, err := s.storage.ResolveBackend(ctx, "")
	if err != nil {
		return "", err
	}
	inner, _, err := filesvc.NewFileServiceFromStorageConfig(backend.Provider, backend.ToStorageEngineConfig(), "")
	if err != nil {
		return "", err
	}
	providerRef, err := filesvc.PutPlatformSkillArchive(ctx, inner, objectKey, data)
	if err != nil {
		return "", err
	}
	return types.BuildStorageBackendPath(backend.ID, providerRef), nil
}

func (s *platformSkillArchiveStore) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	backend, providerRef, err := s.platformSkillArchiveTarget(ctx, ref)
	if err != nil {
		return nil, err
	}
	inner, _, err := filesvc.NewFileServiceFromStorageConfig(backend.Provider, backend.ToStorageEngineConfig(), "")
	if err != nil {
		return nil, err
	}
	if err := filesvc.ValidatePlatformSkillArchiveRef(inner, providerRef); err != nil {
		return nil, err
	}
	return inner.GetFile(ctx, providerRef)
}

func (s *platformSkillArchiveStore) Delete(ctx context.Context, ref string) error {
	backend, providerRef, err := s.platformSkillArchiveTarget(ctx, ref)
	if err != nil {
		return err
	}
	inner, _, err := filesvc.NewFileServiceFromStorageConfig(backend.Provider, backend.ToStorageEngineConfig(), "")
	if err != nil {
		return err
	}
	if err := filesvc.ValidatePlatformSkillArchiveRef(inner, providerRef); err != nil {
		return err
	}
	return inner.DeleteFile(ctx, providerRef)
}

func (s *platformSkillArchiveStore) platformSkillArchiveTarget(ctx context.Context, ref string) (*types.StorageBackend, string, error) {
	backendID, providerRef, ok := types.ParseStorageBackendPath(strings.TrimSpace(ref))
	if !ok {
		return nil, "", fmt.Errorf("platform skill archive reference must be storage-backend qualified")
	}
	backend, err := s.storage.ResolveBackend(ctx, backendID)
	if err != nil {
		return nil, "", err
	}
	if types.ParseProviderScheme(providerRef) != backend.Provider {
		return nil, "", fmt.Errorf("platform skill archive provider does not match its backend")
	}
	return backend, providerRef, nil
}

func platformSkillObjectKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if strings.Contains(key, "\\") {
		return "", fmt.Errorf("invalid platform skill archive key")
	}
	clean := path.Clean(key)
	if key == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("invalid platform skill archive key")
	}
	return "platform/skills/" + clean, nil
}

func validateStorageBackendEndpoint(backend *types.StorageBackend) error {
	if backend.Provider == "local" || (backend.Provider == "minio" && backend.Config.Mode == "docker") {
		return nil
	}
	endpoint := strings.TrimSpace(backend.Config.Endpoint)
	if backend.Provider == "cos" || endpoint == "" {
		return nil
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "https://"
		if backend.Provider == "minio" && !backend.Config.UseSSL {
			scheme = "http://"
		}
		endpoint = scheme + endpoint
	}
	if err := secutils.ValidateURLForSSRF(endpoint); err != nil {
		return apperrors.NewBadRequestError("storage endpoint failed SSRF validation").WithDetails(err.Error())
	}
	return nil
}

var (
	_ interfaces.StorageBackendService     = (*StorageBackendService)(nil)
	_ interfaces.StorageBackendResolver    = (*StorageBackendService)(nil)
	_ interfaces.PlatformSkillArchiveStore = (*platformSkillArchiveStore)(nil)
)
