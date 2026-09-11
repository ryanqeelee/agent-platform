package router

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var _ interfaces.FileService = (*stubFileService)(nil)

type stubFileService struct {
	getFile func(ctx context.Context, filePath string) (io.ReadCloser, error)
}

type stubResourceCatalog struct {
	resource   *types.StoredResource
	bindings   []*types.ResourceBinding
	bindingErr error
}

type stubKnowledgeByID struct{ knowledge *types.Knowledge }

func (s *stubKnowledgeByID) GetKnowledgeByIDOnly(context.Context, string) (*types.Knowledge, error) {
	return s.knowledge, nil
}

type stubKnowledgeBaseByID struct{ kb *types.KnowledgeBase }

func (s *stubKnowledgeBaseByID) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type stubMessageFileLookup struct {
	get      func(ctx context.Context, sessionID, messageID string) (*types.Message, error)
	governed func(ctx context.Context, messageID string) (bool, error)
}

func (s *stubMessageFileLookup) IsGovernedAnalysisMessage(
	ctx context.Context, messageID string,
) (bool, error) {
	if s.governed == nil {
		return false, nil
	}
	return s.governed(ctx, messageID)
}

func (s *stubMessageFileLookup) GetMessageForRead(
	ctx context.Context,
	sessionID, messageID string,
) (*types.Message, error) {
	return s.get(ctx, sessionID, messageID)
}

type stubSharedAgentFileLookup struct {
	get func(
		ctx context.Context,
		tenantID uint64,
		callerTenantRole types.TenantRole,
		agentID string,
		sourceTenantID ...uint64,
	) (*types.CustomAgent, error)
}

func (s *stubSharedAgentFileLookup) GetSharedAgentForTenant(
	ctx context.Context,
	tenantID uint64,
	callerTenantRole types.TenantRole,
	agentID string,
	sourceTenantID ...uint64,
) (*types.CustomAgent, error) {
	return s.get(ctx, tenantID, callerTenantRole, agentID, sourceTenantID...)
}

func (s *stubResourceCatalog) Register(
	context.Context,
	uint64,
	string,
	interfaces.ResourceRegistration,
) (string, error) {
	panic("unexpected Register")
}

func (s *stubResourceCatalog) Resolve(context.Context, string) (*types.StoredResource, error) {
	return s.resource, nil
}

func (s *stubResourceCatalog) ResolvePath(_ context.Context, value string) (string, *types.StoredResource, error) {
	if _, ok := types.ParseResourcePath(value); ok && s.resource != nil {
		return s.resource.PhysicalPath, s.resource, nil
	}
	return value, nil, nil
}

func (s *stubResourceCatalog) ResolveTenantPath(_ context.Context, _ uint64, value string) (string, *types.StoredResource, error) {
	return s.ResolvePath(context.Background(), value)
}

func (s *stubResourceCatalog) ListKnowledgeBindings(context.Context, string) ([]*types.ResourceBinding, error) {
	result := make([]*types.ResourceBinding, 0, len(s.bindings))
	for _, binding := range s.bindings {
		if binding != nil && binding.OwnerType == types.ResourceOwnerKnowledge {
			result = append(result, binding)
		}
	}
	return result, s.bindingErr
}

func (s *stubResourceCatalog) ListMessageBindings(context.Context, string) ([]*types.ResourceBinding, error) {
	result := make([]*types.ResourceBinding, 0, len(s.bindings))
	for _, binding := range s.bindings {
		if binding != nil && binding.OwnerType == types.ResourceOwnerMessage {
			result = append(result, binding)
		}
	}
	return result, s.bindingErr
}

func (s *stubResourceCatalog) Bind(context.Context, string, string, string, string) error {
	panic("unexpected Bind")
}

func (s *stubResourceCatalog) Release(context.Context, string, string, string) (int64, error) {
	panic("unexpected Release")
}

func (s *stubResourceCatalog) MarkDeleted(context.Context, string) error {
	panic("unexpected MarkDeleted")
}

func (s *stubResourceCatalog) CreateAccessGrant(context.Context, string, time.Duration) (string, error) {
	panic("unexpected CreateAccessGrant")
}

func (s *stubResourceCatalog) ResolveAccessGrant(context.Context, string) (*types.StoredResource, error) {
	return s.resource, nil
}

func (s *stubFileService) CheckConnectivity(ctx context.Context) error {
	return nil
}

func (s *stubFileService) SaveFile(ctx context.Context, file *multipart.FileHeader, tenantID uint64, knowledgeID string) (string, error) {
	panic("unexpected call to SaveFile")
}

func (s *stubFileService) SaveBytes(ctx context.Context, data []byte, tenantID uint64, fileName string, temp bool) (string, error) {
	panic("unexpected call to SaveBytes")
}

func (s *stubFileService) GetFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	if s.getFile == nil {
		panic("unexpected call to GetFile")
	}
	return s.getFile(ctx, filePath)
}

func (s *stubFileService) GetFileURL(ctx context.Context, filePath string) (string, error) {
	panic("unexpected call to GetFileURL")
}

func (s *stubFileService) DeleteFile(ctx context.Context, filePath string) error {
	panic("unexpected call to DeleteFile")
}

func (s *stubFileService) CopyFile(ctx context.Context, srcPath string, tenantID uint64, knowledgeID string) (string, error) {
	panic("unexpected call to CopyFile")
}

func TestServeFilesFallsBackToGlobalFileService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	var requestedPath string
	filePath := "local://42/docs/example.txt"
	resourceRef := types.BuildResourcePath("AbCdEfGhIjKlMnOpQrStUv")
	serveFilesWithResources(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			requestedPath = filePath
			return io.NopCloser(strings.NewReader("fallback-body")), nil
		},
	}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 42, PhysicalPath: filePath}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(resourceRef), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if requestedPath != filePath {
		t.Fatalf("requested path = %q, want %q", requestedPath, filePath)
	}
	if body := recorder.Body.String(); body != "fallback-body" {
		t.Fatalf("body = %q, want %q", body, "fallback-body")
	}
}

func TestServeFilesResolvesShortResourceReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")
	const ref = "resource://AbCdEfGhIjKlMnOpQrStUv"
	const physical = "local://42/exports/a.png"

	engine := gin.New()
	var requestedPath string
	serveFilesWithResources(engine, &stubFileService{getFile: func(_ context.Context, path string) (io.ReadCloser, error) {
		requestedPath = path
		return io.NopCloser(strings.NewReader("image")), nil
	}}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 42, PhysicalPath: physical}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(ref), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if requestedPath != physical {
		t.Fatalf("requested path = %q, want %q", requestedPath, physical)
	}
}

func TestGenericFileRoutesRejectGovernedMessageArtifactAndPreserveOrdinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	for _, route := range []string{"/files", "/api/v1/embed/channel-1/files"} {
		for _, tc := range []struct {
			name      string
			governed  bool
			wantCode  int
			wantBytes string
		}{
			{name: "governed", governed: true, wantCode: http.StatusForbidden},
			{name: "ordinary", governed: false, wantCode: http.StatusOK, wantBytes: "ordinary-artifact"},
		} {
			t.Run(route+" "+tc.name, func(t *testing.T) {
				const handle = "AbCdEfGhIjKlMnOpQrStUv"
				engine := gin.New()
				engine.GET(route, func(c *gin.Context) {
					ctx := context.WithValue(c.Request.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42})
					c.Request = c.Request.WithContext(ctx)
					c.Next()
				}, newFileServeHandler(
					&stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
						if tc.governed {
							t.Fatal("governed artifact must not reach storage")
						}
						return io.NopCloser(strings.NewReader(tc.wantBytes)), nil
					}},
					nil,
					&stubResourceCatalog{
						resource: &types.StoredResource{Handle: handle, TenantID: 42, PhysicalPath: "local://42/exports/result.csv"},
						bindings: []*types.ResourceBinding{{
							OwnerType: types.ResourceOwnerMessage, OwnerID: "artifact-message", Relation: types.ResourceRelationArtifact,
						}},
					},
					nil,
					nil,
					&stubMessageFileLookup{governed: func(context.Context, string) (bool, error) {
						return tc.governed, nil
					}},
				))

				req := httptest.NewRequest(http.MethodGet, route+"?file_path="+url.QueryEscape(types.BuildResourcePath(handle)), nil)
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				require.Equal(t, tc.wantCode, w.Code)
				require.Equal(t, tc.wantBytes, w.Body.String())
			})
		}
	}
}

func TestServeFilesRejectsUnregisteredPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")
	const filePath = "local://42/docs/legacy.txt"
	engine := gin.New()
	serveFilesWithResources(engine, &stubFileService{getFile: func(_ context.Context, path string) (io.ReadCloser, error) {
		t.Fatalf("unregistered path reached file service: %q", path)
		return nil, nil
	}}, nil, &stubResourceCatalog{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(filePath), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got, want := w.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d body=%s", got, want, w.Body.String())
	}
}

func TestServeFilesServesKnowledgeBoundResourceAfterLiveAccessCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	serveFilesWithResources(engine, &stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("image")), nil
	}}, nil, &stubResourceCatalog{
		resource: &types.StoredResource{Handle: "AbCdEfGhIjKlMnOpQrStUv", TenantID: 42, PhysicalPath: "local://42/exports/a.png"},
		bindings: []*types.ResourceBinding{{OwnerType: "knowledge", OwnerID: "knowledge-1"}},
	}, &stubKnowledgeByID{knowledge: &types.Knowledge{
		ID: "knowledge-1", TenantID: 42, KnowledgeBaseID: "kb-1",
	}}, &stubKnowledgeBaseByID{kb: &types.KnowledgeBase{ID: "kb-1", TenantID: 42}})
	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(types.BuildResourcePath("AbCdEfGhIjKlMnOpQrStUv")), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if got, want := w.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d body=%s", got, want, w.Body.String())
	}
}

func TestServeFilesRejectsCrossTenantResourceReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ref = "resource://AbCdEfGhIjKlMnOpQrStUv"
	engine := gin.New()
	serveFilesWithResources(engine, &stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
		t.Fatal("GetFile should not be called")
		return nil, nil
	}}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 7, PhysicalPath: "local://7/exports/a.png"}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(ref), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestServeFilesFailsClosedWhenKnowledgeBindingLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ref = "resource://AbCdEfGhIjKlMnOpQrStUv"
	engine := gin.New()
	serveFilesWithResources(engine, &stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
		t.Fatal("GetFile should not be called when binding lookup fails")
		return nil, nil
	}}, nil, &stubResourceCatalog{
		resource:   &types.StoredResource{TenantID: 42, Handle: "AbCdEfGhIjKlMnOpQrStUv", PhysicalPath: "local://42/exports/a.png"},
		bindingErr: context.DeadlineExceeded,
	}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(ref), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestResourceGrantServesShortPublicURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	physical := "local://42/exports/a.png"
	engine := gin.New()
	serveResourceGrants(
		engine,
		&stubResourceCatalog{resource: &types.StoredResource{
			ID:           "resource-1",
			TenantID:     42,
			PhysicalPath: physical,
			OriginalName: "a.png",
			MimeType:     "image/png",
		}},
		&stubTenantService{get: func(_ context.Context, id uint64) (*types.Tenant, error) {
			return &types.Tenant{ID: id}, nil
		}},
		&stubFileService{getFile: func(_ context.Context, path string) (io.ReadCloser, error) {
			if path != physical {
				t.Fatalf("path = %q, want %q", path, physical)
			}
			return io.NopCloser(strings.NewReader("image")), nil
		}},
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/r/GrantTokenAbCdEfGhIjKlM", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "image" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

func TestResourceGrantRejectsKnowledgeBoundResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	serveResourceGrants(engine, &stubResourceCatalog{
		resource: &types.StoredResource{ID: "resource-1", Handle: "AbCdEfGhIjKlMnOpQrStUv", TenantID: 42, PhysicalPath: "local://42/exports/a.png"},
		bindings: []*types.ResourceBinding{{OwnerType: "knowledge", OwnerID: "old-knowledge"}},
	}, &stubTenantService{}, &stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
		t.Fatal("knowledge-bound grant must not reach storage")
		return nil, nil
	}}, nil, nil, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/r/GrantTokenAbCdEfGhIjKlM", nil))
	if got, want := w.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestResourceGrantRejectsGovernedMessageArtifactAndPreservesOrdinary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name      string
		governed  bool
		wantCode  int
		wantBytes string
	}{
		{name: "governed", governed: true, wantCode: http.StatusNotFound},
		{name: "ordinary", governed: false, wantCode: http.StatusOK, wantBytes: "ordinary-artifact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const handle = "AbCdEfGhIjKlMnOpQrStUv"
			engine := gin.New()
			serveResourceGrants(
				engine,
				&stubResourceCatalog{
					resource: &types.StoredResource{
						ID: "resource-1", Handle: handle, TenantID: 42,
						PhysicalPath: "local://42/exports/result.csv", OriginalName: "result.csv",
					},
					bindings: []*types.ResourceBinding{{
						OwnerType: types.ResourceOwnerMessage, OwnerID: "artifact-message", Relation: types.ResourceRelationArtifact,
					}},
				},
				&stubTenantService{get: func(context.Context, uint64) (*types.Tenant, error) {
					return &types.Tenant{ID: 42}, nil
				}},
				&stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
					if tc.governed {
						t.Fatal("governed grant must not reach storage")
					}
					return io.NopCloser(strings.NewReader(tc.wantBytes)), nil
				}},
				nil,
				nil,
				nil,
				&stubMessageFileLookup{governed: func(context.Context, string) (bool, error) {
					return tc.governed, nil
				}},
			)

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/r/GrantTokenAbCdEfGhIjKlM", nil))
			require.Equal(t, tc.wantCode, w.Code)
			require.Equal(t, tc.wantBytes, w.Body.String())
		})
	}
}

func TestServeFilesDoesNotFallbackWhenProviderDoesNotMatchGlobalStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "minio")

	engine := gin.New()
	filePath := "local://42/docs/example.txt"
	resourceRef := types.BuildResourcePath("AbCdEfGhIjKlMnOpQrStUv")
	serveFilesWithResources(engine, &stubFileService{
		getFile: func(ctx context.Context, filePath string) (io.ReadCloser, error) {
			t.Fatalf("GetFile should not be called for mismatched provider, got %q", filePath)
			return nil, nil
		},
	}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 42, PhysicalPath: filePath}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(resourceRef), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

// /files carries its own API-key guard (middleware.AllowFileServeAPIKey):
// full-access and tenant-wide retrieve keys may serve tenant-bounded paths,
// but KB-restricted keys (and keys lacking retrieve) are denied because a raw
// storage path cannot be bounded to a KB allow-list.
func TestServeFilesAPIKeyScopeMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	const filePath = "local://42/docs/example.txt"
	resourceRef := types.BuildResourcePath("AbCdEfGhIjKlMnOpQrStUv")

	cases := []struct {
		name     string
		scope    types.TenantAPIKeyScope
		wantCode int
	}{
		{
			name:     "full access allowed",
			scope:    types.TenantAPIKeyScope{FullAccess: true},
			wantCode: http.StatusOK,
		},
		{
			name: "tenant-wide retrieve allowed",
			scope: types.TenantAPIKeyScope{
				Capabilities: types.StringArray{string(types.APIKeyCapabilityRetrieve)},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "kb-restricted retrieve denied",
			scope: types.TenantAPIKeyScope{
				KnowledgeBaseIDs: types.StringArray{"kb-1"},
				Capabilities:     types.StringArray{string(types.APIKeyCapabilityRetrieve)},
			},
			wantCode: http.StatusForbidden,
		},
		{
			name: "non-retrieve capability denied",
			scope: types.TenantAPIKeyScope{
				Capabilities: types.StringArray{string(types.APIKeyCapabilityChat)},
			},
			wantCode: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			serveFilesWithResources(engine, &stubFileService{
				getFile: func(_ context.Context, _ string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("body")), nil
				},
			}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 42, PhysicalPath: filePath}}, nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(resourceRef), nil)
			ctx := context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42})
			ctx = types.WithTenantAPIKeyScope(ctx, tc.scope)
			req = req.WithContext(ctx)

			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)

			if got := recorder.Code; got != tc.wantCode {
				t.Fatalf("status = %d, want %d body=%s", got, tc.wantCode, recorder.Body.String())
			}
		})
	}
}

func TestKBScopedFilesRejectsUnregisteredPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")
	const ownerTenantID = uint64(10008)
	const filePath = "local://10008/exports/legacy.jpg"
	engine := gin.New()
	engine.GET("/knowledge-bases/:id/files", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, ownerTenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}, newKBScopedFileServeHandlerWithResources(
		&stubTenantService{get: func(_ context.Context, id uint64) (*types.Tenant, error) { return &types.Tenant{ID: id}, nil }},
		&stubFileService{getFile: func(_ context.Context, path string) (io.ReadCloser, error) {
			t.Fatalf("unregistered path reached file service: %q", path)
			return nil, nil
		}},
		nil, &stubResourceCatalog{}, nil,
	))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/knowledge-bases/kb-1/files?file_path="+url.QueryEscape(filePath), nil))
	if got, want := w.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d body=%s", got, want, w.Body.String())
	}
}

func TestKBScopedFilesRequiresFilePath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/knowledge-bases/:id/files", newKBScopedFileServeHandlerWithResources(
		&stubTenantService{}, &stubFileService{}, nil, &stubResourceCatalog{}, nil,
	))

	req := httptest.NewRequest(http.MethodGet, "/knowledge-bases/kb-1/files", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

// Registered resources are only reachable through the KB that owns the
// bound knowledge. A stale resource handle therefore cannot be replayed via a
// different KB, nor can an unbound catalog entry acquire access merely by
// being addressed through this proxy.
func TestKBScopedFilesRejectsUnboundOrCrossKnowledgeBaseResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	const ownerTenantID = uint64(10008)
	const handle = "abcdefghijklmnopqrstuv"
	resource := &types.StoredResource{
		Handle:       handle,
		TenantID:     ownerTenantID,
		PhysicalPath: "local://10008/exports/img.jpg",
	}

	for _, tc := range []struct {
		name      string
		bindings  []*types.ResourceBinding
		knowledge *types.Knowledge
	}{
		{
			name: "unbound resource",
		},
		{
			name:     "bound to another knowledge base",
			bindings: []*types.ResourceBinding{{OwnerType: "knowledge", OwnerID: "knowledge-1"}},
			knowledge: &types.Knowledge{
				ID:              "knowledge-1",
				TenantID:        ownerTenantID,
				KnowledgeBaseID: "kb-other",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := &stubResourceCatalog{resource: resource, bindings: tc.bindings}
			engine := gin.New()
			engine.GET("/knowledge-bases/:id/files", func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, ownerTenantID)
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			}, newKBScopedFileServeHandlerWithResources(
				&stubTenantService{},
				&stubFileService{getFile: func(_ context.Context, path string) (io.ReadCloser, error) {
					t.Fatalf("GetFile must not be called for %s: %q", tc.name, path)
					return nil, nil
				}},
				nil,
				catalog,
				&downloadKnowledgeLookup{knowledge: tc.knowledge},
			))

			req := httptest.NewRequest(http.MethodGet,
				"/knowledge-bases/kb-expected/files?file_path="+url.QueryEscape(types.BuildResourcePath(handle)), nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if got, want := w.Code, http.StatusForbidden; got != want {
				t.Fatalf("status = %d, want %d body=%s", got, want, w.Body.String())
			}
		})
	}
}

func newMessageScopedFilesTestEngine(
	callerTenantID uint64,
	messageService messageFileLookup,
	agentShareService sharedAgentFileLookup,
	tenantService interfaces.TenantService,
	global interfaces.FileService,
	resourceCatalog interfaces.ResourceCatalog,
) *gin.Engine {
	engine := gin.New()
	engine.GET("/sessions/:id/messages/:message_id/files",
		func(c *gin.Context) {
			ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, callerTenantID)
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		},
		newMessageScopedFileServeHandler(
			messageService,
			agentShareService,
			tenantService,
			global,
			nil,
			resourceCatalog,
		),
	)
	return engine
}

func TestMessageScopedFilesServesSharedAgentResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	const (
		callerTenantID = uint64(42)
		ownerTenantID  = uint64(7)
		ref            = "resource://AbCdEfGhIjKlMnOpQrStUv"
		physical       = "local://7/exports/chart.png"
	)
	var requestedPath string
	engine := newMessageScopedFilesTestEngine(
		callerTenantID,
		&stubMessageFileLookup{get: func(_ context.Context, sessionID, messageID string) (*types.Message, error) {
			if sessionID != "session-1" || messageID != "message-1" {
				t.Fatalf("unexpected message scope %s/%s", sessionID, messageID)
			}
			return &types.Message{AgentID: "agent-1", AgentTenantID: ownerTenantID}, nil
		}},
		&stubSharedAgentFileLookup{get: func(
			_ context.Context,
			tenantID uint64,
			_ types.TenantRole,
			agentID string,
			sourceTenantID ...uint64,
		) (*types.CustomAgent, error) {
			if tenantID != callerTenantID || agentID != "agent-1" || len(sourceTenantID) != 1 || sourceTenantID[0] != ownerTenantID {
				t.Fatalf("unexpected shared-agent lookup tenant=%d agent=%s source=%v", tenantID, agentID, sourceTenantID)
			}
			return &types.CustomAgent{ID: agentID, TenantID: ownerTenantID}, nil
		}},
		&stubTenantService{get: func(_ context.Context, id uint64) (*types.Tenant, error) {
			return &types.Tenant{ID: id}, nil
		}},
		&stubFileService{getFile: func(_ context.Context, filePath string) (io.ReadCloser, error) {
			requestedPath = filePath
			return io.NopCloser(strings.NewReader("shared-agent-image")), nil
		}},
		&stubResourceCatalog{resource: &types.StoredResource{
			TenantID:     ownerTenantID,
			PhysicalPath: physical,
			MimeType:     "image/png",
		}},
	)

	req := httptest.NewRequest(http.MethodGet,
		"/sessions/session-1/messages/message-1/files?file_path="+url.QueryEscape(ref), nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "shared-agent-image" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if requestedPath != physical {
		t.Fatalf("requested path = %q, want %q", requestedPath, physical)
	}
}

func TestMessageScopedFilesServesSameTenantResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	const (
		tenantID = uint64(42)
		ref      = "resource://AbCdEfGhIjKlMnOpQrStUv"
		physical = "local://42/exports/chart.png"
	)
	var requestedPath string
	engine := newMessageScopedFilesTestEngine(
		tenantID,
		&stubMessageFileLookup{get: func(_ context.Context, sessionID, messageID string) (*types.Message, error) {
			if sessionID != "session-1" || messageID != "message-1" {
				t.Fatalf("unexpected message scope %s/%s", sessionID, messageID)
			}
			return &types.Message{AgentTenantID: tenantID}, nil
		}},
		&stubSharedAgentFileLookup{get: func(
			context.Context, uint64, types.TenantRole, string, ...uint64,
		) (*types.CustomAgent, error) {
			t.Fatal("shared-agent lookup should not run for same-tenant resources")
			return nil, nil
		}},
		&stubTenantService{get: func(_ context.Context, id uint64) (*types.Tenant, error) {
			return &types.Tenant{ID: id}, nil
		}},
		&stubFileService{getFile: func(_ context.Context, filePath string) (io.ReadCloser, error) {
			requestedPath = filePath
			return io.NopCloser(strings.NewReader("same-tenant-image")), nil
		}},
		&stubResourceCatalog{resource: &types.StoredResource{
			TenantID:     tenantID,
			PhysicalPath: physical,
			MimeType:     "image/png",
		}},
	)

	req := httptest.NewRequest(http.MethodGet,
		"/sessions/session-1/messages/message-1/files?file_path="+url.QueryEscape(ref), nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "same-tenant-image" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if requestedPath != physical {
		t.Fatalf("requested path = %q, want %q", requestedPath, physical)
	}
}

func TestMessageScopedFilesRequiresFilePath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newMessageScopedFilesTestEngine(
		42,
		&stubMessageFileLookup{get: func(context.Context, string, string) (*types.Message, error) {
			return &types.Message{AgentTenantID: 42}, nil
		}},
		&stubSharedAgentFileLookup{},
		&stubTenantService{},
		&stubFileService{},
		&stubResourceCatalog{},
	)

	req := httptest.NewRequest(http.MethodGet, "/sessions/session-1/messages/message-1/files", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestMessageScopedFilesRejectsRevokedSharedAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ref = "resource://AbCdEfGhIjKlMnOpQrStUv"

	engine := newMessageScopedFilesTestEngine(
		42,
		&stubMessageFileLookup{get: func(context.Context, string, string) (*types.Message, error) {
			return &types.Message{AgentID: "agent-1", AgentTenantID: 7}, nil
		}},
		&stubSharedAgentFileLookup{get: func(
			context.Context, uint64, types.TenantRole, string, ...uint64,
		) (*types.CustomAgent, error) {
			return nil, nil
		}},
		&stubTenantService{get: func(context.Context, uint64) (*types.Tenant, error) {
			t.Fatal("tenant lookup should not run after share revocation")
			return nil, nil
		}},
		&stubFileService{getFile: func(context.Context, string) (io.ReadCloser, error) {
			t.Fatal("GetFile should not run after share revocation")
			return nil, nil
		}},
		&stubResourceCatalog{resource: &types.StoredResource{TenantID: 7, PhysicalPath: "local://7/exports/chart.png"}},
	)

	req := httptest.NewRequest(http.MethodGet,
		"/sessions/session-1/messages/message-1/files?file_path="+url.QueryEscape(ref), nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestMessageScopedFilesRejectsResourceOutsideMessageTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const ref = "resource://AbCdEfGhIjKlMnOpQrStUv"

	engine := newMessageScopedFilesTestEngine(
		42,
		&stubMessageFileLookup{get: func(context.Context, string, string) (*types.Message, error) {
			return &types.Message{AgentID: "agent-1", AgentTenantID: 8}, nil
		}},
		&stubSharedAgentFileLookup{get: func(
			context.Context, uint64, types.TenantRole, string, ...uint64,
		) (*types.CustomAgent, error) {
			t.Fatal("shared-agent lookup should not run for a mismatched resource tenant")
			return nil, nil
		}},
		&stubTenantService{},
		&stubFileService{},
		&stubResourceCatalog{resource: &types.StoredResource{TenantID: 7, PhysicalPath: "local://7/exports/chart.png"}},
	)

	req := httptest.NewRequest(http.MethodGet,
		"/sessions/session-1/messages/message-1/files?file_path="+url.QueryEscape(ref), nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestServeFilesForcesActiveContentDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("STORAGE_TYPE", "local")

	engine := gin.New()
	filePath := "local://42/docs/payload.svg"
	resourceRef := types.BuildResourcePath("AbCdEfGhIjKlMnOpQrStUv")
	serveFilesWithResources(engine, &stubFileService{
		getFile: func(_ context.Context, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(`<svg onload="alert(1)"></svg>`)), nil
		},
	}, nil, &stubResourceCatalog{resource: &types.StoredResource{TenantID: 42, PhysicalPath: filePath}}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/files?file_path="+url.QueryEscape(resourceRef), nil)
	req = req.WithContext(context.WithValue(req.Context(), types.TenantInfoContextKey, &types.Tenant{ID: 42}))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
