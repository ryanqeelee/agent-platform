package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

var ErrInvalidMemoryContract = errors.New("memory: invalid contract payload")

const memorySnapshotMaxItems = 50

func (s *Service) Snapshot(ctx context.Context, consumer string) (*types.PersonalMemorySnapshot, error) {
	if consumer == "" {
		consumer = types.MemoryConsumerAnalysis
	}
	if !types.IsValidMemoryConsumer(consumer) {
		return nil, fmt.Errorf("%w: invalid consumer", ErrInvalidMemoryContract)
	}
	scope, err := ResolveScope(ctx)
	if err != nil {
		return nil, err
	}
	var (
		state interfaces.MemoryAuthorityState
		items []*types.MemoryItem
	)
	_, err = s.repo.WithAuthority(ctx, scope, interfaces.MemoryAuthorityRequest{}, func(
		ctx context.Context, repo interfaces.MemoryRepository, current interfaces.MemoryAuthorityState,
	) (*interfaces.MemoryAuthorityMutation, error) {
		state = current
		kinds := types.MemoryKinds
		if consumer == types.MemoryConsumerAnalysis {
			// The analysis projection intentionally starts with preferences only.
			kinds = []string{types.MemoryKindPreference}
		}
		var loadErr error
		items, loadErr = repo.ListApplicableItems(ctx, scope, consumer, kinds, memorySnapshotMaxItems)
		return &interfaces.MemoryAuthorityMutation{}, loadErr
	})
	if err != nil {
		return nil, err
	}
	status := "available"
	if state.Config == nil || !state.Config.MemoryEnabled() || !state.UserEnabled {
		status = "disabled"
		items = nil
	}
	writeMode := types.MemoryWriteExplicitOnly
	workspaceEnabled := false
	if state.Config != nil {
		writeMode = state.Config.WriteMode
		workspaceEnabled = state.Config.MemoryEnabled()
	}
	projected := make([]*types.PersonalMemorySnapshotItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		projected = append(projected, &types.PersonalMemorySnapshotItem{
			ID: item.ID, Scope: item.Scope, Kind: item.Kind, Topic: item.Topic,
			Content: item.Content, Importance: item.Importance, Origin: item.Origin,
			Status: types.MemoryStatusActive,
		})
	}
	return &types.PersonalMemorySnapshot{
		Schema: types.PersonalMemorySnapshotSchema, Status: status, Consumer: consumer,
		Policy: types.PersonalMemoryPolicy{
			WorkspaceEnabled: workspaceEnabled, UserEnabled: state.UserEnabled, WriteMode: writeMode,
			WorkspaceGeneration: state.WorkspaceGeneration, SubjectGeneration: state.SubjectGeneration,
		},
		Revision: state.Revision, Items: projected,
	}, nil
}

func canonicalHash(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateCommandShape(command *types.PersonalMemoryCommand) error {
	if command == nil || command.Schema != types.PersonalMemoryCommandSchema {
		return fmt.Errorf("%w: schema must be %s", ErrInvalidMemoryContract, types.PersonalMemoryCommandSchema)
	}
	if strings.TrimSpace(command.OperationID) == "" || len(command.OperationID) > 128 {
		return fmt.Errorf("%w: operation_id is required", ErrInvalidMemoryContract)
	}
	if command.Expected == nil || command.Expected.WorkspaceGeneration < 0 ||
		command.Expected.SubjectGeneration < 0 || command.Expected.Revision < 0 {
		return fmt.Errorf("%w: expected is required", ErrInvalidMemoryContract)
	}
	if len(command.Changes) == 0 || len(command.Changes) > 20 {
		return fmt.Errorf("%w: changes must contain 1 to 20 entries", ErrInvalidMemoryContract)
	}
	if strings.TrimSpace(command.Source.SessionID) == "" || len(command.Source.SessionID) > 128 ||
		strings.TrimSpace(command.Source.MessageID) == "" || len(command.Source.MessageID) > 128 {
		return fmt.Errorf("%w: source session_id and message_id are required", ErrInvalidMemoryContract)
	}
	seen := make(map[string]struct{}, len(command.Changes))
	for index := range command.Changes {
		change := &command.Changes[index]
		switch change.Op {
		case types.MemoryChangeCreate:
			if change.ID != "" || strings.TrimSpace(change.Content) == "" || !types.IsValidMemoryKind(change.Kind) {
				return fmt.Errorf("%w: invalid create change", ErrInvalidMemoryContract)
			}
		case types.MemoryChangeUpdate:
			if change.ID == "" || (change.Content == "" && change.Scope == "" && change.Kind == "" &&
				change.Topic == "" && change.Importance == nil) {
				return fmt.Errorf("%w: invalid update change", ErrInvalidMemoryContract)
			}
		case types.MemoryChangeDelete:
			if change.ID == "" || change.Content != "" || change.Scope != "" || change.Kind != "" || change.Topic != "" || change.Importance != nil {
				return fmt.Errorf("%w: invalid delete change", ErrInvalidMemoryContract)
			}
		default:
			return fmt.Errorf("%w: unsupported change operation", ErrInvalidMemoryContract)
		}
		if change.ID != "" {
			if _, duplicate := seen[change.ID]; duplicate {
				return fmt.Errorf("%w: duplicate item id", ErrInvalidMemoryContract)
			}
			seen[change.ID] = struct{}{}
		}
		if change.Scope != "" && !types.IsValidMemoryScope(change.Scope) {
			return fmt.Errorf("%w: invalid memory scope", ErrInvalidMemoryContract)
		}
		if change.Kind != "" && !types.IsValidMemoryKind(change.Kind) {
			return fmt.Errorf("%w: invalid memory kind", ErrInvalidMemoryContract)
		}
		if change.Importance != nil && (*change.Importance < 1 || *change.Importance > 5) {
			return fmt.Errorf("%w: importance must be between 1 and 5", ErrInvalidMemoryContract)
		}
		if utf8.RuneCountInString(change.Content) > 300 || utf8.RuneCountInString(change.Topic) > 80 {
			return fmt.Errorf("%w: content or topic exceeds its limit", ErrInvalidMemoryContract)
		}
	}
	return nil
}

func validCommandSource(source types.PersonalMemoryCommandSource, allowAuto bool) bool {
	if source.Runtime != types.MemoryConsumerEmployee && source.Runtime != types.MemoryConsumerAnalysis {
		return false
	}
	if source.Mode == types.MemoryCommandModeExplicit || source.Mode == types.MemoryCommandModeManual {
		return true
	}
	return allowAuto && source.Mode == types.MemoryCommandModeAuto
}

func (s *Service) ApplyCommand(
	ctx context.Context, command *types.PersonalMemoryCommand,
) (*types.PersonalMemoryReceipt, error) {
	scope, err := ResolveScope(ctx)
	if err != nil {
		return nil, err
	}
	return s.applyCommandForScope(ctx, scope, command, false)
}

func (s *Service) applyCommand(
	ctx context.Context, command *types.PersonalMemoryCommand, allowAuto bool,
) (*types.PersonalMemoryReceipt, error) {
	scope, err := ResolveScope(ctx)
	if err != nil {
		return nil, err
	}
	return s.applyCommandForScope(ctx, scope, command, allowAuto)
}

func (s *Service) applyCommandForScope(
	ctx context.Context, scope interfaces.MemoryScope, command *types.PersonalMemoryCommand, allowAuto bool,
) (*types.PersonalMemoryReceipt, error) {
	if err := validateCommandShape(command); err != nil {
		return nil, err
	}
	hash, err := canonicalHash(command)
	if err != nil {
		return nil, err
	}
	preReject := ""
	if !validCommandSource(command.Source, allowAuto) {
		preReject = types.MemoryReasonInvalidSource
	}
	bumpGeneration := false
	for _, change := range command.Changes {
		if change.Op == types.MemoryChangeDelete {
			bumpGeneration = true
			break
		}
	}
	newCommand := false
	receipt, err := s.repo.WithAuthority(ctx, scope, interfaces.MemoryAuthorityRequest{
		Expected: command.Expected, RequireEnabled: true,
		RequireAuto:    command.Source.Mode == types.MemoryCommandModeAuto,
		BumpGeneration: bumpGeneration, OperationID: command.OperationID,
		CommandHash: hash, PreRejectReason: preReject,
	}, func(
		ctx context.Context, repo interfaces.MemoryRepository, state interfaces.MemoryAuthorityState,
	) (*interfaces.MemoryAuthorityMutation, error) {
		newCommand = true
		txService := *s
		txService.repo = repo
		txService.deferEmbeddings = true
		return txService.applyCommandChanges(ctx, scope, state.Config, command)
	})
	if err != nil || receipt == nil || !newCommand || receipt.Status != types.MemoryReceiptApplied {
		return receipt, err
	}
	// Embeddings are deliberately outside the transaction. A concurrent delete
	// may make an item disappear; reloading by scoped ID makes that a harmless
	// no-op and can never recreate the memory item.
	cfg := s.workspaceConfig(ctx, scope.TenantID)
	for _, id := range receipt.ItemIDs {
		item, loadErr := s.repo.GetItem(ctx, scope, id)
		if loadErr != nil || item == nil || item.Status != types.MemoryStatusActive {
			continue
		}
		s.storeItemEmbedding(ctx, scope, cfg, item)
	}
	return receipt, nil
}

func sanitizeContractContent(content string) (string, string) {
	content = types.SanitizeMemoryContent(content)
	if content == "" {
		return "", types.MemoryReasonSensitiveContent
	}
	if redacted, changed := types.RedactSensitive(content); changed {
		if types.IsMostlyRedacted(redacted) {
			return "", types.MemoryReasonSensitiveContent
		}
		content = types.SanitizeMemoryContent(redacted)
	}
	if content == "" {
		return "", types.MemoryReasonSensitiveContent
	}
	return content, ""
}

func defaultChangeScope(command *types.PersonalMemoryCommand) string {
	if command.Source.Mode == types.MemoryCommandModeAuto {
		if command.Source.Runtime == types.MemoryConsumerAnalysis {
			return types.MemoryScopeAnalysis
		}
		return types.MemoryScopeEmployee
	}
	return types.MemoryScopeShared
}

func (s *Service) applyCommandChanges(
	ctx context.Context,
	scope interfaces.MemoryScope,
	cfg *types.MemoryConfig,
	command *types.PersonalMemoryCommand,
) (*interfaces.MemoryAuthorityMutation, error) {
	prepared := make([]types.PersonalMemoryChange, len(command.Changes))
	copy(prepared, command.Changes)
	existing := make(map[string]*types.MemoryItem, len(command.Changes))
	for index := range prepared {
		change := &prepared[index]
		if change.ID != "" {
			item, err := s.repo.GetItem(ctx, scope, change.ID)
			if err != nil {
				return nil, err
			}
			if item == nil {
				return &interfaces.MemoryAuthorityMutation{ReasonCode: types.MemoryReasonItemNotFound}, nil
			}
			existing[change.ID] = item
		}
		if change.Op == types.MemoryChangeCreate {
			if change.Scope == "" {
				change.Scope = defaultChangeScope(command)
			}
			if change.Importance == nil {
				importance := 3
				change.Importance = &importance
			}
		}
		if change.Op == types.MemoryChangeCreate || change.Content != "" {
			content, reason := sanitizeContractContent(change.Content)
			if reason != "" {
				return &interfaces.MemoryAuthorityMutation{ReasonCode: reason}, nil
			}
			change.Content = content
		}
		change.Topic = types.SanitizeMemoryTopic(change.Topic)
		if change.Op == types.MemoryChangeCreate && command.Source.Mode == types.MemoryCommandModeAuto {
			forgotten, err := s.repo.HasTombstone(ctx, scope, types.MemoryFingerprint(change.Content))
			if err != nil {
				return nil, err
			}
			if !forgotten {
				forgotten, err = s.repo.HasTombstoneForMessage(ctx, scope, command.Source.MessageID, rejectedMessageWindow)
				if err != nil {
					return nil, err
				}
			}
			if forgotten {
				return &interfaces.MemoryAuthorityMutation{ReasonCode: types.MemoryReasonPreviouslyForgot}, nil
			}
		}
	}

	mutation := &interfaces.MemoryAuthorityMutation{}
	for index := range prepared {
		change := &prepared[index]
		switch change.Op {
		case types.MemoryChangeCreate:
			id := uuid.NewString()
			origin := change.InternalOrigin
			if origin == "" {
				origin = types.MemoryOriginManual
				if command.Source.Mode == types.MemoryCommandModeExplicit {
					origin = types.MemoryOriginExplicit
				} else if command.Source.Mode == types.MemoryCommandModeAuto {
					origin = types.MemoryOriginExtracted
				}
			}
			stored, err := s.write(ctx, scope, cfg, types.MemoryItem{
				ID: id, Scope: change.Scope, Kind: change.Kind, Topic: change.Topic,
				Content: change.Content, Importance: *change.Importance, Origin: origin,
				SourceSessionID: command.Source.SessionID, SourceMessageID: command.Source.MessageID,
				Inferred: change.InternalInferred, ExpiresAt: change.InternalExpiresAt,
			})
			if err != nil {
				return nil, err
			}
			if stored != nil {
				mutation.ItemIDs = append(mutation.ItemIDs, stored.ID)
				mutation.Mutated = mutation.Mutated || stored.ID == id
			}
		case types.MemoryChangeUpdate:
			item := *existing[change.ID]
			before := item
			if change.Scope != "" {
				item.Scope = change.Scope
			}
			if change.Kind != "" {
				item.Kind = change.Kind
			}
			if change.Topic != "" {
				item.Topic = change.Topic
			}
			if change.Content != "" {
				item.Content = change.Content
			}
			if change.Importance != nil {
				item.Importance = *change.Importance
			}
			item.NormalizedKey = types.MemoryItemKey(item.Topic, item.Content)
			if command.Source.Mode != types.MemoryCommandModeAuto {
				item.Origin = types.MemoryOriginManual
			}
			if before.Scope != item.Scope || before.Kind != item.Kind || before.Topic != item.Topic ||
				before.Content != item.Content || before.Importance != item.Importance || before.Origin != item.Origin {
				if before.Kind != item.Kind || before.Topic != item.Topic || before.Content != item.Content {
					if err := s.repo.DeleteItemEmbedding(ctx, scope, item.ID); err != nil {
						return nil, err
					}
				}
				if err := s.repo.UpdateItem(ctx, scope, &item); err != nil {
					return nil, err
				}
				mutation.Mutated = true
			}
			mutation.ItemIDs = append(mutation.ItemIDs, item.ID)
		case types.MemoryChangeDelete:
			item := existing[change.ID]
			if err := s.repo.AddTombstone(ctx, scope, item.Topic,
				types.MemoryFingerprint(item.Content), item.SourceMessageID); err != nil {
				return nil, err
			}
			if err := s.repo.DeleteItem(ctx, scope, item.ID); err != nil {
				return nil, err
			}
			mutation.Mutated = true
			mutation.ItemIDs = append(mutation.ItemIDs, item.ID)
		}
	}
	if mutation.Mutated {
		mutation.BumpRevision = true
		if err := s.enforceCapacityAtomic(ctx, scope, cfg); err != nil {
			return nil, err
		}
		if err := s.rebuildBlockAtomic(ctx, scope); err != nil {
			return nil, err
		}
	}
	if command.InternalExpressionID != "" {
		if err := s.repo.MarkExpressionsProcessed(
			ctx, scope, []string{command.InternalExpressionID}, "",
		); err != nil {
			return nil, err
		}
	}
	return mutation, nil
}

func (s *Service) GetCommandReceipt(
	ctx context.Context, operationID string,
) (*types.PersonalMemoryReceipt, error) {
	if strings.TrimSpace(operationID) == "" {
		return nil, fmt.Errorf("%w: operation_id is required", ErrInvalidMemoryContract)
	}
	scope, err := ResolveScope(ctx)
	if err != nil {
		return nil, err
	}
	receipt, err := s.repo.GetCommandReceipt(ctx, scope, operationID)
	if err != nil || receipt == nil {
		return nil, err
	}
	return receipt.DTO(), nil
}

func (s *Service) SubmitExpression(
	ctx context.Context, expression *types.PersonalMemoryExpression,
) (*types.PersonalMemoryExpressionReceipt, error) {
	return s.submitExpression(ctx, expression, "", time.Time{})
}

func (s *Service) SubmitExpressionWithModel(
	ctx context.Context, expression *types.PersonalMemoryExpression, chatModelID string,
) (*types.PersonalMemoryExpressionReceipt, error) {
	return s.submitExpression(ctx, expression, chatModelID, time.Time{})
}

func (s *Service) submitExpression(
	ctx context.Context,
	expression *types.PersonalMemoryExpression,
	chatModelID string,
	createdAt time.Time,
) (*types.PersonalMemoryExpressionReceipt, error) {
	if expression == nil || expression.Schema != types.PersonalMemoryExpressionSchema ||
		expression.ExpectedPolicy == nil || strings.TrimSpace(expression.ExpressionID) == "" ||
		len(expression.ExpressionID) > 128 ||
		(expression.Runtime != types.MemoryConsumerEmployee && expression.Runtime != types.MemoryConsumerAnalysis) ||
		strings.TrimSpace(expression.SessionID) == "" || len(expression.SessionID) > 128 ||
		strings.TrimSpace(expression.MessageID) == "" || len(expression.MessageID) > 128 ||
		expression.ExpectedPolicy.WorkspaceGeneration < 0 || expression.ExpectedPolicy.SubjectGeneration < 0 ||
		strings.TrimSpace(expression.Text) == "" || utf8.RuneCountInString(expression.Text) > 32000 {
		return nil, fmt.Errorf("%w: invalid expression", ErrInvalidMemoryContract)
	}
	hash, err := canonicalHash(expression)
	if err != nil {
		return nil, err
	}
	scope, err := ResolveScope(ctx)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(expression.Text)
	stored := &types.MemoryExpression{
		ExpressionID: expression.ExpressionID, ExpressionHash: hash,
		Runtime: expression.Runtime, SessionID: expression.SessionID, MessageID: expression.MessageID,
		Text: &text, WorkspaceGeneration: expression.ExpectedPolicy.WorkspaceGeneration,
		SubjectGeneration: expression.ExpectedPolicy.SubjectGeneration,
		ChatModelID:       chatModelID,
		CreatedAt:         createdAt,
	}
	receipt, err := s.repo.AcceptExpression(ctx, scope, stored)
	if err != nil || receipt == nil || !receipt.Pending {
		return receipt, err
	}
	s.scheduleAcceptedExpression(ctx, scope, stored)
	return receipt, nil
}

func (s *Service) scheduleAcceptedExpression(
	ctx context.Context, scope interfaces.MemoryScope, expression *types.MemoryExpression,
) {
	if s.enqueuer == nil {
		logger.Warnf(ctx, "memory: expression accepted without task enqueuer")
		return
	}
	cfg := s.workspaceConfig(ctx, scope.TenantID)
	previous, shouldEnqueue, err := s.repo.EnqueuePendingSession(
		ctx, scope, expression.SessionID, cfg.ExtractDelay()+extractInFlightGrace,
	)
	if err != nil || !shouldEnqueue {
		if err != nil {
			logger.Warnf(ctx, "memory: schedule accepted expression failed: %v", err)
		}
		return
	}
	delay := cfg.ExtractDelay()
	if previous != nil && previous.LastExtractedAt != nil {
		if remaining := cfg.ExtractMinInterval() - time.Since(*previous.LastExtractedAt); remaining > delay {
			delay = remaining
		}
	}
	s.enqueueExtractionWithPolicy(ctx, scope, expression.SessionID, expression.MessageID, expression.ChatModelID, delay,
		expression.WorkspaceGeneration, expression.SubjectGeneration)
}
