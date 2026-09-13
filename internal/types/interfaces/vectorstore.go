package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// StoreRegistry provides VectorStore-based engine registration/lookup.
type StoreRegistry interface {
	// RegisterWithStoreID registers an engine service by VectorStore ID.
	// Upsert semantics: existing entry is overwritten silently.
	RegisterWithStoreID(storeID string, svc RetrieveEngineService)
	// GetByStoreID retrieves an engine service by VectorStore ID.
	GetByStoreID(storeID string) (RetrieveEngineService, error)
	// UnregisterByStoreID removes an engine service by VectorStore ID (idempotent).
	UnregisterByStoreID(storeID string)
}

// EngineFactory creates a RetrieveEngineService from a VectorStore's config.
// Defined as a function type to avoid circular imports between container and service packages.
type EngineFactory func(ctx context.Context, store types.VectorStore) (RetrieveEngineService, error)

// VectorStoreService manages platform-global vector connection configuration.
type VectorStoreService interface {
	// CreateStore validates and creates a new vector store.
	CreateStore(ctx context.Context, store *types.VectorStore) error
	// UpdateStore updates an existing vector store (name only).
	UpdateStore(ctx context.Context, store *types.VectorStore) error
	// DeleteStore deletes a vector store by id.
	// Rejects deletion when any active knowledge base is bound to the store
	// (binding guard); the caller must unbind or delete those KBs first.
	DeleteStore(ctx context.Context, id string) error
	SetDefaultStore(ctx context.Context, id string) error
	// TestConnection tests connectivity to a vector database.
	// Returns the detected server version on success (e.g., "7.10.1"), empty string if unknown.
	//
	// Validation-free: intended for stored configs already validated at create
	// time. Handlers receiving raw
	// user input MUST use TestRawConnection instead.
	TestConnection(ctx context.Context, engineType types.RetrieverEngineType, config types.ConnectionConfig) (string, error)
	// TestRawConnection validates raw user-supplied connection config
	// (engine-type allowlist, required fields, SSRF policy) and then delegates
	// to TestConnection. This is the entry point for unpersisted user input.
	TestRawConnection(ctx context.Context, engineType types.RetrieverEngineType, config types.ConnectionConfig) (string, error)
	// SaveDetectedVersion updates the connection_config.version for a stored vector store.
	SaveDetectedVersion(ctx context.Context, store *types.VectorStore, version string) error

	// ResolveStoreView returns the API-safe display projection of a single
	// global store ID.
	//
	// Never returns connection credentials in any form: the StoreDisplay
	// payload carries only Name / Source / EngineType / Status.
	ResolveStoreView(ctx context.Context, storeID string) (types.StoreDisplay, error)

	// BatchResolveStoreView resolves multiple global store IDs in one DB read.
	//
	// Intended for list endpoints that need store metadata for many KBs at
	// once without incurring N+1 ResolveStoreView calls.
	BatchResolveStoreView(ctx context.Context, storeIDs []string) (map[string]types.StoreDisplay, error)

	// DefaultStoreView returns the display-safe projection of the configured
	// global default, or UnavailableStoreDisplay when no default is set.
	DefaultStoreView(ctx context.Context) types.StoreDisplay
}

// VectorStoreRepository defines the repository interface for VectorStore CRUD.
type VectorStoreRepository interface {
	// Create creates a new vector store
	Create(ctx context.Context, store *types.VectorStore) error
	// GetByID retrieves a global vector store by ID.
	GetByID(ctx context.Context, id string) (*types.VectorStore, error)
	// GetDefault returns the platform fallback row, or nil when it is unset.
	GetDefault(ctx context.Context) (*types.VectorStore, error)
	// List lists all platform vector stores.
	List(ctx context.Context) ([]*types.VectorStore, error)
	// Update updates a vector store (only mutable fields: name)
	Update(ctx context.Context, store *types.VectorStore) error
	// UpdateConnectionConfig updates only the connection_config column
	UpdateConnectionConfig(ctx context.Context, store *types.VectorStore) error
	// Delete soft-deletes a vector store
	Delete(ctx context.Context, id string) error
}
