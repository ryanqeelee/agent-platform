package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type GovernedEdgeResolver interface {
	Resolve(context.Context, uint64) (types.GovernedEdgeConnection, error)
	VerifyCandidate(context.Context, types.GovernedEdgeBinding) error
}

type GovernedEdgeBindingService interface {
	Prepare(context.Context, GovernedEdgeBindingPrepareCommand) (*types.GovernedEdgeBinding, error)
	Confirm(context.Context, GovernedEdgeBindingConfirmCommand) (*types.GovernedEdgeBinding, error)
	Revoke(context.Context, string, string, int64) (*EdgeNodeRevocationReceipt, error)
}
