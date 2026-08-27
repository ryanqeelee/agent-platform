package types

import "testing"

func TestMemberLifecycleRoleMatrix(t *testing.T) {
	cases := []struct {
		name          string
		actor, target TenantRole
		want          bool
	}{
		{"owner appoints admin", TenantRoleOwner, TenantRoleAdmin, true},
		{"owner manages knowledge administrator", TenantRoleOwner, TenantRoleContributor, true},
		{"owner manages employee", TenantRoleOwner, TenantRoleViewer, true},
		{"owner never normal-path manages owner", TenantRoleOwner, TenantRoleOwner, false},
		{"admin manages knowledge administrator", TenantRoleAdmin, TenantRoleContributor, true},
		{"admin manages employee", TenantRoleAdmin, TenantRoleViewer, true},
		{"admin cannot manage admin", TenantRoleAdmin, TenantRoleAdmin, false},
		{"admin cannot manage owner", TenantRoleAdmin, TenantRoleOwner, false},
		{"knowledge administrator cannot manage employee", TenantRoleContributor, TenantRoleViewer, false},
		{"employee cannot manage knowledge administrator", TenantRoleViewer, TenantRoleContributor, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanManageMemberRole(tc.actor, tc.target); got != tc.want {
				t.Fatalf("CanManageMemberRole(%q, %q) = %v, want %v", tc.actor, tc.target, got, tc.want)
			}
		})
	}
}
