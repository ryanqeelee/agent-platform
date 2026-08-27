package session

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestPlatformModelOverride(t *testing.T) {
	assert.Empty(t, platformModelOverride(context.Background(), "model-secret"))

	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	assert.Equal(t, "model-secret", platformModelOverride(ctx, "model-secret"))
}
