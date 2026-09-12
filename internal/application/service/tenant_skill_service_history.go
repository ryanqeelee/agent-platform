package service

import (
	"context"
	"errors"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// GetInstallTranscriptHistory returns only the durable prompt and assistant
// message named by one tenant-scoped skill installation. The skill lookup is
// the authority boundary; callers cannot supply session or message locators.
func (s *TenantSkillService) GetInstallTranscriptHistory(
	ctx context.Context, tenantID uint64, configID, skillID string,
) ([]*types.Message, error) {
	skill, err := s.GetSkill(ctx, tenantID, configID, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	if skill.InstallSessionID == "" || skill.InstallMessageID == "" {
		if skill.Status == types.SkillStatusInstalling {
			return []*types.Message{}, nil
		}
		return nil, apperrors.NewNotFoundError("this skill has no install transcript history")
	}
	if s.messages == nil {
		return nil, apperrors.NewInternalServerError("install transcript history is unavailable")
	}

	prompt, err := s.messages.GetFirstMessageOfUser(ctx, skill.InstallSessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("install transcript history not found")
		}
		return nil, err
	}
	assistant, err := s.messages.GetMessage(ctx, skill.InstallSessionID, skill.InstallMessageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("install transcript history not found")
		}
		return nil, err
	}
	if prompt == nil || prompt.Role != "user" || assistant == nil || assistant.Role != "assistant" {
		return nil, apperrors.NewNotFoundError("install transcript history not found")
	}
	return []*types.Message{prompt, assistant}, nil
}
