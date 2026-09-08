package types

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const (
	PersonalMemorySnapshotSchema          = "personal_memory_snapshot/1"
	PersonalMemoryCommandSchema           = "personal_memory_command/1"
	PersonalMemoryReceiptSchema           = "personal_memory_receipt/1"
	PersonalMemoryExpressionSchema        = "personal_memory_expression/1"
	PersonalMemoryExpressionReceiptSchema = "personal_memory_expression_receipt/1"

	MemoryScopeShared   = "shared"
	MemoryScopeEmployee = "employee"
	MemoryScopeAnalysis = "analysis"

	MemoryConsumerEmployee = "employee"
	MemoryConsumerAnalysis = "analysis"

	MemoryCommandModeExplicit = "explicit"
	MemoryCommandModeManual   = "manual"
	// MemoryCommandModeAuto is internal-only. The public command decoder rejects
	// it; accepted expressions are the only path that may create one.
	MemoryCommandModeAuto = "auto"

	MemoryChangeCreate = "create"
	MemoryChangeUpdate = "update"
	MemoryChangeDelete = "delete"

	MemoryReceiptApplied  = "applied"
	MemoryReceiptNoop     = "noop"
	MemoryReceiptRejected = "rejected"

	MemoryReasonPolicyDisabled   = "policy_disabled"
	MemoryReasonPolicyStale      = "policy_stale"
	MemoryReasonRevisionConflict = "revision_conflict"
	MemoryReasonItemNotFound     = "item_not_found"
	MemoryReasonInvalidSource    = "invalid_source"
	MemoryReasonPreviouslyForgot = "previously_forgotten"
	MemoryReasonSensitiveContent = "sensitive_content"

	MemoryExpressionAccepted = "accepted"
	MemoryExpressionReplayed = "replayed"
	MemoryExpressionRejected = "rejected"

	MemoryExpressionPending   = "pending"
	MemoryExpressionProcessed = "processed"
)

func IsValidMemoryScope(scope string) bool {
	return scope == MemoryScopeShared || scope == MemoryScopeEmployee || scope == MemoryScopeAnalysis
}

func IsValidMemoryConsumer(consumer string) bool {
	return consumer == MemoryConsumerEmployee || consumer == MemoryConsumerAnalysis
}

// MemoryPolicyVersion is the optimistic policy/revision tuple carried on the
// wire. It is evidence of what the caller read, never authority to write.
type MemoryPolicyVersion struct {
	WorkspaceGeneration int64 `json:"workspace_generation"`
	SubjectGeneration   int64 `json:"subject_generation"`
	Revision            int64 `json:"revision"`
}

func (v *MemoryPolicyVersion) UnmarshalJSON(data []byte) error {
	var wire struct {
		WorkspaceGeneration *int64 `json:"workspace_generation"`
		SubjectGeneration   *int64 `json:"subject_generation"`
		Revision            *int64 `json:"revision"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.WorkspaceGeneration == nil || wire.SubjectGeneration == nil || wire.Revision == nil {
		return fmt.Errorf("workspace_generation, subject_generation, and revision are required")
	}
	v.WorkspaceGeneration = *wire.WorkspaceGeneration
	v.SubjectGeneration = *wire.SubjectGeneration
	v.Revision = *wire.Revision
	return nil
}

type PersonalMemoryPolicy struct {
	WorkspaceEnabled    bool   `json:"workspace_enabled"`
	UserEnabled         bool   `json:"user_enabled"`
	WriteMode           string `json:"write_mode"`
	WorkspaceGeneration int64  `json:"workspace_generation"`
	SubjectGeneration   int64  `json:"subject_generation"`
}

type PersonalMemorySnapshotItem struct {
	ID         string `json:"id"`
	Scope      string `json:"scope"`
	Kind       string `json:"kind"`
	Topic      string `json:"topic"`
	Content    string `json:"content"`
	Importance int    `json:"importance"`
	Origin     string `json:"origin"`
	Status     string `json:"status"`
}

type PersonalMemorySnapshot struct {
	Schema   string                        `json:"schema"`
	Status   string                        `json:"status"`
	Consumer string                        `json:"consumer"`
	Policy   PersonalMemoryPolicy          `json:"policy"`
	Revision int64                         `json:"revision"`
	Items    []*PersonalMemorySnapshotItem `json:"items"`
}

type PersonalMemoryCommandSource struct {
	Runtime   string `json:"runtime"`
	Mode      string `json:"mode"`
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
}

// PersonalMemoryChange is also used after expression extraction. The fields
// below InternalOrigin are deliberately not serialized and preserve existing
// inferred/pending and expiry behavior without widening the public contract.
type PersonalMemoryChange struct {
	Op         string `json:"op"`
	ID         string `json:"id,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Topic      string `json:"topic,omitempty"`
	Content    string `json:"content,omitempty"`
	Importance *int   `json:"importance,omitempty"`

	InternalOrigin    string     `json:"-"`
	InternalInferred  bool       `json:"-"`
	InternalExpiresAt *time.Time `json:"-"`
}

type PersonalMemoryCommand struct {
	Schema      string                      `json:"schema"`
	OperationID string                      `json:"operation_id"`
	Source      PersonalMemoryCommandSource `json:"source"`
	Expected    *MemoryPolicyVersion        `json:"expected"`
	Changes     []PersonalMemoryChange      `json:"changes"`

	InternalExpressionID string `json:"-"`
}

type PersonalMemoryReceipt struct {
	Schema              string     `json:"schema"`
	OperationID         string     `json:"operation_id"`
	Status              string     `json:"status"`
	ReasonCode          *string    `json:"reason_code"`
	Revision            int64      `json:"revision"`
	WorkspaceGeneration int64      `json:"workspace_generation"`
	SubjectGeneration   int64      `json:"subject_generation"`
	ItemIDs             []string   `json:"item_ids"`
	CommittedAt         *time.Time `json:"committed_at"`
}

type PersonalMemoryExpressionExpectedPolicy struct {
	WorkspaceGeneration int64 `json:"workspace_generation"`
	SubjectGeneration   int64 `json:"subject_generation"`
}

func (p *PersonalMemoryExpressionExpectedPolicy) UnmarshalJSON(data []byte) error {
	var wire struct {
		WorkspaceGeneration *int64 `json:"workspace_generation"`
		SubjectGeneration   *int64 `json:"subject_generation"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.WorkspaceGeneration == nil || wire.SubjectGeneration == nil {
		return fmt.Errorf("workspace_generation and subject_generation are required")
	}
	p.WorkspaceGeneration = *wire.WorkspaceGeneration
	p.SubjectGeneration = *wire.SubjectGeneration
	return nil
}

type PersonalMemoryExpression struct {
	Schema         string                                  `json:"schema"`
	ExpressionID   string                                  `json:"expression_id"`
	Runtime        string                                  `json:"runtime"`
	SessionID      string                                  `json:"session_id"`
	MessageID      string                                  `json:"message_id"`
	Text           string                                  `json:"text"`
	ExpectedPolicy *PersonalMemoryExpressionExpectedPolicy `json:"expected_policy"`
}

type PersonalMemoryExpressionReceipt struct {
	Schema       string  `json:"schema"`
	ExpressionID string  `json:"expression_id"`
	Status       string  `json:"status"`
	ReasonCode   *string `json:"reason_code"`
	Pending      bool    `json:"-"`
}

// MemoryStringList is used by content-free command receipts.
type MemoryStringList []string

func (v MemoryStringList) Value() (driver.Value, error) {
	if v == nil {
		v = MemoryStringList{}
	}
	return json.Marshal(v)
}

func (v *MemoryStringList) Scan(value interface{}) error {
	if value == nil {
		*v = MemoryStringList{}
		return nil
	}
	var data []byte
	switch raw := value.(type) {
	case []byte:
		data = raw
	case string:
		data = []byte(raw)
	default:
		*v = MemoryStringList{}
		return nil
	}
	return json.Unmarshal(data, v)
}

// MemoryCommandReceipt stores only a canonical command digest and result; it
// intentionally never retains memory bodies.
type MemoryCommandReceipt struct {
	ID                  string           `gorm:"primaryKey;type:varchar(36)"`
	TenantID            uint64           `gorm:"column:tenant_id;not null;uniqueIndex:idx_memory_command_receipts_scope,priority:1"`
	SubjectID           string           `gorm:"column:subject_id;type:varchar(512);not null;uniqueIndex:idx_memory_command_receipts_scope,priority:2"`
	OperationID         string           `gorm:"column:operation_id;type:varchar(128);not null;uniqueIndex:idx_memory_command_receipts_scope,priority:3"`
	CommandHash         string           `gorm:"column:command_hash;type:varchar(64);not null"`
	Status              string           `gorm:"type:varchar(16);not null"`
	ReasonCode          *string          `gorm:"column:reason_code;type:varchar(32)"`
	Revision            int64            `gorm:"not null"`
	WorkspaceGeneration int64            `gorm:"column:workspace_generation;not null"`
	SubjectGeneration   int64            `gorm:"column:subject_generation;not null"`
	ItemIDs             MemoryStringList `gorm:"column:item_ids;type:jsonb;not null"`
	CommittedAt         *time.Time       `gorm:"column:committed_at"`
	CreatedAt           time.Time
}

func (MemoryCommandReceipt) TableName() string { return "memory_command_receipts" }

func (r *MemoryCommandReceipt) DTO() *PersonalMemoryReceipt {
	if r == nil {
		return nil
	}
	ids := []string(r.ItemIDs)
	if ids == nil {
		ids = []string{}
	}
	return &PersonalMemoryReceipt{
		Schema: PersonalMemoryReceiptSchema, OperationID: r.OperationID,
		Status: r.Status, ReasonCode: r.ReasonCode, Revision: r.Revision,
		WorkspaceGeneration: r.WorkspaceGeneration, SubjectGeneration: r.SubjectGeneration,
		ItemIDs: ids, CommittedAt: r.CommittedAt,
	}
}

// MemoryExpression keeps source text only while extraction is pending. A
// processed row retains the digest and outcome so retries are stable without
// retaining the original statement indefinitely.
type MemoryExpression struct {
	ID                  string  `gorm:"primaryKey;type:varchar(36)"`
	TenantID            uint64  `gorm:"column:tenant_id;not null;uniqueIndex:idx_memory_expressions_scope,priority:1"`
	SubjectID           string  `gorm:"column:subject_id;type:varchar(512);not null;uniqueIndex:idx_memory_expressions_scope,priority:2"`
	ExpressionID        string  `gorm:"column:expression_id;type:varchar(128);not null;uniqueIndex:idx_memory_expressions_scope,priority:3"`
	ExpressionHash      string  `gorm:"column:expression_hash;type:varchar(64);not null"`
	Runtime             string  `gorm:"type:varchar(16);not null"`
	SessionID           string  `gorm:"column:session_id;type:varchar(128);not null"`
	MessageID           string  `gorm:"column:message_id;type:varchar(128);not null"`
	ChatModelID         string  `gorm:"column:chat_model_id;type:varchar(64)"`
	Text                *string `gorm:"type:text"`
	Status              string  `gorm:"type:varchar(16);not null"`
	OutcomeReason       *string `gorm:"column:outcome_reason;type:varchar(64)"`
	WorkspaceGeneration int64   `gorm:"column:workspace_generation;not null"`
	SubjectGeneration   int64   `gorm:"column:subject_generation;not null"`
	CreatedAt           time.Time
	ProcessedAt         *time.Time `gorm:"column:processed_at"`
}

func (MemoryExpression) TableName() string { return "memory_expressions" }

const memoryPolicyVersionContextKey ContextKey = "MemoryPolicyVersion"

func WithMemoryPolicyVersion(ctx context.Context, version MemoryPolicyVersion) context.Context {
	return context.WithValue(ctx, memoryPolicyVersionContextKey, version)
}

func MemoryPolicyVersionFromContext(ctx context.Context) (*MemoryPolicyVersion, bool) {
	if ctx == nil {
		return nil, false
	}
	version, ok := ctx.Value(memoryPolicyVersionContextKey).(MemoryPolicyVersion)
	if !ok {
		return nil, false
	}
	return &version, true
}
