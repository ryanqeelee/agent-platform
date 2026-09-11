package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tenantlessModelHandlerService struct {
	interfaces.ModelService
	created       *types.Model
	models        []*types.Model
	createErr     error
	credentialErr error
}

func (s *tenantlessModelHandlerService) CreateModel(_ context.Context, model *types.Model) error {
	if s.createErr != nil {
		return s.createErr
	}
	model.ID = "global-chat"
	s.created = model
	return nil
}

func (s *tenantlessModelHandlerService) ListModels(context.Context) ([]*types.Model, error) {
	return s.models, nil
}

func (s *tenantlessModelHandlerService) UpdateModelCredentials(
	_ context.Context, id string, apiKey, _ *string,
) (*types.Model, error) {
	if s.credentialErr != nil {
		return nil, s.credentialErr
	}
	model := &types.Model{ID: id, IsBuiltin: true}
	if apiKey != nil {
		model.Parameters.APIKey = *apiKey
	}
	return model, nil
}

func TestModelHandlersExposeCredentialEncryptionConfigurationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("create", func(t *testing.T) {
		svc := &tenantlessModelHandlerService{createErr: types.ErrModelCredentialEncryptionUnavailable}
		h := NewModelHandler(svc, nil)
		c, _ := tenantlessSystemAdminGinContext(http.MethodPost, "/api/v1/models", []byte(
			`{"name":"global-chat","type":"KnowledgeQA","source":"remote","parameters":{"api_key":"not-returned"}}`,
		))

		h.CreateModel(c)

		require.Len(t, c.Errors, 1)
		appErr, ok := apperrors.IsAppError(c.Errors[0].Err)
		require.True(t, ok)
		assert.Equal(t, apperrors.ErrBadRequest, appErr.Code)
		assert.Contains(t, appErr.Message, "SYSTEM_AES_KEY")
	})

	t.Run("credential subresource", func(t *testing.T) {
		svc := &tenantlessModelHandlerService{credentialErr: apperrors.NewBadRequestError(
			types.ErrModelCredentialEncryptionUnavailable.Error(),
		)}
		h := NewModelCredentialsHandler(svc, nil)
		c, _ := tenantlessSystemAdminGinContext(
			http.MethodPut, "/api/v1/models/global-chat/credentials", []byte(`{"api_key":"not-returned"}`),
		)
		c.Params = gin.Params{{Key: "id", Value: "global-chat"}}

		h.Put(c)

		require.Len(t, c.Errors, 1)
		appErr, ok := apperrors.IsAppError(c.Errors[0].Err)
		require.True(t, ok)
		assert.Equal(t, apperrors.ErrBadRequest, appErr.Code)
		assert.Contains(t, appErr.Message, "SYSTEM_AES_KEY")
	})
}

func tenantlessSystemAdminGinContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), types.SystemAdminContextKey, true))
	c.Request = req
	return c, recorder
}

func TestModelHandlerTenantlessSystemAdminUsesPlatformScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &tenantlessModelHandlerService{
		models: []*types.Model{{
			ID: "builtin-chat", IsBuiltin: true,
			Parameters: types.ModelParameters{APIKey: "never-return-this"},
		}},
	}
	h := NewModelHandler(svc, nil)

	t.Run("create accepts platform scope and redacts credentials", func(t *testing.T) {
		c, recorder := tenantlessSystemAdminGinContext(http.MethodPost, "/api/v1/models", []byte(
			`{"name":"global-chat","type":"KnowledgeQA","source":"remote","parameters":{"api_key":"never-return-this"}}`,
		))
		h.CreateModel(c)

		require.Empty(t, c.Errors)
		assert.Equal(t, http.StatusCreated, recorder.Code)
		require.NotNil(t, svc.created)
		assert.Zero(t, svc.created.TenantID)
		assert.NotContains(t, recorder.Body.String(), "never-return-this")
	})

	t.Run("list accepts platform scope and returns configured metadata only", func(t *testing.T) {
		c, recorder := tenantlessSystemAdminGinContext(http.MethodGet, "/api/v1/models", nil)
		h.ListModels(c)

		require.Empty(t, c.Errors)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "never-return-this")
		assert.Contains(t, recorder.Body.String(), `"configured":true`)
	})
}

func TestModelCredentialsHandlerTenantlessSystemAdminUsesEncryptedSubresourceContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &tenantlessModelHandlerService{}
	h := NewModelCredentialsHandler(svc, nil)
	c, recorder := tenantlessSystemAdminGinContext(
		http.MethodPut,
		"/api/v1/models/global-chat/credentials",
		[]byte(`{"api_key":"never-return-this"}`),
	)
	c.Params = gin.Params{{Key: "id", Value: "global-chat"}}

	h.Put(c)

	require.Empty(t, c.Errors)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "never-return-this")
	assert.Contains(t, recorder.Body.String(), `"configured":true`)
}
