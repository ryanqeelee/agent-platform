package types

const RoleBoundedOperationalStatusV1 = "RoleBoundedOperationalStatusV1"

type OperationalStatusAudience string

const (
	OperationalStatusEmployee        OperationalStatusAudience = "employee"
	OperationalStatusEnterpriseAdmin OperationalStatusAudience = "enterprise_admin"
	OperationalStatusPlatform        OperationalStatusAudience = "platform"
)

type OperationalStatusCode string

const (
	OperationalStatusOK                          OperationalStatusCode = "ok"
	OperationalStatusCapabilityHidden            OperationalStatusCode = "capability_hidden"
	OperationalStatusAccessUnavailable           OperationalStatusCode = "access_unavailable"
	OperationalStatusServiceUnavailable          OperationalStatusCode = "service_unavailable"
	OperationalStatusKnowledgeProcessingFailed   OperationalStatusCode = "knowledge_processing_failed"
	OperationalStatusModelRuntimeUnavailable     OperationalStatusCode = "model_runtime_unavailable"
	OperationalStatusRetrievalUnavailable        OperationalStatusCode = "retrieval_unavailable"
	OperationalStatusParsingFailed               OperationalStatusCode = "parsing_failed"
	OperationalStatusStorageUnavailable          OperationalStatusCode = "storage_unavailable"
	OperationalStatusExternalToolUnavailable     OperationalStatusCode = "external_tool_unavailable"
	OperationalStatusEnterpriseAttentionRequired OperationalStatusCode = "enterprise_attention_required"
)

type operationalStatusPublicProjection struct {
	State      string
	Summary    string
	NextAction string
}

var operationalStatusPublic = map[OperationalStatusCode]operationalStatusPublicProjection{
	OperationalStatusOK:                          {"available", "Capability is available.", "none"},
	OperationalStatusCapabilityHidden:            {"hidden", "Capability is not available.", "none"},
	OperationalStatusAccessUnavailable:           {"disabled", "This capability is not enabled for your account.", "contact_admin"},
	OperationalStatusServiceUnavailable:          {"disabled", "This capability is currently unavailable.", "service_unavailable"},
	OperationalStatusKnowledgeProcessingFailed:   {"failed", "Knowledge processing did not complete.", "retry"},
	OperationalStatusModelRuntimeUnavailable:     {"unavailable", "Model capability is temporarily unavailable.", "retry"},
	OperationalStatusRetrievalUnavailable:        {"unavailable", "Knowledge retrieval is temporarily unavailable.", "retry"},
	OperationalStatusParsingFailed:               {"failed", "Document processing did not complete.", "retry"},
	OperationalStatusStorageUnavailable:          {"unavailable", "Storage capability is temporarily unavailable.", "retry"},
	OperationalStatusExternalToolUnavailable:     {"unavailable", "The external tool is temporarily unavailable.", "retry"},
	OperationalStatusEnterpriseAttentionRequired: {"attention", "Enterprise service requires attention.", "open_admin_surface"},
}

type SafePartialResultRefV1 struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type EnterpriseOperationalScopeV1 struct {
	Kind                string `json:"kind"`
	ProductBaseTenantID string `json:"productBaseTenantId"`
}

type EnterpriseOperationalProcessingV1 struct {
	Kind      string `json:"kind"`
	Stage     string `json:"stage"`
	Retryable bool   `json:"retryable"`
}

type EnterpriseOperationalContextV1 struct {
	Scope      EnterpriseOperationalScopeV1       `json:"scope"`
	Processing *EnterpriseOperationalProcessingV1 `json:"processing,omitempty"`
}

type PlatformOperationalDiagnosticsV1 struct {
	CorrelationRef string `json:"correlationRef"`
	Domain         string `json:"domain"`
	CauseCode      string `json:"causeCode"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Runtime        string `json:"runtime,omitempty"`
	Subsystem      string `json:"subsystem,omitempty"`
}

type OperationalStatusFactsV1 struct {
	PartialResultRef    *SafePartialResultRefV1
	EnterpriseContext   *EnterpriseOperationalContextV1
	PlatformDiagnostics *PlatformOperationalDiagnosticsV1
}

type RoleBoundedOperationalStatus struct {
	ContractVersion     string                            `json:"contractVersion"`
	State               string                            `json:"state"`
	StatusCode          OperationalStatusCode             `json:"statusCode"`
	SafeSummary         string                            `json:"safeSummary"`
	NextAction          string                            `json:"nextAction"`
	PartialResultRef    *SafePartialResultRefV1           `json:"partialResultRef,omitempty"`
	EnterpriseContext   *EnterpriseOperationalContextV1   `json:"enterpriseContext,omitempty"`
	PlatformDiagnostics *PlatformOperationalDiagnosticsV1 `json:"platformDiagnostics,omitempty"`
}

func ProjectOperationalStatus(
	code OperationalStatusCode,
	audience OperationalStatusAudience,
	facts OperationalStatusFactsV1,
) RoleBoundedOperationalStatus {
	public := operationalStatusPublic[code]
	result := RoleBoundedOperationalStatus{
		ContractVersion:  RoleBoundedOperationalStatusV1,
		State:            public.State,
		StatusCode:       code,
		SafeSummary:      public.Summary,
		NextAction:       public.NextAction,
		PartialResultRef: facts.PartialResultRef,
	}
	if facts.PartialResultRef != nil {
		result.State = "partial"
		result.NextAction = "review_partial_result"
	}
	if audience == OperationalStatusEnterpriseAdmin || audience == OperationalStatusPlatform {
		result.EnterpriseContext = facts.EnterpriseContext
	}
	if audience == OperationalStatusPlatform {
		result.PlatformDiagnostics = facts.PlatformDiagnostics
	}
	return result
}

func OperationalStatusSummary(code OperationalStatusCode) string {
	return operationalStatusPublic[code].Summary
}
