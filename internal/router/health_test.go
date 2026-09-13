package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func healthRouteStatus(router *gin.Engine, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func TestHealthRoutesSeparateLivenessFromDatabaseReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	router := gin.New()
	registerHealthRoutes(router, db)

	live := healthRouteStatus(router, "/health")
	ready := healthRouteStatus(router, "/ready")
	require.Equal(t, http.StatusOK, live.Code, live.Body.String())
	require.JSONEq(t, `{"status":"ok"}`, live.Body.String())
	require.Equal(t, http.StatusOK, ready.Code, ready.Body.String())
	require.JSONEq(t, `{"status":"ready"}`, ready.Body.String())

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	live = healthRouteStatus(router, "/health")
	ready = healthRouteStatus(router, "/ready")
	require.Equal(t, http.StatusOK, live.Code, live.Body.String())
	require.JSONEq(t, `{"status":"ok"}`, live.Body.String())
	require.Equal(t, http.StatusServiceUnavailable, ready.Code, ready.Body.String())
	require.JSONEq(t, `{"status":"unavailable"}`, ready.Body.String())
}
