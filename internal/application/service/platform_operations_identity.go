package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrPlatformOperationConflict        = errors.New("platform operation conflicts with its durable receipt")
	ErrPlatformOperationNotFound        = errors.New("platform operation receipt not found")
	ErrPlatformOperationTargetForbidden = errors.New("platform operation target is forbidden")
)

type platformOperationsIdentityService struct {
	config   *config.Config
	repo     interfaces.PlatformOperationsIdentityRepository
	settings interfaces.SystemSettingService
}

func NewPlatformOperationsIdentityService(
	configInfo *config.Config,
	repo interfaces.PlatformOperationsIdentityRepository,
	settings interfaces.SystemSettingService,
) interfaces.PlatformOperationsIdentityService {
	return &platformOperationsIdentityService{config: configInfo, repo: repo, settings: settings}
}

func platformInitialAdministratorResult(
	receipt *types.PlatformInitialAdministratorReceipt,
	user *types.User,
	replayed bool,
) *types.PlatformInitialAdministratorResult {
	return &types.PlatformInitialAdministratorResult{
		CommandID: receipt.CommandID, UserID: user.ID, Username: user.Username,
		Email: user.Email, Status: receipt.Status, Replayed: replayed,
	}
}

func (s *platformOperationsIdentityService) CreateInitialAdministrator(
	ctx context.Context,
	actorUserID, commandID string,
	req *types.AdminCreateUserRequest,
) (*types.PlatformInitialAdministratorResult, error) {
	if req == nil || req.Password == nil || commandID == "" {
		return nil, ErrPasswordPolicy
	}
	username, email := strings.TrimSpace(req.Username), strings.TrimSpace(req.Email)
	parsedEmail, emailErr := mail.ParseAddress(email)
	if len(username) < 2 || len(username) > 50 || len(email) > 255 || emailErr != nil || parsedEmail.Address != email {
		return nil, ErrPasswordPolicy
	}
	password := *req.Password
	if err := ValidatePasswordPolicy(password, ResolveComplexPasswordEnabled(ctx, s.config, s.settings)); err != nil {
		return nil, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(username + "\x00" + email))
	user := &types.User{
		ID: uuid.NewString(), Username: username, Email: email,
		PasswordHash: string(passwordHash), IsActive: true,
	}
	receipt, stored, replayed, err := s.repo.CreateInitialAdministrator(
		ctx, actorUserID, commandID, hex.EncodeToString(digest[:]), password, user)
	if errors.Is(err, repository.ErrPlatformOperationConflict) {
		return nil, ErrPlatformOperationConflict
	}
	if errors.Is(err, repository.ErrMemberActionForbidden) {
		return nil, ErrMemberActionForbidden
	}
	if err != nil {
		return nil, err
	}
	return platformInitialAdministratorResult(receipt, stored, replayed), nil
}

func (s *platformOperationsIdentityService) GetInitialAdministrator(
	ctx context.Context, actorUserID, commandID string,
) (*types.PlatformInitialAdministratorResult, error) {
	if commandID == "" {
		return nil, ErrPlatformOperationNotFound
	}
	receipt, user, err := s.repo.GetInitialAdministrator(ctx, actorUserID, commandID)
	if errors.Is(err, repository.ErrPlatformOperationNotFound) {
		return nil, ErrPlatformOperationNotFound
	}
	if errors.Is(err, repository.ErrPlatformOperationConflict) {
		return nil, ErrPlatformOperationConflict
	}
	if errors.Is(err, repository.ErrMemberActionForbidden) {
		return nil, ErrMemberActionForbidden
	}
	if err != nil {
		return nil, err
	}
	return platformInitialAdministratorResult(receipt, user, true), nil
}

func (s *platformOperationsIdentityService) ResetEnterpriseMemberPassword(
	ctx context.Context,
	actorUserID string,
	tenantID uint64,
	targetUserID, newPassword string,
) error {
	if types.IsSyntheticUserID(targetUserID) {
		return ErrPlatformOperationTargetForbidden
	}
	if err := ValidatePasswordPolicy(newPassword, ResolveComplexPasswordEnabled(ctx, s.config, s.settings)); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	err = s.repo.ResetEnterpriseMemberPassword(ctx, actorUserID, tenantID, targetUserID, string(hash))
	if errors.Is(err, repository.ErrPlatformOperationTargetForbidden) {
		return ErrPlatformOperationTargetForbidden
	}
	if errors.Is(err, repository.ErrMemberActionForbidden) {
		return ErrMemberActionForbidden
	}
	if errors.Is(err, repository.ErrEnterpriseNotActive) {
		return ErrEnterpriseNotActive
	}
	return err
}
