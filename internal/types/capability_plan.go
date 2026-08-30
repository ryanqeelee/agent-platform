package types

type EnterpriseAICapabilityProjection struct {
	ServiceLevel string             `json:"service_level"`
	Status       string             `json:"status"`
	Usage        *float64           `json:"usage"`
	Quota        *float64           `json:"quota"`
	Health       EnterpriseAIHealth `json:"health"`
}

type EnterpriseAIHealth struct {
	Status string `json:"status"`
}

type AICapabilityPlanResolution struct {
	ContractVersion string                           `json:"contract_version"`
	PlanVersionID   string                           `json:"plan_version_id"`
	Source          string                           `json:"source"`
	Enterprise      EnterpriseAICapabilityProjection `json:"enterprise"`
}

type KnowledgeProcessingPlanPin struct {
	ContractVersion string `json:"contract_version"`
	PlanVersionID   string `json:"plan_version_id"`
}

type PlatformSettingsScope struct {
	Kind                string `json:"kind"`
	ProductBaseTenantID string `json:"product_base_tenant_id"`
}

type PlatformActivePlan struct {
	ContractVersion string `json:"contract_version"`
	VersionID       string `json:"version_id"`
}

type PlatformModelRuntimeSettings struct {
	ContractVersion    string                `json:"contract_version"`
	Scope              PlatformSettingsScope `json:"scope"`
	ActivePlan         PlatformActivePlan    `json:"active_plan"`
	RequestRuntimeRefs struct {
		EmployeeAssistantRequestRuntime string `json:"employee_assistant_request_runtime"`
		OperatingAnalysisRequestRuntime string `json:"operating_analysis_request_runtime"`
	} `json:"request_runtime_refs"`
}

type PlatformRetrievalProcessingSettings struct {
	ContractVersion string                `json:"contract_version"`
	Scope           PlatformSettingsScope `json:"scope"`
	ActivePlan      PlatformActivePlan    `json:"active_plan"`
	CapabilityRefs  struct {
		Embedding string `json:"embedding"`
		Reranking string `json:"reranking"`
		Parsing   string `json:"parsing"`
	} `json:"capability_refs"`
}
