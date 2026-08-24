package policy

import (
	"testing"
)

func TestEnterpriseManagedKnowledgePolicy(t *testing.T) {
	if SharingAvailable() {
		t.Fatal("sharing must be unavailable")
	}
}
