package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contribProject(t *testing.T, module, constraint string) string {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "composer.json"),
		[]byte(`{"extra":{"drupal-scaffold":{"locations":{"web-root":"web/"}}},"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"}}`), 0o644)
	os.MkdirAll(filepath.Join(root, "vendor", "mglaman", "composer-drupal-lenient"), 0o755)
	dir := filepath.Join(root, "web", "modules", "contrib", module)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, module+".info.yml"),
		[]byte("name: Test\ntype: module\ncore_version_requirement: "+constraint+"\n"), 0o644)
	return root
}

// A patch alone does not make a module installable: composer resolves the core
// requirement from published metadata, never from patched files. The patch and
// the lenient listing only work together.
func TestPatchContribForCore_PatchesRegistersAndAllows(t *testing.T) {
	root := contribProject(t, "variationcache", "^9.5 || ^10")

	result, err := PatchContribForCore(root, "variationcache", "11", false, true)
	if err != nil {
		t.Fatalf("PatchContribForCore error: %v", err)
	}

	if result.After != "^9.5 || ^10 || ^11" {
		t.Errorf("After = %q, want the original constraint widened", result.After)
	}
	if !result.Registered || !result.LenientListed {
		t.Errorf("registered=%v lenient=%v, both are required for composer to accept it", result.Registered, result.LenientListed)
	}

	body, err := os.ReadFile(result.PatchPath)
	if err != nil {
		t.Fatalf("patch not written: %v", err)
	}
	patchText := string(body)
	if !strings.Contains(patchText, "variationcache.info.yml") || !strings.Contains(patchText, "^11") {
		t.Errorf("patch does not widen the declaration:\n%s", patchText)
	}
	if strings.Contains(patchText, root) {
		t.Errorf("patch carries absolute paths and will not apply with -p1:\n%s", patchText)
	}

	var composer struct {
		Extra struct {
			Patches map[string]map[string]string `json:"patches"`
			Lenient struct {
				AllowedList []string `json:"allowed-list"`
			} `json:"drupal-lenient"`
		} `json:"extra"`
	}
	data, _ := os.ReadFile(filepath.Join(root, "composer.json"))
	if err := json.Unmarshal(data, &composer); err != nil {
		t.Fatalf("composer.json is no longer valid: %v", err)
	}
	if len(composer.Extra.Patches["drupal/variationcache"]) != 1 {
		t.Errorf("patch not registered: %+v", composer.Extra.Patches)
	}
	if len(composer.Extra.Lenient.AllowedList) != 1 {
		t.Errorf("package not added to the lenient list: %+v", composer.Extra.Lenient.AllowedList)
	}
}

func TestPatchContribForCore_SkipsModulesAlreadyCompatible(t *testing.T) {
	root := contribProject(t, "token", "^10 || ^11")

	result, err := PatchContribForCore(root, "token", "11", false, true)
	if err != nil {
		t.Fatalf("PatchContribForCore error: %v", err)
	}
	if result.PatchPath != "" || result.Registered {
		t.Errorf("a compatible module was patched anyway: %+v", result)
	}
	if !strings.Contains(result.Note, "already declares") {
		t.Errorf("note = %q, want it to say no patch was needed", result.Note)
	}
}

func TestPatchContribForCore_DryRunWritesNothing(t *testing.T) {
	root := contribProject(t, "variationcache", "^9.5 || ^10")
	infoPath := filepath.Join(root, "web", "modules", "contrib", "variationcache", "variationcache.info.yml")
	before, _ := os.ReadFile(infoPath)

	result, err := PatchContribForCore(root, "variationcache", "11", true, true)
	if err != nil {
		t.Fatalf("PatchContribForCore error: %v", err)
	}
	if result.PatchPath != "" || result.Registered || result.LenientListed {
		t.Errorf("dry run made changes: %+v", result)
	}
	after, _ := os.ReadFile(infoPath)
	if string(before) != string(after) {
		t.Error("dry run modified the module")
	}
}

func TestPatchContribForCore_ReportsAMissingModule(t *testing.T) {
	root := contribProject(t, "token", "^10")
	if _, err := PatchContribForCore(root, "absent_module", "11", false, true); err == nil {
		t.Error("a module that is not installed was accepted")
	}
}

// A widened declaration only claims compatibility. The patch has to carry the
// code fixes that make the claim true, which is the same treatment the
// project's own modules get.
func TestPatchContribForCore_RunsRectorUnlessDeclarationOnly(t *testing.T) {
	root := contribProject(t, "promotur_sso_application", "^9 || ^10")

	origRun := drupexecRunWithEnv()
	defer restoreRunWithEnv(origRun)
	var ranRector, ranStandards bool
	setRunWithEnv(func(dir string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		switch {
		case strings.Contains(cmd, "rector"):
			ranRector = true
		case strings.Contains(cmd, "phpcbf"):
			ranStandards = true
		}
		return "", "", 0, nil
	})

	// rector needs a config and phpcbf needs to exist for the pass to run.
	os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644)
	os.MkdirAll(filepath.Join(root, "vendor", "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "vendor", "bin", "phpcbf"), []byte("#!/bin/sh"), 0o755)

	if _, err := PatchContribForCore(root, "promotur_sso_application", "11", false, false); err != nil {
		t.Fatalf("PatchContribForCore error: %v", err)
	}
	if !ranRector {
		t.Error("rector was not run over the module")
	}
	if !ranStandards {
		t.Error("the coding standards pass was not run over the module")
	}

	ranRector, ranStandards = false, false
	root2 := contribProject(t, "other_module", "^9 || ^10")
	if _, err := PatchContribForCore(root2, "other_module", "11", false, true); err != nil {
		t.Fatalf("PatchContribForCore error: %v", err)
	}
	if ranRector || ranStandards {
		t.Error("--declaration-only still ran the code passes")
	}
}
