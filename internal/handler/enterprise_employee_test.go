package handler

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type employeeUserStub struct {
	interfaces.UserService
	tenant uint64
	req    *types.RegisterRequest
}

func (s *employeeUserStub) CreateEnterpriseEmployee(_ context.Context, tenant uint64, req *types.RegisterRequest) (*types.User, *types.TenantMember, error) {
	s.tenant = tenant
	s.req = req
	return &types.User{ID: "employee", Username: req.Username, Email: req.Email}, &types.TenantMember{UserID: "employee", TenantID: tenant, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}, nil
}
func TestEnterpriseEmployeeIgnoresClientAuthority(t *testing.T) {
	s := &employeeUserStub{}
	h := &TenantMemberHandler{userService: s}
	r := gin.New()
	r.POST("/tenants/:id/employees", h.CreateEmployee)
	body := `{"username":" Alice ","email":"alice@example.invalid","password":" Password123 ","tenant_id":99,"role":"admin","is_system_admin":true,"operating_analysis_access":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/tenants/1/employees", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 201 || s.tenant != 1 || s.req.Password != " Password123 " || s.req.Username != "Alice" {
		t.Fatalf("status=%d tenant=%d", w.Code, s.tenant)
	}
	if strings.Contains(w.Body.String(), "Password123") || !strings.Contains(w.Body.String(), `"role":"viewer"`) || !strings.Contains(w.Body.String(), `"operating_analysis_access":false`) {
		t.Fatal("credential or authority leaked")
	}
}
