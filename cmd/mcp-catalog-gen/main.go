// Command mcp-catalog-gen writes deterministic MCP catalog artifacts.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nireneko/drup/internal/packaging"
)

func main() {
	root, err := findRepositoryRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	checkOnly := len(os.Args) == 2 && os.Args[1] == "--check"
	if len(os.Args) > 1 && !checkOnly {
		fmt.Fprintln(os.Stderr, "usage: mcp-catalog-gen [--check]")
		os.Exit(2)
	}
	if checkOnly {
		err = packaging.CheckCatalogArtifacts(root)
	} else {
		err = packaging.WriteCatalogArtifacts(root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func findRepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repository root from %s", dir)
		}
		dir = parent
	}
}
