package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildLoginMemberships_SuspendedOnlyDoesNotSynthesizeViewer(t *testing.T) {
	members, repo := newServiceWithRepo()
	repo.rows = []*types.TenantMember{{
		UserID: "suspended", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusSuspended,
	}}
	svc := &userService{memberService: members}
	got := svc.BuildLoginMemberships(context.Background(), &types.User{ID: "suspended", TenantID: 1}, &types.Tenant{ID: 1, Name: "home"})
	if len(got) != 0 {
		t.Fatalf("suspended-only memberships = %#v, want no fallback viewer", got)
	}
}
