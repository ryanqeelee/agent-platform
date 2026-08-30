package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type failingVectorStoreDiagnosticService struct {
	interfaces.VectorStoreService
}

func (failingVectorStoreDiagnosticService) TestRawConnection(
	context.Context,
	types.RetrieverEngineType,
	types.ConnectionConfig,
) (string, error) {
	return "", errors.New("dial tcp secret.internal:9200: connection refused; password=secret-value")
}

func TestVectorStoreRawDiagnosticDoesNotExposeConnectionDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(types.TenantIDContextKey.String(), uint64(7))
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/vector-stores/test", strings.NewReader(
		`{"engine_type":"elasticsearch","connection_config":{"endpoint":"secret.internal:9200"}}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	NewVectorStoreHandler(nil, failingVectorStoreDiagnosticService{}, nil).TestStoreRaw(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "连接被拒绝")
	require.NotContains(t, recorder.Body.String(), "secret.internal")
	require.NotContains(t, recorder.Body.String(), "secret-value")
}
