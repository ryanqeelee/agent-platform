package router

import (
	"os"
	"strings"
	"testing"
)

func TestKnowledgeShareAndActivityRoutesUseCurrentAdminAuthority(t *testing.T) {
	agentRoutes, err := os.ReadFile("routes_agent.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		`kbShares.POST("", g.Admin(), orgHandler.ShareKnowledgeBase)`,
		`kbShares.PUT("/:share_id", g.Admin(), orgHandler.UpdateSharePermission)`,
		`kbShares.DELETE("/:share_id", g.Admin(), orgHandler.RemoveShare)`,
	} {
		if !strings.Contains(string(agentRoutes), route) {
			t.Fatalf("knowledge share route missing current Admin guard: %s", route)
		}
	}
	knowledgeRoutes, err := os.ReadFile("routes_knowledge.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(knowledgeRoutes), `g.Admin(), g.KBAccessRead("id"), auditHandler.ListKnowledgeBaseActivity`) {
		t.Fatal("knowledge base activity route must require current Admin authority")
	}
}
