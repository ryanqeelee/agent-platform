package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/policy"
	"github.com/Tencent/WeKnora/internal/types"
)

type enterpriseManagedKBShareService struct{}

func (enterpriseManagedKBShareService) ShareKnowledgeBase(context.Context, string, string, string, uint64, types.OrgMemberRole) (*types.KnowledgeBaseShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) UpdateSharePermission(context.Context, string, types.OrgMemberRole, string, uint64) error {
	return policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) RemoveShare(context.Context, string, string, uint64) error {
	return policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) ListSharesByKnowledgeBase(context.Context, string, uint64) ([]*types.KnowledgeBaseShare, error) {
	return []*types.KnowledgeBaseShare{}, nil
}
func (enterpriseManagedKBShareService) ListSharesByOrganization(context.Context, string) ([]*types.KnowledgeBaseShare, error) {
	return []*types.KnowledgeBaseShare{}, nil
}
func (enterpriseManagedKBShareService) ListSharedKnowledgeBases(context.Context, uint64, types.TenantRole) ([]*types.SharedKnowledgeBaseInfo, error) {
	return []*types.SharedKnowledgeBaseInfo{}, nil
}
func (enterpriseManagedKBShareService) ListSharedKnowledgeBasesInOrganization(context.Context, string, uint64, types.TenantRole) ([]*types.OrganizationSharedKnowledgeBaseItem, error) {
	return []*types.OrganizationSharedKnowledgeBaseItem{}, nil
}
func (enterpriseManagedKBShareService) ListSharedKnowledgeBaseIDsByOrganizations(context.Context, []string, uint64) (map[string][]string, error) {
	return map[string][]string{}, nil
}
func (enterpriseManagedKBShareService) GetShare(context.Context, string) (*types.KnowledgeBaseShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) GetShareByKBAndOrg(context.Context, string, string) (*types.KnowledgeBaseShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) CheckTenantKBPermission(context.Context, string, uint64, types.TenantRole) (types.OrgMemberRole, bool, error) {
	return "", false, nil
}
func (enterpriseManagedKBShareService) HasTenantKBPermission(context.Context, string, uint64, types.TenantRole, types.OrgMemberRole) (bool, error) {
	return false, nil
}
func (enterpriseManagedKBShareService) GetKBSourceTenant(context.Context, string) (uint64, error) {
	return 0, policy.ErrSharingUnavailable
}
func (enterpriseManagedKBShareService) CountSharesByKnowledgeBaseIDs(context.Context, []string) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (enterpriseManagedKBShareService) CountByOrganizations(context.Context, []string) (map[string]int64, error) {
	return map[string]int64{}, nil
}

type enterpriseManagedAgentShareService struct{}

func (enterpriseManagedAgentShareService) ShareAgent(context.Context, string, string, string, uint64, types.OrgMemberRole) (*types.AgentShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) RemoveShare(context.Context, string, string, uint64) error {
	return policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) ListSharesByAgent(context.Context, string, uint64) ([]*types.AgentShare, error) {
	return []*types.AgentShare{}, nil
}
func (enterpriseManagedAgentShareService) ListSharesByOrganization(context.Context, string) ([]*types.AgentShare, error) {
	return []*types.AgentShare{}, nil
}
func (enterpriseManagedAgentShareService) ListSharedAgents(context.Context, uint64, types.TenantRole) ([]*types.SharedAgentInfo, error) {
	return []*types.SharedAgentInfo{}, nil
}
func (enterpriseManagedAgentShareService) ListSharedAgentsInOrganization(context.Context, string, uint64, types.TenantRole) ([]*types.OrganizationSharedAgentItem, error) {
	return []*types.OrganizationSharedAgentItem{}, nil
}
func (enterpriseManagedAgentShareService) ListSharedAgentsInOrganizations(context.Context, []string, uint64, types.TenantRole) (map[string][]*types.OrganizationSharedAgentItem, error) {
	return map[string][]*types.OrganizationSharedAgentItem{}, nil
}
func (enterpriseManagedAgentShareService) SetSharedAgentDisabledByMe(context.Context, uint64, string, uint64, bool) error {
	return policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) GetSharedAgentForTenant(context.Context, uint64, types.TenantRole, string, ...uint64) (*types.CustomAgent, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) TenantCanAccessKBViaSomeSharedAgent(context.Context, uint64, types.TenantRole, *types.KnowledgeBase) (bool, error) {
	return false, nil
}
func (enterpriseManagedAgentShareService) GetShare(context.Context, string) (*types.AgentShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) GetShareByAgentAndOrg(context.Context, string, string) (*types.AgentShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) GetShareByAgentIDForTenant(context.Context, uint64, string, uint64) (*types.AgentShare, error) {
	return nil, policy.ErrSharingUnavailable
}
func (enterpriseManagedAgentShareService) CountByOrganizations(context.Context, []string) (map[string]int64, error) {
	return map[string]int64{}, nil
}
