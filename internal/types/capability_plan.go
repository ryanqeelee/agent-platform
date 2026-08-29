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
