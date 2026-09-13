package types

import "time"

const (
	AICapabilityPlanContractVersion            = "AICapabilityPlanV1"
	KnowledgeProcessingPlanContractVersion     = "KnowledgeProcessingPlanPinV1"
	PlatformModelRuntimeContractVersion        = "PlatformModelRuntimeSettingsV1"
	PlatformRetrievalContractVersion           = "PlatformRetrievalProcessingSettingsV1"
	AssistantScenarioCapabilityContractVersion = "AssistantScenarioCapabilityV1"
)

// AICapabilityPlanRefs keeps capability references opaque. Their meaning and
// lifecycle belong to the systems that consume them.
type AICapabilityPlanRefs struct {
	EmployeeAssistantRequestRuntime string `json:"employee_assistant_request_runtime"`
	OperatingAnalysisRequestRuntime string `json:"operating_analysis_request_runtime"`
	Embedding                       string `json:"embedding"`
	Reranking                       string `json:"reranking"`
	Parsing                         string `json:"parsing"`
}

// AICapabilityPlanVersion is an immutable version in the local capability registry.
type AICapabilityPlanVersion struct {
	VersionID                          string    `json:"version_id" gorm:"column:version_id;type:text;primaryKey"`
	ContractVersion                    string    `json:"contract_version" gorm:"column:contract_version;type:text;not null"`
	ServiceLevel                       string    `json:"service_level" gorm:"column:service_level;type:text;not null"`
	EmployeeAssistantRequestRuntimeRef string    `json:"-" gorm:"column:employee_assistant_request_runtime_ref;type:text;not null"`
	OperatingAnalysisRequestRuntimeRef string    `json:"-" gorm:"column:operating_analysis_request_runtime_ref;type:text;not null"`
	EmbeddingRef                       string    `json:"-" gorm:"column:embedding_ref;type:text;not null"`
	RerankingRef                       string    `json:"-" gorm:"column:reranking_ref;type:text;not null"`
	ParsingRef                         string    `json:"-" gorm:"column:parsing_ref;type:text;not null"`
	CreatedBy                          string    `json:"created_by" gorm:"column:created_by;type:text;not null"`
	CreatedAt                          time.Time `json:"created_at" gorm:"column:created_at;not null"`
}

func (AICapabilityPlanVersion) TableName() string { return "ai_capability_plan_versions" }

// CapabilityRefs returns the exact stored references without interpreting them.
func (p AICapabilityPlanVersion) CapabilityRefs() AICapabilityPlanRefs {
	return AICapabilityPlanRefs{
		EmployeeAssistantRequestRuntime: p.EmployeeAssistantRequestRuntimeRef,
		OperatingAnalysisRequestRuntime: p.OperatingAnalysisRequestRuntimeRef,
		Embedding:                       p.EmbeddingRef,
		Reranking:                       p.RerankingRef,
		Parsing:                         p.ParsingRef,
	}
}

// AICapabilityPlanDocument is the management projection of an immutable version.
type AICapabilityPlanDocument struct {
	ContractVersion string               `json:"contract_version"`
	VersionID       string               `json:"version_id"`
	ServiceLevel    string               `json:"service_level"`
	CapabilityRefs  AICapabilityPlanRefs `json:"capability_refs"`
	CreatedBy       string               `json:"created_by"`
	CreatedAt       time.Time            `json:"created_at"`
}

// Document converts the persistence row to the frozen management shape.
func (p AICapabilityPlanVersion) Document() AICapabilityPlanDocument {
	return AICapabilityPlanDocument{
		ContractVersion: p.ContractVersion,
		VersionID:       p.VersionID,
		ServiceLevel:    p.ServiceLevel,
		CapabilityRefs:  p.CapabilityRefs(),
		CreatedBy:       p.CreatedBy,
		CreatedAt:       p.CreatedAt,
	}
}

// AICapabilityPlanDefault holds the singleton platform default pointer.
type AICapabilityPlanDefault struct {
	SingletonKey bool      `gorm:"column:singleton_key;primaryKey"`
	VersionID    string    `gorm:"column:version_id;type:text;not null"`
	UpdatedBy    string    `gorm:"column:updated_by;type:text;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (AICapabilityPlanDefault) TableName() string { return "ai_capability_plan_default" }

// TenantAICapabilityPlanAssignment overrides the platform default for one tenant.
type TenantAICapabilityPlanAssignment struct {
	TenantID  uint64    `gorm:"column:tenant_id;primaryKey"`
	VersionID string    `gorm:"column:version_id;type:text;not null"`
	UpdatedBy string    `gorm:"column:updated_by;type:text;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (TenantAICapabilityPlanAssignment) TableName() string {
	return "tenant_ai_capability_plan_assignments"
}

// AssistantScenarioCapabilityDefault stores the singleton scenario defaults.
type AssistantScenarioCapabilityDefault struct {
	SingletonKey   bool      `gorm:"column:singleton_key;primaryKey"`
	ExternalSearch bool      `gorm:"column:external_search;not null"`
	MCP            bool      `gorm:"column:mcp;not null"`
	Tools          bool      `gorm:"column:tools;not null"`
	UpdatedBy      string    `gorm:"column:updated_by;type:text;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (AssistantScenarioCapabilityDefault) TableName() string {
	return "assistant_scenario_capability_default"
}

// TenantAssistantScenarioCapabilityOverride preserves explicit false values.
type TenantAssistantScenarioCapabilityOverride struct {
	TenantID       uint64    `gorm:"column:tenant_id;primaryKey"`
	ExternalSearch bool      `gorm:"column:external_search;not null"`
	MCP            bool      `gorm:"column:mcp;not null"`
	Tools          bool      `gorm:"column:tools;not null"`
	UpdatedBy      string    `gorm:"column:updated_by;type:text;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (TenantAssistantScenarioCapabilityOverride) TableName() string {
	return "tenant_assistant_scenario_capability_overrides"
}

// ResolvedAICapabilityPlan records both the immutable version and its source.
type ResolvedAICapabilityPlan struct {
	Plan   AICapabilityPlanVersion
	Source string
}

// ResolvedAssistantScenarioCapabilities records the effective values and source.
type ResolvedAssistantScenarioCapabilities struct {
	Capabilities AssistantScenarioCapabilities
	Source       string
}
