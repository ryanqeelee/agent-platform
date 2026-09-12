package tools

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// Edge currently returns HTTP status and detail, not a structured reason code.
// Recognize only known validation diagnostics; never treat every 422 as SQL.
type governedRequestError struct {
	status int
	detail string
	code   string
}

func newGovernedRequestError(status int, detail string) error {
	code := "request_failed"
	switch {
	case status == 401 || status == 403:
		code = "access_denied"
	case status == 422 && (detail == "Catalog version changed; discover the current Catalog" || detail == "data freshness changed; discover the current Catalog"):
		code = "catalog_changed"
	case status == 422 && strings.Contains(detail, "DB::Exception:") && (strings.Contains(detail, "(UNKNOWN_IDENTIFIER)") || strings.Contains(detail, "(SYNTAX_ERROR)")):
		code = "invalid_sql"
	case status >= 500 && status <= 599:
		code = "service_unavailable"
	}
	return &governedRequestError{status: status, detail: detail, code: code}
}

func (e *governedRequestError) Error() string {
	hint := ""
	switch e.code {
	case "catalog_changed":
		hint = " Call governed_data_schema without a table, then read the relevant current table definitions and revalidate the SQL before querying. Do not retry unchanged with the old catalog."
	case "invalid_sql":
		hint = " Check the current table columns and SQL alias scope, correct the SQL, then query again. Do not substitute a suggested column without checking its metric meaning."
	case "access_denied":
		hint = " Do not retry or switch identity to bypass this denial."
	}
	return fmt.Sprintf("Edge request failed (HTTP %d): %s%s", e.status, e.detail, hint)
}

// Only transport-owned errors can create the machine code used by presentation.
func GovernedFailureData(err error) map[string]interface{} {
	var failure *governedRequestError
	if errors.As(err, &failure) {
		return map[string]interface{}{"governed_error_code": failure.code}
	}
	return nil
}

// ToolFailureOperationalStatus projects allowlisted diagnoses, never raw errors.
// The same projection serves live SSE, stored steps and history replay.
func ToolFailureOperationalStatus(toolName string, data map[string]interface{}) types.RoleBoundedOperationalStatus {
	if toolName != ToolGovernedDataQuery && toolName != ToolGovernedDataSchema {
		return ExternalToolOperationalStatus()
	}
	code, _ := data["governed_error_code"].(string)
	statusCode := types.OperationalStatusGovernedRequestFailed
	switch code {
	case "catalog_changed":
		statusCode = types.OperationalStatusGovernedCatalogChanged
	case "invalid_sql":
		statusCode = types.OperationalStatusGovernedQueryInvalid
	case "access_denied":
		statusCode = types.OperationalStatusAccessUnavailable
	case "service_unavailable":
		statusCode = types.OperationalStatusServiceUnavailable
	}
	return types.ProjectOperationalStatus(statusCode, types.OperationalStatusEmployee, types.OperationalStatusFactsV1{})
}
