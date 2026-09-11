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
		if body["source_id"] != "" {
			t.Error("source must be resolved by Center")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"GovernedDataSchemaV1","source":{"source_id":"business"},"catalog_version":"version-1","catalog_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tables":[]}`))
	}))
	defer endpoint.Close()
	s := &agentService{cfg: &config.Config{Agent: &config.AgentConfig{GovernedData: &config.GovernedDataConfig{BaseURL: endpoint.URL}}}}
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user-a")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10001))
	bound := types.WithGovernedDataUserCredential(ctx, "user-jwt")
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
	if calls != 1 {
		t.Fatalf("unexpected data requests: %d", calls)
	}
	s.cfg.Agent.GovernedData = nil
	if err := s.registerGovernedDataTools(bound, tools.NewToolRegistry(), selected, "native-session"); err == nil {
		t.Fatal("unconfigured service accepted")
	}
}
