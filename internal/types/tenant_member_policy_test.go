package types

import "testing"

func TestMemberLifecycleRoleMatrix(t *testing.T) {
	roles := []TenantRole{TenantRoleAdmin, TenantRoleViewer, TenantRoleOwner, TenantRoleContributor, "unknown"}
	for _, actor := range roles {
		for _, target := range roles {
			want := actor == TenantRoleAdmin && (target == TenantRoleAdmin || target == TenantRoleViewer)
			if CanManageMemberRole(actor, target) != want || CanInviteMemberRole(actor, target) != want {
				t.Errorf("actor=%s target=%s want=%v", actor, target, want)
			}
		}
	}
	for _, retired := range []TenantRole{TenantRoleOwner, TenantRoleContributor, "unknown"} {
		if retired.IsValid() || retired.HasPermission(TenantRoleViewer) || TenantRoleAdmin.HasPermission(retired) {
			t.Fatalf("retired role accepted: %s", retired)
		}
	}
}
