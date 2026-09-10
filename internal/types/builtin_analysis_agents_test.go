package types

import (
	"path/filepath"
	"testing"
)

func TestBuiltinAnalysisProductsAreIndependentNativeDefinitions(t *testing.T) {
	if err := LoadBuiltinAgentsConfig(filepath.Join("..", "..", "config")); err != nil {
		t.Fatal(err)
	}

	fileAnalyst := GetBuiltinAgent(BuiltinDataAnalystID, 7)
	base := GetBuiltinAgent(BuiltinDataAnalysisBaseID, 7)
	operating := GetBuiltinAgent(BuiltinOperatingAnalystID, 7)
	if fileAnalyst == nil || base == nil || operating == nil {
		t.Fatal("all native analysis product definitions must resolve")
	}
	if fileAnalyst.ID != "builtin-data-analyst" || fileAnalyst.Config.SystemPromptID != "data_analyst" {
		t.Fatal("existing data analyst definition changed")
	}
	if base.ID == fileAnalyst.ID || operating.ID == fileAnalyst.ID || base.ID == operating.ID {
		t.Fatal("analysis products must have independent identities")
	}
	if base.Config.SystemPromptID != "data_analysis_base" || operating.Config.SystemPromptID != "operating_analyst" {
		t.Fatal("analysis products must select independent prompts")
	}
	if base.Config.AgentType != AgentTypeDataAnalysis || operating.Config.AgentType != AgentTypeDataAnalysis {
		t.Fatal("analysis products must use the native data-analysis engine category")
	}
	for _, agent := range []*CustomAgent{base, operating} {
		if !agent.Config.ImageUploadEnabled || agent.Config.KBSelectionMode != "all" || !agent.Config.WebSearchEnabled || agent.Config.CitationEnabled == nil || !*agent.Config.CitationEnabled {
			t.Fatalf("native analysis input capabilities missing for %s", agent.ID)
		}
	}
	if !containsStrings(base.Config.AllowedTools, "data_schema", "data_analysis", "knowledge_search", "wiki_search", "wiki_read_page") {
		t.Fatalf("unexpected Base tools: %v", base.Config.AllowedTools)
	}
	if !containsStrings(operating.Config.AllowedTools, "governed_data_schema", "governed_data_query", "data_schema", "data_analysis", "knowledge_search", "wiki_search", "wiki_read_page") {
		t.Fatalf("unexpected operating tools: %v", operating.Config.AllowedTools)
	}
	if base.Config.SkillsSelectionMode != "all" || operating.Config.SkillsSelectionMode != "all" {
		t.Fatal("native analysis products must select the shared sandbox skill lane")
	}
}

func containsStrings(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, value := range got {
		set[value] = true
	}
	for _, value := range want {
		if !set[value] {
			return false
		}
	}
	return true
}
