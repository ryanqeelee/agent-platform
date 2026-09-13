package types

import (
	"time"
)

const (
	OperatingBriefScopeAll   = "all_authorized"
	OperatingBriefScopeStore = "store"

	OperatingBriefRefreshQueued    = "queued"
	OperatingBriefRefreshRunning   = "running"
	OperatingBriefRefreshSucceeded = "succeeded"
	OperatingBriefRefreshFailed    = "failed"
)

// OperatingBriefRefresh is the durable association for one native background
// refresh. Browser scope references never enter this record or its task payload.
type OperatingBriefRefresh struct {
	ID              string    `gorm:"column:id;not null;uniqueIndex"`
	TenantID        uint64    `gorm:"column:tenant_id;primaryKey;not null"`
	RequesterUserID string    `gorm:"column:requester_user_id;not null"`
	ScopeKind       string    `gorm:"column:scope_kind;primaryKey;not null"`
	StoreID         string    `gorm:"column:store_id;primaryKey;not null;default:''"`
	Status          string    `gorm:"column:status;not null"`
	ErrorCode       string    `gorm:"column:error_code"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (OperatingBriefRefresh) TableName() string { return "operating_brief_refreshes" }

// OperatingBriefSnapshot is the Platform-owned current projection for one
// tenant scope. Data and handoff questions remain readable without Edge or
// Python online; replacement invalidates the previous public references.
type OperatingBriefSnapshot struct {
	SnapshotRef        string    `gorm:"column:snapshot_ref;not null;uniqueIndex"`
	TenantID           uint64    `gorm:"column:tenant_id;primaryKey;not null"`
	ScopeKind          string    `gorm:"column:scope_kind;primaryKey;not null"`
	StoreID            string    `gorm:"column:store_id;primaryKey;not null;default:''"`
	SourceID           string    `gorm:"column:source_id;not null"`
	BindingID          string    `gorm:"column:binding_id;not null"`
	BindingRevision    int64     `gorm:"column:binding_revision;not null"`
	DeploymentRevision int64     `gorm:"column:deployment_revision;not null"`
	CatalogVersion     string    `gorm:"column:catalog_version;not null"`
	FreshnessToken     string    `gorm:"column:freshness_token;not null"`
	State              string    `gorm:"column:state;not null"`
	Quality            string    `gorm:"column:quality"`
	ReasonCode         string    `gorm:"column:reason_code"`
	InputSetDigest     string    `gorm:"column:input_set_digest"`
	Revision           *int64    `gorm:"column:revision"`
	ReadyAt            string    `gorm:"column:ready_at"`
	Data               JSON      `gorm:"column:data_json;type:jsonb;not null"`
	HandoffQuestions   JSON      `gorm:"column:handoff_questions_json;type:jsonb;not null"`
	SnapshotMetadata   JSON      `gorm:"column:snapshot_metadata_json;type:jsonb;not null"`
	FixedSlots         JSON      `gorm:"column:fixed_slots_json;type:jsonb;not null"`
	GeneratedAt        time.Time `gorm:"column:generated_at;not null"`
}

func (OperatingBriefSnapshot) TableName() string { return "operating_brief_snapshots" }

// OperatingBriefScopeRef is the opaque selector for one current roster store.
// StoreID remains server-side and the caller's live tenant/member permission is
// checked separately on every read.
type OperatingBriefScopeRef struct {
	ScopeRef  string    `gorm:"column:scope_ref;primaryKey"`
	TenantID  uint64    `gorm:"column:tenant_id;not null;index"`
	StoreID   string    `gorm:"column:store_id;not null"`
	Label     string    `gorm:"column:label;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (OperatingBriefScopeRef) TableName() string { return "operating_brief_scope_refs" }

type OperatingBriefScope struct {
	Kind    string `json:"kind"`
	StoreID string `json:"storeId,omitempty"`
}

type OperatingBriefRefreshPayload struct {
	TenantID        uint64              `json:"tenantID"`
	RequesterUserID string              `json:"requesterUserID"`
	Scope           OperatingBriefScope `json:"scope"`
	RefreshID       string              `json:"refreshID"`
}
