package retriever

import (
	"context"
	"errors"
	"slices"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Sentinel errors returned by factory functions. Callers may use errors.Is to
// classify. User-facing responses MUST wrap or replace these with generic
// messages — the sentinels intentionally omit store UUIDs to avoid enumeration
// leaks. Structured logs inside the factory record the tenant/store IDs.
var (
	// ErrTenantInfoMissing is returned when the factory needs a tenant from
	// context (synchronous, unbound KB path) and none is present.
	ErrTenantInfoMissing = errors.New("tenant info not found in context")

	// ErrVectorStoreNotFound is returned when the store does not exist for the
	// tenant. Async workers should treat this as non-retryable: no amount of
	// waiting brings back a store that is not in the database.
	ErrVectorStoreNotFound = errors.New("vector store not available")

	// ErrVectorStoreUnavailable is returned when the store exists but its
	// engine could not be produced right now — the metadata database was
	// unreachable, or building the engine failed against a backend that may
	// simply be down. Async workers should retry rather than discard the task,
	// which is why this is a separate sentinel: reporting it as not-found
	// would turn a passing outage into permanently dropped work. It carries no
	// detail because the underlying errors embed endpoints and credentials;
	// the cause is logged where it happens.
	ErrVectorStoreUnavailable = errors.New("vector store engine unavailable")

	// ErrVectorStoreForbidden is returned when the resolved store is not
	// owned by the given tenant. This guards against cross-tenant access
	// in case the upstream validation layer has a gap. Async workers should
	// treat this as non-retryable.
	ErrVectorStoreForbidden = errors.New("vector store access denied")
)

// StoreConfigAvailability verifies platform-global vector configuration.
// Tenant data authorization remains in the repository/driver payload filters.
type StoreConfigAvailability interface {
	StoreUsable(ctx context.Context, storeID string) (bool, error)
	DefaultStoreID(ctx context.Context) (string, error)
}

// VerifyBinding asserts that a non-empty storeID is owned by tenantID and
// registered in the in-memory engine registry. It encapsulates the two
// checks that gate every store-bound resolution so that callers outside
// the retriever package (notably the KB create-validation path) can reuse
// the same sentinel hierarchy instead of duplicating the logic.
//
// Resolution rules:
//
//   - ownership.StoreOwnedBy returns an infrastructure error → that error
//     is returned verbatim so callers can decide retry/abort.
//   - ownership returns (false, nil) → ErrVectorStoreForbidden.
//   - ownership returns (true, nil) + registry.GetByStoreID fails →
//     ErrVectorStoreNotFound.
//   - all checks succeed → nil.
//
// VerifyBinding itself never echoes the store UUID; callers MUST wrap the
// sentinels into user-facing errors at the boundary (and log the
// tenant/store pair via structured fields when appropriate).
//
// resolveBoundEngine (below) intentionally does NOT delegate to VerifyBinding
// because it also needs the resolved engine service; sharing would require
// either a second registry lookup or returning the service from VerifyBinding,
// both of which dilute the helper's single purpose. The two paths are kept
// in lockstep by the factory_test.go matrix.
func VerifyBinding(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	availability StoreConfigAvailability,
	tenantID uint64,
	storeID string,
) error {
	usable, err := availability.StoreUsable(ctx, storeID)
	if err != nil {
		return err
	}
	if !usable {
		return ErrVectorStoreNotFound
	}
	if _, err := registry.GetOrLoadByStoreID(ctx, tenantID, storeID); err != nil {
		return classifyLookupError(err)
	}
	return nil
}

// classifyLookupError narrows an engine-lookup failure to what the caller is
// allowed to see, while preserving the distinction that decides whether work
// gets retried or discarded. Context errors and the store sentinels pass
// through; anything unexpected is reported as retryable, because treating an
// unknown failure as permanent is what silently drops work.
func classifyLookupError(err error) error {
	switch {
	case isContextError(err),
		errors.Is(err, ErrVectorStoreNotFound),
		errors.Is(err, ErrVectorStoreUnavailable),
		errors.Is(err, ErrVectorStoreForbidden):
		return err
	default:
		return ErrVectorStoreUnavailable
	}
}

// isContextError reports whether err is the caller giving up rather than a
// verdict about the store. The distinction matters because async workers treat
// the store sentinels as permanent and stop retrying, so a cancelled or
// timed-out request must not be reported as one.
func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// CreateRetrieveEngineForKB returns a CompositeRetrieveEngine resolved from
// a KB's VectorStore binding.
//
// Resolution rules:
//
//   - vectorStoreID == nil || *vectorStoreID == "" → the platform default
//     store ID is resolved from the typed configuration repository.
//   - otherwise →
//     1) ownership.StoreOwnedBy(*storeID, tenantID) must return true;
//     cross-tenant attempts yield ErrVectorStoreForbidden.
//     2) registry.GetByStoreID(*storeID) must succeed;
//     unregistered stores yield ErrVectorStoreNotFound.
//     3) the single engine is wrapped by NewCompositeRetrieveEngine so
//     that its Support()-based retriever-type matching is preserved.
//
// Use this for 23 synchronous call sites across the application services.
// Async task handlers that cannot rely on ctx-based TenantInfo (currently:
// ProcessKBDeleteTask, ProcessIndexDelete) must use
// CreateRetrieveEngineFromPayload instead.
func CreateRetrieveEngineForKB(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	availability StoreConfigAvailability,
	tenantID uint64,
	vectorStoreID *string,
) (*CompositeRetrieveEngine, error) {
	// Normalize nil and empty-string pointer to "unbound" so that callers
	// cannot accidentally route an empty UUID into GetByStoreID.
	if vectorStoreID == nil || *vectorStoreID == "" {
		storeID, err := availability.DefaultStoreID(ctx)
		if err != nil {
			return nil, err
		}
		if storeID == "" {
			return nil, ErrVectorStoreNotFound
		}
		return resolveBoundEngine(ctx, registry, availability, tenantID, storeID)
	}

	return resolveBoundEngine(ctx, registry, availability, tenantID, *vectorStoreID)
}

// CreateRetrieveEngineFromPayload is the async-task variant. It does not
// read TenantInfo from ctx because async handlers do not populate it.
// Instead, tenantID is passed explicitly from the deserialized payload and
// is verified against the store's tenant when vectorStoreID is non-empty.
//
// Tasks without a vectorStoreID resolve the typed platform default. The
// effectiveEngines argument remains only for payload compatibility and is not
// an environment fallback.
func CreateRetrieveEngineFromPayload(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	availability StoreConfigAvailability,
	tenantID uint64,
	effectiveEngines []types.RetrieverEngineParams,
	vectorStoreID *string,
) (*CompositeRetrieveEngine, error) {
	_ = effectiveEngines
	if vectorStoreID == nil || *vectorStoreID == "" {
		storeID, err := availability.DefaultStoreID(ctx)
		if err != nil {
			return nil, err
		}
		if storeID == "" {
			return nil, ErrVectorStoreNotFound
		}
		return resolveBoundEngine(ctx, registry, availability, tenantID, storeID)
	}

	return resolveBoundEngine(ctx, registry, availability, tenantID, *vectorStoreID)
}

// resolveBoundEngine is the shared ownership-verified lookup path used by
// both CreateRetrieveEngineForKB and CreateRetrieveEngineFromPayload. It
// returns sentinel errors so that handlers can classify them (for example,
// async workers convert Forbidden/NotFound into asynq.SkipRetry).
func resolveBoundEngine(
	ctx context.Context,
	registry interfaces.RetrieveEngineRegistry,
	availability StoreConfigAvailability,
	tenantID uint64,
	storeID string,
) (*CompositeRetrieveEngine, error) {
	usable, err := availability.StoreUsable(ctx, storeID)
	if err != nil {
		// This lookup queries the database with the caller's context, so it is
		// where a shutdown or a disconnect is usually noticed first. Reporting
		// that as a store verdict would let async workers discard work that
		// only needs running again.
		if isContextError(err) {
			return nil, err
		}
		// Infrastructure failure — record the raw error for operators but
		// do not leak internals to the caller. The store itself may be fine,
		// so this is retryable rather than not-found.
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
			"store_id":  storeID,
			"reason":    "ownership lookup failed",
		})
		return nil, ErrVectorStoreUnavailable
	}
	if !usable {
		logger.Warnf(ctx, "[retriever.factory] vector store unavailable: store=%s", storeID)
		return nil, ErrVectorStoreNotFound
	}

	svc, err := registry.GetOrLoadByStoreID(ctx, tenantID, storeID)
	if err != nil {
		if isContextError(err) {
			return nil, err
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
			"store_id":  storeID,
			"reason":    "store engine could not be resolved",
		})
		return nil, classifyLookupError(err)
	}

	// Build the composite directly from the resolved service.
	//
	// We cannot delegate to NewCompositeRetrieveEngine here because that
	// function resolves engines through registry.GetRetrieveEngineService,
	// which reads from the byEngineType map (env stores). DB stores live
	// in the byStoreID map and are not reachable via engine type alone —
	// multiple stores can share the same engine type.
	//
	// Semantics: a KB bound to a DB store uses every retriever type that
	// store supports. This intentionally overrides the tenant-level
	// effective-engines filter, because binding a KB to a specific store
	// is an explicit opt-out of tenant-default routing.
	return &CompositeRetrieveEngine{
		engineInfos: []*engineInfo{{
			retrieveEngine: svc,
			retrieverType:  slices.Clone(svc.Support()),
		}},
	}, nil
}
