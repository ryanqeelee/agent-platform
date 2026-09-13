package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type StorageBackendRepository interface {
	Create(ctx context.Context, backend *types.StorageBackend) error
	GetByID(ctx context.Context, id string) (*types.StorageBackend, error)
	GetDefault(ctx context.Context) (*types.StorageBackend, error)
	List(ctx context.Context) ([]*types.StorageBackend, error)
	Update(ctx context.Context, backend *types.StorageBackend) error
	Delete(ctx context.Context, id string) error
}

type StorageBackendService interface {
	Create(ctx context.Context, backend *types.StorageBackend) error
	Update(ctx context.Context, backend *types.StorageBackend) error
	Delete(ctx context.Context, id string) error
	SetDefault(ctx context.Context, id string) error
	Test(ctx context.Context, backend *types.StorageBackend) error
}

// StorageBackendResolver is the single runtime entry point for resolving one
// concrete storage instance. Explicit backendID wins; only an unset ID may use
// the platform default. Tenant remains an input solely for data namespacing.
type StorageBackendResolver interface {
	ResolveFileService(ctx context.Context, backendID, localBaseDir string) (FileService, string, error)
	ResolveBackend(ctx context.Context, backendID string) (*types.StorageBackend, error)
}
