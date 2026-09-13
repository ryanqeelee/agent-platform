package interfaces

import (
	"context"
	"io"
)

// PlatformSkillArchiveStore persists platform-global skill bundles in the
// storage driver's platform/skills namespace and returns canonical storage://
// references. The skill domain never creates enterprise resource rows.
type PlatformSkillArchiveStore interface {
	Put(ctx context.Context, key string, data []byte) (string, error)
	Open(ctx context.Context, ref string) (io.ReadCloser, error)
	Delete(ctx context.Context, ref string) error
}
