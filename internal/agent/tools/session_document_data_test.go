package tools

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/hibiken/asynq"

	"github.com/Tencent/WeKnora/internal/types"
)

type sessionDocumentServiceStub struct {
	document  *types.TemporaryDocument
	data      []byte
	getErr    error
	openErr   error
	getCalls  []sessionDocumentScope
	openCalls []sessionDocumentScope
}

type sessionDocumentScope struct {
	tenantID   uint64
	sessionID  string
	documentID string
}

func (s *sessionDocumentServiceStub) Get(
	_ context.Context, tenantID uint64, sessionID, documentID string,
) (*types.TemporaryDocument, error) {
	s.getCalls = append(s.getCalls, sessionDocumentScope{tenantID, sessionID, documentID})
	return s.document, s.getErr
}

func (s *sessionDocumentServiceStub) OpenFile(
	_ context.Context, tenantID uint64, sessionID, documentID string,
) (io.ReadCloser, string, error) {
	s.openCalls = append(s.openCalls, sessionDocumentScope{tenantID, sessionID, documentID})
	if s.openErr != nil {
		return nil, "", s.openErr
	}
	return io.NopCloser(bytes.NewReader(s.data)), "sales.csv", nil
}

func (*sessionDocumentServiceStub) Create(
	context.Context, uint64, string, string, string, int64, io.Reader, types.TemporaryDocumentCreateOptions,
) (*types.TemporaryDocument, error) {
	return nil, errors.New("not implemented")
}
func (*sessionDocumentServiceStub) List(context.Context, uint64, string) ([]*types.TemporaryDocument, error) {
	return nil, errors.New("not implemented")
}
func (*sessionDocumentServiceStub) Delete(context.Context, uint64, string, string) error {
	return errors.New("not implemented")
}
func (*sessionDocumentServiceStub) ResolveForPrompt(
	context.Context, uint64, string, []string, string,
) (*types.TemporaryDocumentPromptResult, error) {
	return nil, errors.New("not implemented")
}
func (*sessionDocumentServiceStub) Process(context.Context, *asynq.Task) error {
	return errors.New("not implemented")
}
func (*sessionDocumentServiceStub) CleanupExpired(context.Context) error {
	return errors.New("not implemented")
}

func newSessionDocumentCSVTool(t *testing.T, sessionID string, tenantID uint64, service *sessionDocumentServiceStub) *DataAnalysisTool {
	t.Helper()
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	return (&DataAnalysisTool{BaseTool: dataAnalysisTool, db: db, sessionID: sessionID}).
		WithSessionDocuments(service, tenantID, types.MessageAttachments{{ID: " doc-1 "}})
}

func readySessionCSV(tenantID uint64, sessionID string) *types.TemporaryDocument {
	return &types.TemporaryDocument{
		ID:        "doc-1",
		TenantID:  tenantID,
		SessionID: sessionID,
		FileName:  "sales.csv",
		FileType:  ".CSV",
		Status:    types.TemporaryDocumentStatusReady,
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestDataAnalysisSessionDocumentExecutesRealCSVQuery(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	service := &sessionDocumentServiceStub{
		document: readySessionCSV(42, "session-1"),
		data:     []byte("item,amount\napple,12.50\npear,6.25\n"),
	}
	tool := newSessionDocumentCSVTool(t, "session-1", 42, service)

	if !tool.IsSessionDocument("doc-1") {
		t.Fatal("captured temporary document must be recognized")
	}
	if tool.IsSessionDocument("doc-2") {
		t.Fatal("uncaptured temporary document must not be recognized")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"knowledge_id":"doc-1",
		"sql":"SELECT SUM(CAST(amount AS DOUBLE)) AS total, COUNT(*) AS row_count FROM doc-1"
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute() result = %+v", result)
	}
	rows, ok := result.Data["rows"].([]map[string]string)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows = %#v, want one aggregate row", result.Data["rows"])
	}
	if rows[0]["total"] != "18.75" || rows[0]["row_count"] != "2" {
		t.Fatalf("aggregate row = %#v, want total=18.75 row_count=2", rows[0])
	}
	wantScope := sessionDocumentScope{tenantID: 42, sessionID: "session-1", documentID: "doc-1"}
	if len(service.getCalls) != 1 || service.getCalls[0] != wantScope {
		t.Fatalf("Get scopes = %#v, want %#v", service.getCalls, wantScope)
	}
	if len(service.openCalls) != 1 || service.openCalls[0] != wantScope {
		t.Fatalf("OpenFile scopes = %#v, want %#v", service.openCalls, wantScope)
	}
}

func TestDataAnalysisSessionDocumentRejectsInvalidLiveState(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  uint64
		sessionID string
		document  *types.TemporaryDocument
		getErr    error
		wantError string
	}{
		{
			name: "disappeared", tenantID: 42, sessionID: "session-1",
			wantError: "not found",
		},
		{
			name: "service error", tenantID: 42, sessionID: "session-1",
			getErr: errors.New("store unavailable"), wantError: "store unavailable",
		},
		{
			name: "expired", tenantID: 42, sessionID: "session-1",
			document: func() *types.TemporaryDocument {
				doc := readySessionCSV(42, "session-1")
				doc.ExpiresAt = time.Now().Add(-time.Minute)
				return doc
			}(),
			wantError: "expired",
		},
		{
			name: "wrong tenant returned", tenantID: 42, sessionID: "session-1",
			document: readySessionCSV(99, "session-1"), wantError: "scope mismatch",
		},
		{
			name: "wrong session returned", tenantID: 42, sessionID: "session-1",
			document: readySessionCSV(42, "session-2"), wantError: "scope mismatch",
		},
		{
			name: "wrong id returned", tenantID: 42, sessionID: "session-1",
			document: func() *types.TemporaryDocument {
				doc := readySessionCSV(42, "session-1")
				doc.ID = "doc-2"
				return doc
			}(),
			wantError: "scope mismatch",
		},
		{
			name: "non table", tenantID: 42, sessionID: "session-1",
			document: func() *types.TemporaryDocument {
				doc := readySessionCSV(42, "session-1")
				doc.FileType = ".pdf"
				return doc
			}(),
			wantError: "unsupported session document type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &sessionDocumentServiceStub{document: test.document, getErr: test.getErr}
			tool := newSessionDocumentCSVTool(t, test.sessionID, test.tenantID, service)
			_, err := tool.LoadFromKnowledgeID(context.Background(), "doc-1")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("LoadFromKnowledgeID() error = %v, want substring %q", err, test.wantError)
			}
			if len(service.openCalls) != 0 {
				t.Fatalf("OpenFile called before live document validation: %#v", service.openCalls)
			}
		})
	}
}

func TestDataAnalysisCapturedDocumentCannotCrossSessionOrTenant(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  uint64
		sessionID string
	}{
		{name: "other session", tenantID: 42, sessionID: "session-2"},
		{name: "other tenant", tenantID: 99, sessionID: "session-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The same ID is captured in each tool. The scoped service returns no
			// document because the actual owner is tenant 42 / session-1.
			service := &sessionDocumentServiceStub{}
			tool := newSessionDocumentCSVTool(t, test.sessionID, test.tenantID, service)
			_, err := tool.LoadFromKnowledgeID(context.Background(), "doc-1")
			if err == nil || !strings.Contains(err.Error(), "not found") {
				t.Fatalf("cross-scope LoadFromKnowledgeID() error = %v, want not found", err)
			}
			want := sessionDocumentScope{tenantID: test.tenantID, sessionID: test.sessionID, documentID: "doc-1"}
			if len(service.getCalls) != 1 || service.getCalls[0] != want {
				t.Fatalf("Get scopes = %#v, want %#v", service.getCalls, want)
			}
			if len(service.openCalls) != 0 {
				t.Fatalf("cross-scope OpenFile calls = %#v, want none", service.openCalls)
			}
		})
	}
}

func TestDataSchemaSessionDocumentUsesActualTable(t *testing.T) {
	service := &sessionDocumentServiceStub{
		document: readySessionCSV(42, "session-1"),
		data:     []byte("item,amount\napple,12.50\npear,6.25\n"),
	}
	loader := newSessionDocumentCSVTool(t, "session-1", 42, service)
	tool := NewDataSchemaTool(nil, nil).WithSessionDocuments(loader)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"knowledge_id":"doc-1"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("Execute() result = %+v", result)
	}
	if result.Data["row_count"] != int64(2) {
		t.Fatalf("row_count = %#v, want 2", result.Data["row_count"])
	}
	if !strings.Contains(result.Output, "- item (VARCHAR)") || !strings.Contains(result.Output, "- amount (VARCHAR)") {
		t.Fatalf("schema output does not describe actual CSV columns:\n%s", result.Output)
	}
}
