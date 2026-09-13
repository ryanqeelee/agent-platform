package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
)

type legacyXLSKnowledgeService struct {
	interfaces.KnowledgeService
	getCalls int
}

func (s *legacyXLSKnowledgeService) GetKnowledgeByID(_ context.Context, id string) (*types.Knowledge, error) {
	s.getCalls++
	return &types.Knowledge{ID: id, FileType: "xls"}, nil
}

func TestDataTableSummaryHandleRejectsLegacyXLSBeforeOtherResources(t *testing.T) {
	knowledgeService := &legacyXLSKnowledgeService{}
	service := &DataTableSummaryService{knowledgeService: knowledgeService}
	payload, err := json.Marshal(DataTableSummaryPayload{TenantID: 42, KnowledgeID: "legacy-xls"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	err = service.Handle(context.Background(), asynq.NewTask("table-summary", payload))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("Handle() error = %v, want asynq.SkipRetry", err)
	}
	for _, want := range []string{"unsupported analytical file format", "convert the file to .xlsx"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Handle() error = %q, want substring %q", err, want)
		}
	}
	if knowledgeService.getCalls != 1 {
		t.Fatalf("GetKnowledgeByID calls = %d, want 1", knowledgeService.getCalls)
	}
}

func TestTableSummaryPreparationErrorStopsRetryOnlyForUnsupportedAnalyticalFormat(t *testing.T) {
	unsupported := fmt.Errorf(
		"%w: legacy .xls; convert the file to .xlsx",
		errUnsupportedAnalyticalFileType,
	)
	got := tableSummaryPreparationError(unsupported)
	if !errors.Is(got, asynq.SkipRetry) {
		t.Fatalf("tableSummaryPreparationError() = %v, want asynq.SkipRetry", got)
	}
	if !strings.Contains(got.Error(), "convert the file to .xlsx") {
		t.Fatalf("tableSummaryPreparationError() hid actionable reason: %v", got)
	}

	transient := errors.New("temporary model lookup failure")
	got = tableSummaryPreparationError(transient)
	if got != transient {
		t.Fatalf("tableSummaryPreparationError() changed retryable error: %v", got)
	}
	if errors.Is(got, asynq.SkipRetry) {
		t.Fatalf("tableSummaryPreparationError() suppressed retryable error: %v", got)
	}
}

func TestBuildSampleDataDescriptionIncludesDataAnalysisRows(t *testing.T) {
	service := &DataTableSummaryService{}
	result := &types.ToolResult{Data: map[string]interface{}{
		"rows": []map[string]string{
			{"date": "20250101", "status": "approved"},
			{"date": "20250102", "status": "pending"},
		},
	}}

	got := service.buildSampleDataDescription(context.Background(), result, 10)
	for _, want := range []string{
		`"date":"20250101"`,
		`"status":"approved"`,
		`"date":"20250102"`,
		`"status":"pending"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sample description missing %s:\n%s", want, got)
		}
	}
}

func TestBuildSampleDataDescriptionSupportsDecodedRows(t *testing.T) {
	service := &DataTableSummaryService{}
	result := &types.ToolResult{Data: map[string]interface{}{
		"rows": []map[string]interface{}{
			{"date": "20250101", "count": float64(3)},
		},
	}}

	got := service.buildSampleDataDescription(context.Background(), result, 10)
	for _, want := range []string{`"date":"20250101"`, `"count":3`} {
		if !strings.Contains(got, want) {
			t.Errorf("sample description missing %s:\n%s", want, got)
		}
	}
}

func TestBuildSampleDataDescriptionLimitsRows(t *testing.T) {
	service := &DataTableSummaryService{}
	result := &types.ToolResult{Data: map[string]interface{}{
		"rows": []map[string]string{
			{"id": "first"},
			{"id": "second"},
		},
	}}

	got := service.buildSampleDataDescription(context.Background(), result, 1)
	if !strings.Contains(got, `"id":"first"`) {
		t.Fatalf("first row missing:\n%s", got)
	}
	if strings.Contains(got, `"id":"second"`) {
		t.Fatalf("sample limit was ignored:\n%s", got)
	}
}
