package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestGovernedDataRegistrationUsesCurrentUser(t *testing.T) {
	var calls int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer user-jwt" || r.Header.Get("X-Tenant-ID") != "10001" {
			t.Error("wrong user authority")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/governed-data/runs" {
			_, _ = w.Write([]byte(`{"contractVersion":"governed-analysis-run/1","runId":"run-1","state":"running","catalogVersion":"version-1","catalogDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","scopeDigest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","bindingDigest":"sha256:75ff2b4664e230e2eacc95a6b54ae2d586542c245bdad5ca5267554c7579fd5c"}`))
			return
		}
		_, _ = w.Write([]byte(`{"schema":"GovernedDataSchemaV1","source":{"source_id":"business"},"catalog_version":"version-1","catalog_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tables":[],"analysis_run":{"contractVersion":"governed-analysis-run/1","runId":"run-1","state":"running"}}`))
	}))
	defer endpoint.Close()
	s := &agentService{cfg: &config.Config{Agent: &config.AgentConfig{GovernedData: &config.GovernedDataConfig{BaseURL: endpoint.URL}}}}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-a")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10001))
	bound := types.WithGovernedDataObservability(types.WithGovernedDataUserCredential(ctx, "user-jwt"))
	client, err := tools.NewGovernedDataClient(endpoint.URL, "user-jwt", "10001", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(bound, "session", "turn", "question"); err != nil {
		t.Fatal(err)
	}
	bound = tools.WithGovernedAnalysisClient(bound, client)
	selected := &types.AgentConfig{AllowedTools: []string{tools.ToolGovernedDataSchema, tools.ToolGovernedDataQuery}, EmployeeAssistant: true}
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		cfg        *types.AgentConfig
		rejects    bool
		registered bool
	}{
		{"selected", bound, selected, false, true},
		{"ordinary agent", bound, &types.AgentConfig{}, false, false},
		{"no jwt", ctx, selected, true, false},
		{"shared agent", context.WithValue(bound, types.TenantIDContextKey, uint64(20002)), selected, true, false},
		{"query without schema", bound, &types.AgentConfig{AllowedTools: []string{tools.ToolGovernedDataQuery}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := tools.NewToolRegistry()
			err := s.registerGovernedDataTools(tc.ctx, registry, tc.cfg, "native-session")
			if (err != nil) != tc.rejects {
				t.Fatalf("unexpected registration result: %v", err)
			}
			tool, lookupErr := registry.GetTool(tools.ToolGovernedDataSchema)
			if (lookupErr == nil) != tc.registered {
				t.Fatal("wrong tool visibility")
			}
			if tc.registered {
				_, err = tool.Execute(tc.ctx, json.RawMessage(`{}`))
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if calls != 2 {
		t.Fatalf("unexpected data requests: %d", calls)
	}
}
