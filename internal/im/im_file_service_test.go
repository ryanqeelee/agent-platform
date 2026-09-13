package im

import (
	"context"
	"io"
	"mime/multipart"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
)

// stubIMFileService implements interfaces.FileService for IM resolver tests.
type stubIMFileService struct {
	getFileURL func(ctx context.Context, filePath string) (string, error)
}

func (s *stubIMFileService) CheckConnectivity(context.Context) error { return nil }

func (s *stubIMFileService) SaveFile(context.Context, *multipart.FileHeader, uint64, string) (string, error) {
	return "", nil
}

func (s *stubIMFileService) SaveBytes(context.Context, []byte, uint64, string, bool) (string, error) {
	return "", nil
}

func (s *stubIMFileService) GetFile(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *stubIMFileService) GetFileURL(ctx context.Context, filePath string) (string, error) {
	if s.getFileURL != nil {
		return s.getFileURL(ctx, filePath)
	}
	return "https://global-storage.example/" + filePath, nil
}

func (s *stubIMFileService) DeleteFile(context.Context, string) error { return nil }

func (s *stubIMFileService) CopyFile(context.Context, string, uint64, string) (string, error) {
	return "", nil
}

type stubStorageBackendResolver struct {
	fileService interfaces.FileService
	provider    string
	calls       int
	backendIDs  []string
}

func (r *stubStorageBackendResolver) ResolveFileService(
	_ context.Context, backendID, _ string,
) (interfaces.FileService, string, error) {
	r.calls++
	r.backendIDs = append(r.backendIDs, backendID)
	return r.fileService, r.provider, nil
}

func (r *stubStorageBackendResolver) ResolveBackend(
	_ context.Context, backendID string,
) (*types.StorageBackend, error) {
	return &types.StorageBackend{ID: backendID, Provider: r.provider}, nil
}

func TestIMFileServiceResolver_CachesPerBackend(t *testing.T) {
	stub := &stubIMFileService{}
	storage := &stubStorageBackendResolver{fileService: stub, provider: "minio"}
	r := newIMFileServiceResolver(&types.Tenant{}, nil, storage)

	svc1 := r.ResolveFileService("storage://backend-a/minio://wizard-test/10000/a.png")
	svc2 := r.ResolveFileService("storage://backend-a/minio://wizard-test/10000/b.png")
	assert.Same(t, svc1, svc2, "same provider should reuse cached FileService")
	assert.Equal(t, 1, storage.calls)
	assert.Equal(t, []string{"backend-a"}, storage.backendIDs)
}

func TestRewriteStorageURLs_MinIOViaGlobalBackend(t *testing.T) {
	stub := &stubIMFileService{
		getFileURL: func(_ context.Context, filePath string) (string, error) {
			return "https://minio.example/presigned?path=" + filePath, nil
		},
	}
	in := `![知识助理"知识库"管理视图界面](minio://wizard-test/10000/exports/c91cf852.png)`
	storage := &stubStorageBackendResolver{fileService: stub, provider: "minio"}
	resolver := newIMFileServiceResolver(&types.Tenant{}, nil, storage)
	out := rewriteStorageURLs(context.Background(), in, resolver)
	assert.Contains(t, out, "https://minio.example/presigned")
	assert.NotContains(t, out, "](minio://")
}

func TestRewriteStorageURLs_ScopedPath(t *testing.T) {
	stub := &stubIMFileService{
		getFileURL: func(_ context.Context, filePath string) (string, error) {
			assert.Equal(t, "storage://backend-a/cos://bucket/ap-test/10000/exports/a.png", filePath)
			return "https://storage.example/a.png", nil
		},
	}
	storage := &stubStorageBackendResolver{fileService: stub, provider: "cos"}
	input := "![img](storage://backend-a/cos://bucket/ap-test/10000/exports/a.png)"
	output := rewriteStorageURLs(context.Background(), input, newIMFileServiceResolver(&types.Tenant{}, nil, storage))
	assert.Contains(t, output, "https://storage.example/a.png")
	assert.Equal(t, []string{"backend-a"}, storage.backendIDs)
}

// When GetFileURL resolves a resource:// alias to a still-internal storage://
// path (no public HTTP URL), the rewrite must be a no-op rather than emit the
// unrenderable URL to the IM client.
func TestRewriteStorageURLs_NonHTTPResultIsNoOp(t *testing.T) {
	stub := &stubIMFileService{
		getFileURL: func(_ context.Context, _ string) (string, error) {
			return "storage://7cb970a6/oss://bcjy/10000/exports/a.png", nil
		},
	}
	in := "![img](resource://xifDo7NTSL300Lp1goVutw)"
	out := rewriteStorageURLs(context.Background(), in, newIMFileServiceResolver(&types.Tenant{}, stub))
	assert.Equal(t, in, out)
	assert.NotContains(t, out, "storage://")
}

// URL schemes are case-insensitive (RFC 3986); an uppercase-scheme result (e.g.
// from an OBS_PROXY_DOMAIN configured as HTTPS://...) is renderable and must be
// substituted, not dropped as "non-HTTP".
func TestRewriteStorageURLs_UppercaseSchemeIsSubstituted(t *testing.T) {
	stub := &stubIMFileService{
		getFileURL: func(_ context.Context, _ string) (string, error) {
			return "HTTPS://cdn.example.com/x.png", nil
		},
	}
	in := "![img](resource://xifDo7NTSL300Lp1goVutw)"
	out := rewriteStorageURLs(context.Background(), in, newIMFileServiceResolver(&types.Tenant{}, stub))
	assert.Contains(t, out, "HTTPS://cdn.example.com/x.png")
	assert.NotContains(t, out, "resource://")
}

func TestCleanIMContent_MinIOGlobalBackendIntegration(t *testing.T) {
	stub := &stubIMFileService{
		getFileURL: func(_ context.Context, _ string) (string, error) {
			return "https://minio.example/img.png", nil
		},
	}
	tenant := &types.Tenant{}
	in := "see ![x](minio://wizard-test/10000/exports/x.png) ok"
	storage := &stubStorageBackendResolver{fileService: stub, provider: "minio"}
	out := cleanIMContent(context.Background(), in, tenant, nil, storage)
	assert.Contains(t, out, "https://minio.example/img.png")
}
