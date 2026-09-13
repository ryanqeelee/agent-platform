package storageurl

import (
	"context"
	"os"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LocalStorageBaseDir is the on-disk root used when a reference resolves to the
// local provider.
func LocalStorageBaseDir() string {
	baseDir := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR"))
	if baseDir == "" {
		baseDir = "/data/files"
	}
	return baseDir
}

// FileServiceResolver resolves and caches one FileService per storage provider.
// The cache is scoped to a single request or outbound message so a long answer
// with many images does not re-create an SDK client per reference.
//
// Not safe for concurrent use; Rewriter drives it from one goroutine at a time.
type FileServiceResolver struct {
	defaultSvc      interfaces.FileService
	storageResolver interfaces.StorageBackendResolver
	ctx             context.Context
	cache           map[string]interfaces.FileService
}

// NewFileServiceResolver builds a resolver. defaultSvc resolves resource://
// handles; provider paths require the platform storage resolver.
func NewFileServiceResolver(
	defaultSvc interfaces.FileService,
	storageResolvers ...interfaces.StorageBackendResolver,
) *FileServiceResolver {
	resolver := &FileServiceResolver{
		defaultSvc: defaultSvc,
		ctx:        context.Background(),
		cache:      make(map[string]interfaces.FileService),
	}
	if len(storageResolvers) > 0 {
		resolver.storageResolver = storageResolvers[0]
	}
	return resolver
}

// WithContext sets the context used for backend lookups and their logs.
func (r *FileServiceResolver) WithContext(ctx context.Context) *FileServiceResolver {
	if ctx != nil {
		r.ctx = ctx
	}
	return r
}

// ResolveFileService implements Resolver.
func (r *FileServiceResolver) ResolveFileService(filePath string) interfaces.FileService {
	if _, ok := types.ParseResourcePath(filePath); ok {
		return r.defaultSvc
	}
	backendID, _, _ := types.ParseStorageBackendPath(filePath)
	provider := types.ParseProviderScheme(filePath)
	if provider == "" {
		return nil
	}
	cacheKey := backendID + ":" + provider
	if svc, ok := r.cache[cacheKey]; ok {
		return svc
	}
	if r.storageResolver != nil {
		svc, _, err := r.storageResolver.ResolveFileService(r.ctx, backendID, LocalStorageBaseDir())
		if err == nil {
			r.cache[cacheKey] = svc
			return svc
		}
		logger.Warnf(r.ctx, "resolve storage backend failed: backend_id=%s provider=%s err=%v",
			backendID, provider, err)
		return nil
	}
	return nil
}

var _ Resolver = (*FileServiceResolver)(nil)
