package service

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestEmployeeDefaultBundlesAreInstallableArchives(t *testing.T) {
	for _, name := range []string{"document-analyzer", "data-processor", "citation-generator", "doc-coauthoring"} {
		raw, err := builtinSkillArchive(filepath.Join("..", "..", "..", "skills", "preloaded", name))
		require.NoError(t, err)
		bundle, err := ParseSkillBundle(raw)
		require.NoError(t, err)
		require.NotEmpty(t, bundle.Name)
		require.NotEmpty(t, bundle.Instructions)
	}
}
