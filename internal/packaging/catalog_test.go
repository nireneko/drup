package packaging

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestBuildCatalogArtifactsMatchesCommittedOutputs(t *testing.T) {
	root := repositoryRoot(t)
	if err := CheckCatalogArtifacts(root); err != nil {
		t.Fatalf("CheckCatalogArtifacts() error = %v", err)
	}
}

func TestBuildCatalogArtifactsIsDeterministic(t *testing.T) {
	root := repositoryRoot(t)
	first, err := BuildCatalogArtifacts(root)
	if err != nil {
		t.Fatalf("first BuildCatalogArtifacts() error = %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("BuildCatalogArtifacts() produced %d artifacts, want 3", len(first))
	}
	second, err := BuildCatalogArtifacts(root)
	if err != nil {
		t.Fatalf("second BuildCatalogArtifacts() error = %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("artifact count differs: first=%d second=%d", len(first), len(second))
	}
	for path, firstBytes := range first {
		if !bytes.Equal(firstBytes, second[path]) {
			t.Errorf("generated artifact %s differs between two runs", path)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
