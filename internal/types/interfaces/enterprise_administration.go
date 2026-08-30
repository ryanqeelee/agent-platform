package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type EnterpriseAdministrationQueueResolver interface {
	ResolveEnterpriseAdministrationQueue(
		ctx context.Context,
		tenantID uint64,
		facts types.EnterpriseAdministrationFacts,
	) (*types.EnterpriseAdministrationPlatformProjection, error)
}

type EnterpriseAdministrationService interface {
	Resolve(ctx context.Context) (*types.EnterpriseAdministrationQueue, error)
}
