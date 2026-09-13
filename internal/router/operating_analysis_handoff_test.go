package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type handoffSessionService struct {
	interfaces.SessionService
	session *types.Session
}

func (s *handoffSessionService) GetOwnedSession(_ context.Context, id string) (*types.Session, error) {
	if s.session == nil || s.session.ID != id {
		return nil, context.Canceled
	}
	return s.session, nil
}

type handoffMessageService struct {
	interfaces.MessageService
	message *types.Message
}

type handoffTenantService struct{ interfaces.TenantService }

func (handoffTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	return &types.Tenant{ID: id, Status: types.TenantStatusActive, AnalysisEnabled: true}, nil
}

type handoffMemberService struct {
	interfaces.TenantMemberService
	allowed bool
}

func (s *handoffMemberService) GetMembership(_ context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	return &types.TenantMember{
		UserID: userID, TenantID: tenantID, Status: types.TenantMemberStatusActive,
		OperatingAnalysisAccess: s.allowed,
	}, nil
}

func (s *handoffMessageService) GetMessage(_ context.Context, sessionID, id string) (*types.Message, error) {
	if s.message == nil || s.message.SessionID != sessionID || s.message.ID != id {
		return nil, context.Canceled
	}
	return s.message, nil
}

func handoffTestHandler(t *testing.T) (*operatingAnalysisHandoffHandler, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &operatingAnalysisHandoffHandler{
		store: &operatingAnalysisHandoffStore{rdb: client},
		sessionService: &handoffSessionService{session: &types.Session{
			ID: "session-1", TenantID: 31, UserID: "user-1",
		}},
		messageService: &handoffMessageService{message: &types.Message{
			ID: "message-1", SessionID: "session-1", Role: "user", Content: "分析本月门店销售变化",
		}},
		members: &handoffMemberService{allowed: true},
		tenants: handoffTenantService{},
	}, server
}

func handoffRequest(
	t *testing.T,
	httpMethod, target string,
	body []byte,
	tenantID uint64,
	actorID string,
	handler gin.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(httpMethod, "/", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(request.Context(), types.TenantIDContextKey, tenantID)
	ctx = context.WithValue(ctx, types.UserIDContextKey, actorID)
	ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: actorID})
	c.Request = request.WithContext(ctx)
	if target != "/" {
		c.Params = gin.Params{{Key: "ref", Value: target}}
	}
	handler(c)
	return recorder
}

func TestOperatingAnalysisHandoffConsumeRechecksCurrentMemberAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := handoffTestHandler(t)
	ref := createHandoff(t, handler)
	members := handler.members.(*handoffMemberService)
	members.allowed = false

	denied := handoffRequest(t, http.MethodPost, ref, nil, 31, "user-1", handler.Consume)
	require.Equal(t, http.StatusForbidden, denied.Code)

	members.allowed = true
	consumed := handoffRequest(t, http.MethodPost, ref, nil, 31, "user-1", handler.Consume)
	require.Equal(t, http.StatusOK, consumed.Code)
}

func createHandoff(t *testing.T, handler *operatingAnalysisHandoffHandler) string {
	t.Helper()
	recorder := handoffRequest(
		t,
		http.MethodPost,
		"/",
		[]byte(`{"sourceSessionId":"session-1","sourceMessageId":"message-1"}`),
		31,
		"user-1",
		handler.Create,
	)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Schema     string `json:"schema"`
		HandoffRef string `json:"handoffRef"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schema != "OperatingAnalysisHandoffV1" || len(body.HandoffRef) != 64 {
		t.Fatalf("unexpected create body: %s", recorder.Body.String())
	}
	return body.HandoffRef
}

func TestOperatingAnalysisHandoffIsActorBoundAndSingleUse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := handoffTestHandler(t)

	wrongActorRef := createHandoff(t, handler)
	wrongActor := handoffRequest(t, http.MethodPost, wrongActorRef, nil, 31, "user-2", handler.Consume)
	if wrongActor.Code != http.StatusNotFound {
		t.Fatalf("wrong actor status = %d", wrongActor.Code)
	}
	burned := handoffRequest(t, http.MethodPost, wrongActorRef, nil, 31, "user-1", handler.Consume)
	if burned.Code != http.StatusNotFound {
		t.Fatalf("burned handoff status = %d", burned.Code)
	}

	ref := createHandoff(t, handler)
	consumed := handoffRequest(t, http.MethodPost, ref, nil, 31, "user-1", handler.Consume)
	if consumed.Code != http.StatusOK {
		t.Fatalf("consume status = %d, body = %s", consumed.Code, consumed.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(consumed.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["schema"] != "OperatingAnalysisHandoffV1" || body["question"] != "分析本月门店销售变化" {
		t.Fatalf("unexpected consume body: %s", consumed.Body.String())
	}
	for _, forbidden := range []string{"answer", "sql", "sources", "runId", "evidence"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("consume response leaked %s", forbidden)
		}
	}
	replayed := handoffRequest(t, http.MethodPost, ref, nil, 31, "user-1", handler.Consume)
	if replayed.Code != http.StatusNotFound {
		t.Fatalf("replay status = %d", replayed.Code)
	}
}

func TestOperatingAnalysisHandoffExpiresAndConsumesAtomically(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, server := handoffTestHandler(t)

	expiredRef := createHandoff(t, handler)
	server.FastForward(operatingAnalysisHandoffTTL + time.Second)
	expired := handoffRequest(t, http.MethodPost, expiredRef, nil, 31, "user-1", handler.Consume)
	if expired.Code != http.StatusNotFound {
		t.Fatalf("expired status = %d", expired.Code)
	}

	ref := createHandoff(t, handler)
	var wg sync.WaitGroup
	wins := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := handoffRequest(t, http.MethodPost, ref, nil, 31, "user-1", handler.Consume)
			wins <- response.Code
		}()
	}
	wg.Wait()
	close(wins)
	okCount := 0
	for code := range wins {
		if code == http.StatusOK {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("successful concurrent consumes = %d", okCount)
	}
}

func TestOperatingAnalysisHandoffRejectsNonOwnedOrNonUserSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := handoffTestHandler(t)

	handler.sessionService.(*handoffSessionService).session.UserID = "user-2"
	notOwned := handoffRequest(
		t, http.MethodPost, "/",
		[]byte(`{"sourceSessionId":"session-1","sourceMessageId":"message-1"}`),
		31, "user-1", handler.Create,
	)
	if notOwned.Code != http.StatusNotFound {
		t.Fatalf("not-owned status = %d", notOwned.Code)
	}

	handler.sessionService.(*handoffSessionService).session.UserID = "user-1"
	handler.messageService.(*handoffMessageService).message.Role = "assistant"
	notUserMessage := handoffRequest(
		t, http.MethodPost, "/",
		[]byte(`{"sourceSessionId":"session-1","sourceMessageId":"message-1"}`),
		31, "user-1", handler.Create,
	)
	if notUserMessage.Code != http.StatusNotFound {
		t.Fatalf("non-user-message status = %d", notUserMessage.Code)
	}
}
