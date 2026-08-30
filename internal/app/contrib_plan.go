package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nireneko/drup/internal/contribplan"
)

// contribPlanComposer is an argv-only, read-only Composer adapter. It is a
// seam so integration tests prove the request shape without installing
// Composer. Composer's JSON output, not stdout prose, crosses into the domain.
var contribPlanComposer = func(projectPath string) ([]byte, error) {
	stdout, stderr, code, err := cliRun(projectPath, "composer", "show", "--outdated", "--direct", "--format=json")
	if err != nil {
		return nil, fmt.Errorf("composer show outdated: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("composer show outdated exited %d: %s", code, sanitizeComposerDiagnostic(stderr))
	}
	return []byte(stdout), nil
}

// contribPlanWhyNot asks Composer's resolver about every proposed major before
// the ledger is exposed. The domain still derives the causal chain from the
// pinned lock/root constraints so the persisted contract remains typed and
// deterministic rather than depending on Composer's human-oriented prose.
var contribPlanWhyNot = func(projectPath, pkg, version string) error {
	_, stderr, code, err := cliRun(projectPath, "composer", "prohibits", pkg, version)
	if err != nil {
		return fmt.Errorf("composer prohibits %s %s: %w", pkg, version, err)
	}
	if code != 0 {
		return fmt.Errorf("composer prohibits %s %s exited %d: %s", pkg, version, code, sanitizeComposerDiagnostic(stderr))
	}
	return nil
}

type composerOutdated struct {
	Installed []struct {
		Name   string `json:"name"`
		Latest string `json:"latest"`
	} `json:"installed"`
}

func realHandleContribPlan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.RunID == "" {
		return nil, fmt.Errorf("project_path and run_id are required")
	}
	store, root, err := canonicalRunStore(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	if _, err := store.Get(params.RunID); err != nil {
		return nil, err
	}
	lockData, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	if err != nil {
		return nil, fmt.Errorf("read composer.lock: %w", err)
	}
	lock, err := contribplan.ParseLock(lockData)
	if err != nil {
		return nil, err
	}
	outdatedData, err := contribPlanComposer(root)
	if err != nil {
		return nil, err
	}
	outdated, err := parseComposerOutdated(outdatedData)
	if err != nil {
		return nil, err
	}
	rootRequirements, err := composerRootRequirements(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil, err
	}
	plan, err := contribplan.Build(contribplan.Input{Lock: lock, RootRequirements: rootRequirements, Outdated: outdated})
	if err != nil {
		return nil, err
	}
	for _, group := range plan.Groups {
		if group.Kind != contribplan.GroupMajor {
			continue
		}
		for _, target := range group.Targets {
			if err := contribPlanWhyNot(root, target, outdated[target]); err != nil {
				return nil, err
			}
		}
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	run, err := store.RecordContribPlan(params.RunID, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"success": true, "project_path": root, "run_id": run.ID, "plan": plan})
}

func composerRootRequirements(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}
	var composer struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}
	for name, constraint := range composer.RequireDev {
		if _, exists := composer.Require[name]; !exists {
			composer.Require[name] = constraint
		}
	}
	return composer.Require, nil
}

func parseComposerOutdated(raw []byte) (map[string]string, error) {
	var output composerOutdated
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("parse composer outdated JSON: %w", err)
	}
	updates := make(map[string]string, len(output.Installed))
	for _, item := range output.Installed {
		if item.Name != "" && item.Latest != "" {
			updates[item.Name] = item.Latest
		}
	}
	return updates, nil
}

// sanitizeComposerDiagnostic never stores arbitrary Composer output in run
// state or MCP responses: it can include local repository URLs and tokens.
func sanitizeComposerDiagnostic(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}
