package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageBackendPathRoundTrip(t *testing.T) {
	path := BuildStorageBackendPath("backend-a", "cos://bucket/region/7/file.pdf")
	id, inner, ok := ParseStorageBackendPath(path)
	require.True(t, ok)
	assert.Equal(t, "backend-a", id)
	assert.Equal(t, "cos://bucket/region/7/file.pdf", inner)
	assert.Equal(t, "cos", ParseProviderScheme(path))
}

func TestSharesStorageBackendWithUsesConcreteInstance(t *testing.T) {
	aID, bID := "cos-a", "cos-b"
	a := &KnowledgeBase{StorageBackendID: &aID, StorageProviderConfig: &StorageProviderConfig{Provider: "cos"}}
	b := &KnowledgeBase{StorageBackendID: &bID, StorageProviderConfig: &StorageProviderConfig{Provider: "cos"}}
	assert.False(t, a.SharesStorageBackendWith(b, "", "cos"))

	b.StorageBackendID = &aID
	assert.True(t, a.SharesStorageBackendWith(b, "", "cos"))
}

func TestNewStorageBackendResponseMasksCredentials(t *testing.T) {
	backend := &StorageBackend{Config: StorageBackendConfig{AccessKeyID: "id", SecretAccessKey: "secret"}}
	response := NewStorageBackendResponse(backend)
	assert.Equal(t, RedactedSecretPlaceholder, response.Config.AccessKeyID)
	assert.Equal(t, RedactedSecretPlaceholder, response.Config.SecretAccessKey)
	assert.Equal(t, "id", backend.Config.AccessKeyID)
}

func TestStorageBackendRejectsTraversingPathPrefix(t *testing.T) {
	backend := &StorageBackend{
		Name: "unsafe", Provider: "local",
		Config: StorageBackendConfig{PathPrefix: "../outside"},
	}
	require.Error(t, backend.Validate())
}
