package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestInvitationProjectionUsesTargetRoleMatrix(t *testing.T) {
	h := &TenantInvitationHandler{configInfo: &config.Config{FrontendBaseURL: "https://retail.example"}}
	inv := &types.TenantInvitation{
		ID: 1, TenantID: 7, Token: "admin-link", Role: types.TenantRoleAdmin,
		Status: types.TenantInvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	actorContext := func(role types.TenantRole, platform bool) context.Context {
		ctx := context.WithValue(context.Background(), types.TenantRoleContextKey, role)
		return context.WithValue(ctx, types.CrossTenantAccessContextKey, platform)
	}

	if got := h.projectInvitationForActor(actorContext(types.TenantRoleViewer, false), inv, nil); got.InviteURL != "" {
		t.Fatalf("Admin received an Admin invitation link: %q", got.InviteURL)
	}
	if got := h.projectInvitationForActor(actorContext(types.TenantRoleAdmin, false), inv, nil); got.InviteURL == "" {
		t.Fatal("Owner could not retrieve an Admin invitation link")
	}
	if got := h.projectInvitationForActor(actorContext(types.TenantRoleAdmin, true), inv, nil); got.InviteURL == "" {
		t.Fatal("effective platform operator could not retrieve a non-Owner invitation link")
	}
}
