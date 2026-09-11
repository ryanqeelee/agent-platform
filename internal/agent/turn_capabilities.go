package agent

import (
	"fmt"
	"strings"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
)

// Report the effective registry, not the configured allowlist or best-effort
// KB metadata. Availability is not a claim of index health or query success.
// This is request-local context, never a new authority or routing decision.
func (e *AgentEngine) buildTurnCapabilities() string {
	hasTool := func(names ...string) bool {
		if e.toolRegistry == nil {
			return false
		}
		for _, name := range names {
			if _, err := e.toolRegistry.GetTool(name); err == nil {
				return true
			}
		}
		return false
	}
	scope := "none_in_scope"
	if types.HasKnowledgeRetrievalScope(e.config.SearchTargets, e.config.KnowledgeBases, e.config.KnowledgeIDs) {
		scope = "configured"
	}
	knowledge := hasTool(agenttools.ToolKnowledgeSearch, agenttools.ToolGrepChunks,
		agenttools.ToolWikiSearch, agenttools.ToolWikiReadPage,
		agenttools.ToolWikiReadSourceDoc, agenttools.ToolListKnowledgeChunks,
		agenttools.ToolGetDocumentInfo, agenttools.ToolQueryKnowledgeGraph)
	var b strings.Builder
	b.WriteString("<turn_capabilities>\n")
	fmt.Fprintf(&b, "<enterprise_knowledge scope=\"%s\" tools_available=\"%t\"/>\n", scope, knowledge)
	if scope == "none_in_scope" {
		b.WriteString("No enterprise knowledge source is attached to this turn's retrieval scope. This says nothing about the user's permissions or whether the enterprise has documents. Use actual conversation attachments and produced resources for the parts they support.\n")
	} else if !knowledge {
		b.WriteString("Knowledge scope is configured, but no knowledge retrieval/read tool is available this turn. Do not infer that the knowledge base is empty or that a search returned no results.\n")
	} else {
		b.WriteString("Use the registered knowledge tools when enterprise evidence is required. Index health, matching content and coverage are not established until retrieval returns.\n")
	}
	fmt.Fprintf(&b, "<public_web search_available=\"%t\" fetch_available=\"%t\"/>\n",
		hasTool(agenttools.ToolWebSearch), hasTool(agenttools.ToolWebFetch))
	fmt.Fprintf(&b, "<skills read_available=\"%t\" execute_available=\"%t\"/>\n",
		hasTool(agenttools.ToolReadSkill), hasTool(agenttools.ToolExecuteSkillScript))
	b.WriteString("These availability facts describe this turn only. Response mode does not grant broader enterprise knowledge permissions or establish missing evidence; public web availability follows the registered tools above. Skills and the sandbox do not provide a hidden enterprise knowledge or web-search endpoint; do not probe directories or environment variables to find one. Use conversation attachments and already-produced files when relevant; creating a new file requires no input attachment. Skill instructions can be read without preparing execution resources.\n")
	b.WriteString("</turn_capabilities>")
	return b.String()
}
