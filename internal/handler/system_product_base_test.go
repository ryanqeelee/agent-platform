package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetProductBaseDescriptor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousCommitID := CommitID
	CommitID = "0123456789abcdef"
	t.Cleanup(func() { CommitID = previousCommitID })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	new(SystemHandler).GetProductBaseDescriptor(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	want := `"schema_version":"ProductShellBrandV1"`
	if body := w.Body.String(); !strings.Contains(body, want) ||
		!strings.Contains(body, `"type":"weknora"`) ||
		!strings.Contains(body, `"product_version":"v0.8.0"`) ||
		!strings.Contains(body, `"source_commit":"0123456789abcdef"`) {
		t.Fatalf("descriptor = %s", body)
	}
}
