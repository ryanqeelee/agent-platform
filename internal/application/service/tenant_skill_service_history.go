package service

import (
	"context"
	"encoding/json"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

// GetInstallTranscriptHistory returns the durable user/assistant message DTOs
// for the latest install. The row lookup is the authority boundary; callers
// cannot supply a transcript or run locator.
func (s *TenantSkillService) GetInstallTranscriptHistory(
	ctx context.Context, configID, skillID string,
) ([]*types.Message, error) {
	skill, err := s.GetSkill(ctx, configID, skillID)
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	if skill.InstallRunID == "" {
		if skill.Status == types.SkillStatusInstalling {
			return []*types.Message{}, nil
		}
		return nil, apperrors.NewNotFoundError("this skill has no install transcript history")
	}
	if len(skill.InstallTranscript) == 0 {
		if skill.Status == types.SkillStatusInstalling {
			return []*types.Message{}, nil
		}
		return nil, apperrors.NewNotFoundError("install transcript history not found")
	}
	messages := []*types.Message{}
	if err := json.Unmarshal(skill.InstallTranscript, &messages); err != nil {
		return nil, err
	}
	if len(messages) != 2 || messages[0] == nil || messages[0].Role != "user" ||
		messages[1] == nil || messages[1].Role != "assistant" {
		return nil, apperrors.NewNotFoundError("install transcript history not found")
	}
	return messages, nil
}
