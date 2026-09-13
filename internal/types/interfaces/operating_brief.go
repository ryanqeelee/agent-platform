package interfaces

import "context"

// OperatingBriefComputeClient is the authenticated pure-compute boundary.
// Platform supplies all governed inputs and retains authorization and storage.
type OperatingBriefComputeClient interface {
	Compute(context.Context, map[string]any) (map[string]any, error)
}
