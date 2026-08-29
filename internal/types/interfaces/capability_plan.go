package interfaces

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	ErrAICapabilityUnavailable = errors.New("AI capability unavailable")
	ErrConversationPlanMissing = errors.New("conversation AI capability plan missing")
)

type AICapabilityPlanResolver interface {
	Resolve(ctx context.Context, tenantID uint64) (*types.AICapabilityPlanResolution, error)
}
