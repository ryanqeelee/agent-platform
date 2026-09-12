package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestLoadBuiltinAnalysisPromptTemplates(t *testing.T) {
	configDir := filepath.Join("..", "..", "config")
	prompts, err := loadPromptTemplates(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if prompts == nil {
		t.Fatal("prompt templates were not loaded")
	}

	cases := []struct {
		templateID string
		agentID    string
		want       []string
	}{
		{templateID: "data_analysis_base", agentID: types.BuiltinDataAnalysisBaseID, want: []string{"You are 环枢数据分析 Base.", "data_schema", "/workspace/output"}},
		{templateID: "operating_analyst", agentID: types.BuiltinOperatingAnalystID, want: []string{"You are 环枢经营分析助手.", "governed_data_schema", "query.truncated", "governed_compare.py", "inputs[].query_id", "/workspace/output"}},
	}
	for _, tc := range cases {
		t.Run(tc.templateID, func(t *testing.T) {
			template := FindTemplateByID(prompts, tc.templateID)
			if template == nil {
				t.Fatalf("template %q was not found after loading prompt_templates", tc.templateID)
			}
			for _, want := range tc.want {
				if !strings.Contains(template.Content, want) {
					t.Fatalf("template %q content is missing %q", tc.templateID, want)
				}
			}
		})
	}

	if err := types.LoadBuiltinAgentsConfig(configDir); err != nil {
		t.Fatal(err)
	}
	resolveBuiltinAgentPromptIDs(prompts)
	for _, tc := range cases {
		agent := types.GetBuiltinAgent(tc.agentID, 1)
		if agent == nil {
			t.Fatalf("builtin %q was not loaded", tc.agentID)
		}
		for _, want := range tc.want {
			if !strings.Contains(agent.Config.SystemPrompt, want) {
				t.Fatalf("builtin %q resolved prompt is missing %q", tc.agentID, want)
			}
		}
	}
}
