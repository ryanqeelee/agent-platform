package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WebSearchProviderRepository defines the repository interface for web search provider CRUD
type WebSearchProviderRepository interface {
	// Create creates a new web search provider
	Create(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// GetByID retrieves a platform provider by ID.
	GetByID(ctx context.Context, id string) (*types.WebSearchProviderEntity, error)
	// GetDefault reads the single platform-default row.
	GetDefault(ctx context.Context) (*types.WebSearchProviderEntity, error)
	// SetDefault atomically selects the exact platform default row.
	SetDefault(ctx context.Context, id string) error
	// List lists all platform web search providers.
	List(ctx context.Context) ([]*types.WebSearchProviderEntity, error)
	// Update updates a web search provider
	Update(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// Delete deletes a web search provider (soft delete)
	Delete(ctx context.Context, id string) error
}

// WebSearchProviderService defines the service interface for web search provider management.
// Definitions are platform-owned; tenant identity only enters runtime requests.
type WebSearchProviderService interface {
	// CreateProvider creates a new web search provider.
	CreateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error
	// UpdateProvider updates an existing provider.
	UpdateProvider(ctx context.Context, provider *types.WebSearchProviderEntity) error
	DeleteProvider(ctx context.Context, id string) error

	// UpdateProviderCredentials writes one or more credential fields.
	// apiKey nil means "do not touch"; empty string is a no-op (clearing
	// goes through ClearProviderCredential). Returns the updated entity.
	UpdateProviderCredentials(
		ctx context.Context, id string, apiKey *string,
	) (*types.WebSearchProviderEntity, error)
	// ClearProviderCredential removes a single credential field. Currently
	// only "api_key" is recognized. Idempotent on already-empty fields.
	ClearProviderCredential(ctx context.Context, id, field string) error
}
