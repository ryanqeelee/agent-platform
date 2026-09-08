package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type TenantMemoryConfigState struct {
	Config     *types.MemoryConfig
	Generation int64
}

// TenantMemoryConfigRepository owns the two memory-policy columns. General
// tenant updates deliberately cannot write either column.
type TenantMemoryConfigRepository interface {
	Get(ctx context.Context, tenantID uint64) (*TenantMemoryConfigState, error)
	Update(ctx context.Context, tenantID uint64, cfg *types.MemoryConfig) (*TenantMemoryConfigState, error)
}

type TenantMemoryConfigService interface {
	Get(ctx context.Context) (*TenantMemoryConfigState, error)
	Update(ctx context.Context, cfg *types.MemoryConfig) (*TenantMemoryConfigState, error)
}
