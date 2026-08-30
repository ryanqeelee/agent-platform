package types

const EnterpriseAdministrationQueueV1 = "EnterpriseAdministrationQueueV1"

type EnterpriseAdministrationFacts struct {
	Role                                string `json:"role"`
	ActiveMemberCount                   int    `json:"active_member_count"`
	OperatingAnalysisMissingAccessCount int    `json:"operating_analysis_missing_access_count"`
}

type EnterpriseAdministrationItem struct {
	Code     string `json:"code"`
	Priority string `json:"priority"`
	Count    int    `json:"count"`
	Target   string `json:"target"`
}

type EnterpriseAdministrationPlatformSummary struct {
	ServiceLevel string `json:"service_level"`
	Status       string `json:"status"`
	MemberQuota  *int   `json:"member_quota"`
	Health       string `json:"health"`
}

type EnterpriseAdministrationPlatformProjection struct {
	ContractVersion string                                  `json:"contract_version"`
	Scope           PlatformSettingsScope                   `json:"scope"`
	AsOf            string                                  `json:"as_of"`
	Summary         EnterpriseAdministrationPlatformSummary `json:"summary"`
	Items           []EnterpriseAdministrationItem          `json:"items"`
}

type EnterpriseAdministrationSummary struct {
	EnterpriseAdministrationPlatformSummary
	MemberUsage      int   `json:"member_usage"`
	StorageUsageByte int64 `json:"storage_usage_bytes"`
	StorageQuotaByte int64 `json:"storage_quota_bytes"`
}

type EnterpriseAdministrationQueue struct {
	ContractVersion string                          `json:"contract_version"`
	AsOf            string                          `json:"as_of"`
	Summary         EnterpriseAdministrationSummary `json:"summary"`
	Items           []EnterpriseAdministrationItem  `json:"items"`
}
