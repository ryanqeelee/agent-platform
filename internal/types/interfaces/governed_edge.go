package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

type GovernedEdgeResolver interface {
	Resolve(context.Context, uint64) (types.GovernedEdgeConnection, error)
}
