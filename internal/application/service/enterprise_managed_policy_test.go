package service

import (
	"context"
	"errors"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/policy"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestEnterpriseManagedMemberAndInvitationPolicy(t *testing.T) {
	memberSvc, memberRepo := newServiceWithRepo()
	if member, err := memberSvc.AddMember(context.Background(), "contributor", 1, types.TenantRoleContributor, nil); err != nil || member.Role != types.TenantRoleContributor {
		t.Fatalf("contributor must remain available: member=%v err=%v", member, err)
	}
	memberRepo.failCreate = apprepo.ErrUserBoundToAnotherEnterprise
	if _, err := memberSvc.AddMember(context.Background(), "bound", 2, types.TenantRoleViewer, nil); !errors.Is(err, ErrUserBoundToAnotherEnterprise) {
		t.Fatalf("cross-enterprise add: %v", err)
	}

	invSvc, invRepo, invitedMemberSvc := newInvitationSvc()
	invitedRepo := invitedMemberSvc.(*tenantMemberService).repo.(*fakeTenantMemberRepo)
	invitedRepo.failCreate = apprepo.ErrUserBoundToAnotherEnterprise
	invRepo.rows = append(invRepo.rows, &types.TenantInvitation{
		ID: 1, TenantID: 2, InviteeUserID: "bound", Role: types.TenantRoleContributor,
		Status: types.TenantInvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	})
	if _, err := invSvc.Accept(context.Background(), 1, "bound"); !errors.Is(err, ErrUserBoundToAnotherEnterprise) {
		t.Fatalf("cross-enterprise invitation accept: %v", err)
	}
	row, _ := invRepo.GetByID(context.Background(), 1)
	if row.Status != types.TenantInvitationStatusPending {
		t.Fatalf("cross-enterprise rejection consumed invitation: %s", row.Status)
	}
}

func TestEnterpriseManagedSharingAdapters(t *testing.T) {
	kb := NewKBShareService()
	if ok, err := kb.HasTenantKBPermission(context.Background(), "kb", 1, types.TenantRoleViewer, types.OrgRoleViewer); err != nil || ok {
		t.Fatalf("KB access must be denied, ok=%v err=%v", ok, err)
	}
	agent := NewAgentShareService()
	if got, err := agent.FindSharedAgentForKnowledgeBase(context.Background(), 1, types.TenantRoleViewer, &types.KnowledgeBase{}); err != nil || got != nil {
		t.Fatalf("agent share access must be denied, got=%v err=%v", got, err)
	}
	if _, err := kb.ShareKnowledgeBase(context.Background(), "kb", "org", "u", 1, types.OrgRoleViewer); !errors.Is(err, policy.ErrSharingUnavailable) {
		t.Fatalf("new KB share: %v", err)
	}
}
