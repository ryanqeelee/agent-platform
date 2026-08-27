package service

import (
	"errors"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	ErrKBNotFound         = errors.New("knowledge base not found")
	ErrNotKBOwner         = errors.New("only knowledge base owner can share")
	ErrOrgRoleCannotShare = errors.New("only editors and admins can share knowledge bases to this organization")
)

// NewKBShareService keeps the existing interface wiring while the platform,
// rather than enterprise users, owns cross-enterprise sharing.
func NewKBShareService() interfaces.KBShareService {
	return enterpriseManagedKBShareService{}
}
