package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type platformChatHistoryRouteModelService struct{ interfaces.ModelService }

func newPlatformChatHistoryRouteEngine(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE platform_chat_history_config (
		id INTEGER PRIMARY KEY, enabled INTEGER NOT NULL, embedding_model_id TEXT NOT NULL,
		updated_by TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE tenant_chat_history_indexes (
		tenant_id INTEGER PRIMARY KEY, knowledge_base_id TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE knowledges (
		id TEXT PRIMARY KEY, knowledge_base_id TEXT NOT NULL, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO platform_chat_history_config
		(id, enabled, embedding_model_id, updated_by, created_at, updated_at)
		VALUES (1, 0, '', 'migration', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`).Error)

	repo := repository.NewPlatformChatHistoryRepository(db)
	h := handler.NewPlatformChatHistoryHandler(service.NewPlatformChatHistoryService(
		repo, &platformChatHistoryRouteModelService{},
	))
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		admin := c.GetHeader("X-Test-System-Admin") == "true"
		ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, admin)
		ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "caller", IsActive: true, IsSystemAdmin: admin})
		ctx = context.WithValue(ctx, types.UserIDContextKey, "caller")
		if tenant := c.GetHeader("X-Test-Tenant"); tenant != "" {
			ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10001))
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	RegisterPlatformChatHistoryRoutes(router.Group("/api/v1"), h, &rbacGuards{})
	return router
}

func TestPlatformChatHistoryRoutesAreTenantlessAndSystemAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newPlatformChatHistoryRouteEngine(t)
	for _, path := range []string{
		"/api/v1/system/admin/chat-history-config",
		"/api/v1/system/admin/chat-history-stats",
	} {
		denied := httptest.NewRecorder()
		router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusForbidden, denied.Code)

		allowed := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Test-System-Admin", "true")
		router.ServeHTTP(allowed, request)
		require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())

		withTenant := httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Test-System-Admin", "true")
		request.Header.Set("X-Test-Tenant", "ignored")
		router.ServeHTTP(withTenant, request)
		require.Equal(t, allowed.Body.String(), withTenant.Body.String())
	}

	legacy := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/tenants/10001/chat-history-config", nil)
	request.Header.Set("X-Test-System-Admin", "true")
	router.ServeHTTP(legacy, request)
	require.Equal(t, http.StatusNotFound, legacy.Code)
}

func TestPlatformChatHistoryConfigPutIsSystemAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newPlatformChatHistoryRouteEngine(t)
	const path = "/api/v1/system/admin/chat-history-config"
	const body = `{"enabled":false,"embedding_model_id":""}`

	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
	require.Equal(t, http.StatusForbidden, denied.Code)

	allowed := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-System-Admin", "true")
	router.ServeHTTP(allowed, request)
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
}
