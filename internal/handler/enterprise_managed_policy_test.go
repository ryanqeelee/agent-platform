package handler

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type enterpriseManagedSettingService struct {
	interfaces.SystemSettingService
}

func (s *enterpriseManagedSettingService) GetString(context.Context, string, string, string) string {
	return config.AuthDefaultTenantModeCreatePersonal
}

func TestEnterpriseManagedAdmissionSettingsCannotReenable(t *testing.T) {
	h := &AuthHandler{configInfo: &config.Config{Auth: &config.AuthConfig{RegistrationMode: config.AuthRegistrationModeSelfServe}}}
	if got := h.resolveRegistrationMode(context.Background()); got != config.AuthRegistrationModeInviteOnly {
		t.Fatalf("registration mode = %s", got)
	}
	on := true
	if resolveTenantSelfServiceCreationEnabled(context.Background(), &config.Config{Tenant: &config.TenantConfig{SelfServiceCreationEnabled: &on}}, nil) {
		t.Fatal("ordinary workspace creation must stay disabled")
	}
	h.systemSettingSvc = &enterpriseManagedSettingService{}
	if got := h.resolveDefaultTenantMode(context.Background()); got != types.TenantProvisioningTenantless {
		t.Fatalf("OIDC provisioning = %s, want tenantless", got)
	}
}
