package service

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var ErrNoTenantMemoryConfigScope = errors.New("tenant memory config: no tenant in context")

type tenantMemoryConfigService struct {
	repo interfaces.TenantMemoryConfigRepository
}

func NewTenantMemoryConfigService(
	repo interfaces.TenantMemoryConfigRepository,
) interfaces.TenantMemoryConfigService {
	return &tenantMemoryConfigService{repo: repo}
}

func tenantMemoryConfigScope(ctx context.Context) (uint64, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return 0, ErrNoTenantMemoryConfigScope
	}
	return tenantID, nil
}

func (s *tenantMemoryConfigService) Get(ctx context.Context) (*interfaces.TenantMemoryConfigState, error) {
	tenantID, err := tenantMemoryConfigScope(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, tenantID)
}

func (s *tenantMemoryConfigService) Update(
	ctx context.Context, cfg *types.MemoryConfig,
) (*interfaces.TenantMemoryConfigState, error) {
	tenantID, err := tenantMemoryConfigScope(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, tenantID, cfg)
}
