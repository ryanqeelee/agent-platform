package retriever

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type vectorStoreConfigAvailability struct {
	repo interfaces.VectorStoreRepository
}

// NewVectorStoreRepoOwnership returns the production
// TenantStoreOwnership implementation backed by VectorStoreRepository.
func NewVectorStoreConfigAvailability(
	repo interfaces.VectorStoreRepository,
) StoreConfigAvailability {
	return &vectorStoreConfigAvailability{repo: repo}
}

// StoreOwnedBy returns true iff a vector store with the given ID exists
// under the given tenant. Errors are reserved for infrastructure failures;
// a non-existent (but well-formed) store ID returns (false, nil).
func (o *vectorStoreConfigAvailability) StoreUsable(ctx context.Context, storeID string) (bool, error) {
	store, err := o.repo.GetByID(ctx, storeID)
	if err != nil {
		return false, err
	}
	return store != nil, nil
}

func (o *vectorStoreConfigAvailability) DefaultStoreID(ctx context.Context) (string, error) {
	store, err := o.repo.GetDefault(ctx)
	if err != nil || store == nil {
		return "", err
	}
	return store.ID, nil
}
