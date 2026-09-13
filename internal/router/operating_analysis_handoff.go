package router

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const operatingAnalysisHandoffTTL = 2 * time.Minute

var errOperatingAnalysisHandoffUnavailable = errors.New("operating analysis handoff unavailable")

type operatingAnalysisHandoff struct {
	TenantID        uint64 `json:"tenant_id"`
	ActorID         string `json:"actor_id"`
	SourceSessionID string `json:"source_session_id"`
	SourceMessageID string `json:"source_message_id"`
}

type operatingAnalysisHandoffStore struct {
	rdb *redis.Client
}

func (s *operatingAnalysisHandoffStore) key(ref string) string {
	namespace := strings.TrimSpace(os.Getenv("WEKNORA_REDIS_NAMESPACE"))
	if namespace != "" {
		return "weknora:operating_analysis_handoff:" + namespace + ":" + ref
	}
	return "weknora:operating_analysis_handoff:" + ref
}

func (s *operatingAnalysisHandoffStore) Put(ctx context.Context, ref string, value operatingAnalysisHandoff) error {
	if s.rdb == nil {
		return errOperatingAnalysisHandoffUnavailable
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	stored, err := s.rdb.SetNX(ctx, s.key(ref), data, operatingAnalysisHandoffTTL).Result()
	if err != nil || !stored {
		return errOperatingAnalysisHandoffUnavailable
	}
	return nil
}

func (s *operatingAnalysisHandoffStore) Take(ctx context.Context, ref string) (operatingAnalysisHandoff, error) {
	if s.rdb == nil {
		return operatingAnalysisHandoff{}, errOperatingAnalysisHandoffUnavailable
	}
	data, err := s.rdb.GetDel(ctx, s.key(ref)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return operatingAnalysisHandoff{}, nil
		}
		return operatingAnalysisHandoff{}, errOperatingAnalysisHandoffUnavailable
	}
	var value operatingAnalysisHandoff
	if err := json.Unmarshal(data, &value); err != nil {
		return operatingAnalysisHandoff{}, errOperatingAnalysisHandoffUnavailable
	}
	return value, nil
}

type operatingAnalysisHandoffHandler struct {
	store          *operatingAnalysisHandoffStore
	sessionService interfaces.SessionService
	messageService interfaces.MessageService
	members        interfaces.TenantMemberService
	tenants        interfaces.TenantService
}

type createOperatingAnalysisHandoffRequest struct {
	SourceSessionID string `json:"sourceSessionId" binding:"required"`
	SourceMessageID string `json:"sourceMessageId" binding:"required"`
}

func (h *operatingAnalysisHandoffHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	permission, permissionErr := service.CurrentOperatingAnalysisReadPermission(ctx, h.members, h.tenants)
	if permissionErr != nil || !permission.Allowed() {
		c.JSON(http.StatusForbidden, gin.H{"error": "operating analysis handoff not permitted"})
		return
	}
	tenantID, actorID, ok := operatingAnalysisHandoffActor(ctx)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "operating analysis handoff not permitted"})
		return
	}
	var request createOperatingAnalysisHandoffRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operating analysis handoff"})
		return
	}
	session, err := h.sessionService.GetOwnedSession(ctx, request.SourceSessionID)
	if err != nil || session == nil || session.TenantID != tenantID || session.UserID != actorID {
		c.JSON(http.StatusNotFound, gin.H{"error": "operating analysis handoff source not found"})
		return
	}
	message, err := h.messageService.GetMessage(ctx, request.SourceSessionID, request.SourceMessageID)
	if err != nil || message == nil || message.SessionID != request.SourceSessionID || message.Role != "user" || strings.TrimSpace(message.Content) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "operating analysis handoff source not found"})
		return
	}
	ref, err := newOperatingAnalysisHandoffRef()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "operating analysis handoff unavailable"})
		return
	}
	expiresAt := time.Now().UTC().Add(operatingAnalysisHandoffTTL)
	if err := h.store.Put(ctx, ref, operatingAnalysisHandoff{
		TenantID:        tenantID,
		ActorID:         actorID,
		SourceSessionID: session.ID,
		SourceMessageID: message.ID,
	}); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "operating analysis handoff unavailable"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"schema":     "OperatingAnalysisHandoffV1",
		"handoffRef": ref,
		"expiresAt":  expiresAt,
	})
}

func (h *operatingAnalysisHandoffHandler) Consume(c *gin.Context) {
	ctx := c.Request.Context()
	permission, permissionErr := service.CurrentOperatingAnalysisReadPermission(ctx, h.members, h.tenants)
	if permissionErr != nil || !permission.Allowed() {
		c.JSON(http.StatusForbidden, gin.H{"error": "operating analysis handoff not permitted"})
		return
	}
	tenantID, actorID, ok := operatingAnalysisHandoffActor(ctx)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "operating analysis handoff not permitted"})
		return
	}
	value, err := h.store.Take(ctx, strings.TrimSpace(c.Param("ref")))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "operating analysis handoff unavailable"})
		return
	}
	if value.TenantID != tenantID || value.ActorID != actorID {
		c.JSON(http.StatusNotFound, gin.H{"error": "operating analysis handoff not found"})
		return
	}
	session, err := h.sessionService.GetOwnedSession(ctx, value.SourceSessionID)
	if err != nil || session == nil || session.TenantID != tenantID || session.UserID != actorID {
		c.JSON(http.StatusNotFound, gin.H{"error": "operating analysis handoff not found"})
		return
	}
	message, err := h.messageService.GetMessage(ctx, value.SourceSessionID, value.SourceMessageID)
	if err != nil || message == nil || message.SessionID != value.SourceSessionID || message.Role != "user" {
		c.JSON(http.StatusNotFound, gin.H{"error": "operating analysis handoff not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"schema":   "OperatingAnalysisHandoffV1",
		"question": message.Content,
	})
}

func operatingAnalysisHandoffActor(ctx context.Context) (uint64, string, bool) {
	if _, apiKey := types.TenantAPIKeyScopeFromContext(ctx); apiKey {
		return 0, "", false
	}
	tenantID, tenantOK := types.TenantIDFromContext(ctx)
	actorID, actorOK := types.UserIDFromContext(ctx)
	if !tenantOK || tenantID == 0 || !actorOK || strings.TrimSpace(actorID) == "" || types.IsSyntheticUserID(actorID) {
		return 0, "", false
	}
	return tenantID, actorID, true
}

func newOperatingAnalysisHandoffRef() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func RegisterOperatingAnalysisHandoffRoutes(
	r *gin.RouterGroup,
	rdb *redis.Client,
	sessionService interfaces.SessionService,
	messageService interfaces.MessageService,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
	g *rbacGuards,
) {
	handler := &operatingAnalysisHandoffHandler{
		store:          &operatingAnalysisHandoffStore{rdb: rdb},
		sessionService: sessionService,
		messageService: messageService,
		members:        members,
		tenants:        tenants,
	}
	routes := r.Group("/operating-analysis-handoffs", g.Viewer())
	routes.POST("", handler.Create)
	routes.POST("/:ref/consume", handler.Consume)
}
