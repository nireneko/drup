package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nireneko/drup/internal/inventory"
	"github.com/nireneko/drup/internal/metrics"
	"github.com/nireneko/drup/internal/runstate"
)

type ReportData struct {
	ProjectPath string         `json:"project_path"`
	TotalErrors int            `json:"total_errors"`
	Resolved    []ResolvedItem `json:"resolved"`
	Pending     []PendingItem  `json:"pending"`
	// The run snapshot is deliberately separate from scan results and token accounting.
	Before          *inventory.Inventory `json:"before,omitempty"`
	After           *inventory.Inventory `json:"after,omitempty"`
	Changes         []Change             `json:"changes,omitempty"`
	Checkpoints     []EvidenceLink       `json:"checkpoints,omitempty"`
	TokenAccounting TokenAccounting      `json:"token_accounting"`
	PipelineMetrics *metrics.Metrics     `json:"pipeline_metrics,omitempty"`
}
type ResolvedItem struct {
	Module string `json:"module"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}
type PendingItem struct {
	Module          string `json:"module"`
	Type            string `json:"type"`
	Error           string `json:"error"`
	SuggestedAction string `json:"suggested_action"`
}
type TokenAccounting struct {
	Total   int            `json:"total"`
	ByAgent map[string]int `json:"by_agent"`
}
type EvidenceLink struct {
	Kind           string   `json:"kind"`
	Summary        string   `json:"summary"`
	PayloadHash    string   `json:"payload_hash,omitempty"`
	ValidationHash string   `json:"validation_hash,omitempty"`
	CandidateHash  string   `json:"candidate_hash,omitempty"`
	CommitHash     string   `json:"commit_hash,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	Target         string   `json:"target,omitempty"`
}
type Change struct {
	inventory.Change
	Evidence []EvidenceLink `json:"evidence"`
}

// BuildFromRun creates a complete report input exclusively from persisted run state.
// It never captures inventory or calls a scanner, which makes restarted output reproducible.
func BuildFromRun(run runstate.Run) (*ReportData, error) {
	if run.InventoryBaseline == nil || run.InventoryFinal == nil {
		return nil, fmt.Errorf("inventory snapshots are incomplete")
	}
	if run.InventoryBaseline.SchemaVersion != inventory.SchemaVersion || run.InventoryFinal.SchemaVersion != inventory.SchemaVersion || run.InventoryBaseline.Core.Version == "" || run.InventoryFinal.Core.Version == "" {
		return nil, fmt.Errorf("inventory snapshots are invalid")
	}
	links := make([]EvidenceLink, 0, len(run.Evidence))
	for _, e := range run.Evidence {
		if e.Kind == "" {
			return nil, fmt.Errorf("run evidence is incomplete")
		}
		links = append(links, EvidenceLink{Kind: e.Kind, Summary: e.Summary, PayloadHash: e.PayloadHash, ValidationHash: e.ValidationHash, CandidateHash: e.CandidateHash, CommitHash: e.CommitHash, Paths: append([]string(nil), e.Paths...), Target: e.Target})
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("run evidence is incomplete")
	}
	sort.Slice(links, func(i, j int) bool { return evidenceKey(links[i]) < evidenceKey(links[j]) })
	delta := inventory.Delta(*run.InventoryBaseline, *run.InventoryFinal)
	if err := requirePatchEvidence(run, *run.InventoryBaseline, *run.InventoryFinal, links); err != nil {
		return nil, err
	}
	changes := make([]Change, len(delta))
	for i, c := range delta {
		changeLinks, err := evidenceForChange(run, c, links)
		if err != nil {
			return nil, err
		}
		changes[i] = Change{Change: c, Evidence: changeLinks}
	}
	return &ReportData{ProjectPath: run.Root, Resolved: []ResolvedItem{}, Pending: []PendingItem{}, Before: run.InventoryBaseline, After: run.InventoryFinal, Changes: changes, Checkpoints: links, TokenAccounting: TokenAccounting{ByAgent: map[string]int{}}}, nil
}

// evidenceForChange accepts only typed, path-bound evidence. A report must not
// turn an unrelated non-empty evidence list into provenance for every delta.
func evidenceForChange(run runstate.Run, change inventory.Change, links []EvidenceLink) ([]EvidenceLink, error) {
	matched := make([]EvidenceLink, 0, len(links))
	commitCandidate := ""
	validationCandidate := ""
	path := evidencePath(change)
	for _, link := range links {
		if !containsPath(link.Paths, path) {
			continue
		}
		matched = append(matched, link)
		if link.Kind == "checkpoint_commit" && strings.TrimSpace(link.CommitHash) != "" && strings.TrimSpace(link.CandidateHash) != "" {
			commitCandidate = link.CandidateHash
		}
		if link.Kind == "validation" && strings.TrimSpace(link.ValidationHash) != "" && strings.TrimSpace(link.CandidateHash) != "" && strings.TrimSpace(link.Target) != "" {
			validationCandidate = link.CandidateHash
		}
	}
	if commitCandidate == "" || validationCandidate == "" || commitCandidate != validationCandidate || !hasBoundCheckpointPlan(run.CheckpointHistory, path, validationCandidate) {
		return nil, fmt.Errorf("change %s %s lacks bound commit, backup, or validation evidence", change.Kind, change.Name)
	}
	return matched, nil
}

func evidencePath(change inventory.Change) string {
	if change.Source == "filesystem" {
		return change.Name
	}
	return change.Source
}

func requirePatchEvidence(run runstate.Run, before, after inventory.Inventory, links []EvidenceLink) error {
	known := make(map[string]struct{}, len(before.Patches))
	for _, patch := range before.Patches {
		known[patchKey(patch)] = struct{}{}
	}
	for _, patch := range after.Patches {
		if _, ok := known[patchKey(patch)]; ok {
			continue
		}
		bound := false
		for _, link := range links {
			if link.Kind != "patch" || !containsPath(link.Paths, patch.Source) || strings.TrimSpace(link.CandidateHash) == "" || strings.TrimSpace(link.Target) == "" {
				continue
			}
			if patchEvidenceMatchesRun(run, patch.Source, link) {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("patch change %s lacks bound patch evidence", patch.Description)
		}
	}
	return nil
}

func patchEvidenceMatchesRun(run runstate.Run, source string, patch EvidenceLink) bool {
	for _, evidence := range run.Evidence {
		if !containsPath(evidence.Paths, source) || evidence.CandidateHash != patch.CandidateHash {
			continue
		}
		if evidence.Kind == "checkpoint_commit" && strings.TrimSpace(evidence.CommitHash) != "" {
			for _, validation := range run.Evidence {
				if validation.Kind != "validation" || validation.CandidateHash != patch.CandidateHash || validation.Target != patch.Target || strings.TrimSpace(validation.ValidationHash) == "" || !containsPath(validation.Paths, source) {
					continue
				}
				if hasBoundCheckpointPlanForTarget(run.CheckpointHistory, source, patch.CandidateHash, patch.Target) {
					return true
				}
			}
		}
	}
	return false
}

func patchKey(patch inventory.Patch) string {
	return patch.Package + "\x00" + patch.Description + "\x00" + patch.URL + "\x00" + patch.Source
}

func hasBoundCheckpointPlan(plans []runstate.CheckpointPlan, source, candidateHash string) bool {
	return hasBoundCheckpointPlanForTarget(plans, source, candidateHash, "")
}

func hasBoundCheckpointPlanForTarget(plans []runstate.CheckpointPlan, source, candidateHash, target string) bool {
	for _, plan := range plans {
		if strings.TrimSpace(plan.BackupID) == "" || plan.CandidateHash != candidateHash || !containsPath(plan.Paths, source) || (target != "" && target != fmt.Sprint(plan.TargetMajor)) {
			continue
		}
		backup, validation := false, false
		for _, step := range plan.Steps {
			if step.Status != runstate.CheckpointStepSucceeded || step.Evidence == nil || !containsPath(step.Evidence.Paths, source) {
				continue
			}
			if step.Name == runstate.CheckpointStepBackup && strings.TrimSpace(step.Evidence.CommandHash) != "" && strings.TrimSpace(step.Evidence.OutputHash) != "" {
				backup = true
			}
			if step.Name == runstate.CheckpointStepValidation && strings.TrimSpace(step.Evidence.CommandHash) != "" && strings.TrimSpace(step.Evidence.OutputHash) != "" && step.Evidence.CandidateHash == candidateHash {
				validation = true
			}
		}
		if backup && validation {
			return true
		}
	}
	return false
}

func containsPath(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}
func evidenceKey(e EvidenceLink) string {
	return e.Kind + "\x00" + e.CommitHash + "\x00" + e.PayloadHash + "\x00" + e.Summary
}
func GenerateJSON(data *ReportData) ([]byte, error) {
	normalize(data)
	return json.MarshalIndent(data, "", "  ")
}
func normalize(data *ReportData) {
	if data.Resolved == nil {
		data.Resolved = []ResolvedItem{}
	}
	if data.Pending == nil {
		data.Pending = []PendingItem{}
	}
	if data.TokenAccounting.ByAgent == nil {
		data.TokenAccounting.ByAgent = map[string]int{}
	}
	sort.Slice(data.Resolved, func(i, j int) bool { return data.Resolved[i].Module < data.Resolved[j].Module })
	sort.Slice(data.Pending, func(i, j int) bool { return data.Pending[i].Module < data.Pending[j].Module })
}
func GenerateMarkdown(data *ReportData) (string, error) {
	normalize(data)
	var b strings.Builder
	b.WriteString("# Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Project**: %s\n- **Total errors**: %d\n- **Resolved**: %d\n- **Pending**: %d\n\n", data.ProjectPath, data.TotalErrors, len(data.Resolved), len(data.Pending)))
	writeInventory := func(name string, value *inventory.Inventory) {
		b.WriteString("# " + name + "\n\n")
		if value == nil {
			b.WriteString("_Not captured._\n\n")
			return
		}
		b.WriteString(fmt.Sprintf("- **Core**: %s (%s)\n- **PHP**: %s (%s)\n- **Packages**: %d\n- **Extensions**: %d\n- **Patches**: %d\n\n", value.Core.Version, value.Core.Source, value.PHP.Version, value.PHP.Source, len(value.Packages), len(value.Extensions), len(value.Patches)))
	}
	writeInventory("Before", data.Before)
	writeInventory("After", data.After)
	b.WriteString("# Changes\n\n")
	if len(data.Changes) == 0 {
		b.WriteString("_No inventory changes._\n\n")
	} else {
		for _, c := range data.Changes {
			b.WriteString(fmt.Sprintf("- **%s %s**: %s → %s\n", c.Kind, c.Name, c.Before, c.After))
		}
		b.WriteString("\n")
	}
	b.WriteString("# Checkpoints\n\n")
	if len(data.Checkpoints) == 0 {
		b.WriteString("_No persisted checkpoint evidence._\n\n")
	} else {
		for _, e := range data.Checkpoints {
			b.WriteString(fmt.Sprintf("- **%s**: %s", e.Kind, e.Summary))
			if e.CommitHash != "" {
				b.WriteString(" (commit " + e.CommitHash + ")")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("# Resolved\n\n")
	if len(data.Resolved) == 0 {
		b.WriteString("_No items resolved._\n\n")
	} else {
		for _, r := range data.Resolved {
			b.WriteString(fmt.Sprintf("- %s: %s\n", r.Module, r.Detail))
		}
		b.WriteString("\n")
	}
	b.WriteString("# Pending Human Review\n\n")
	if len(data.Pending) == 0 {
		b.WriteString("_No pending items._\n\n")
	} else {
		for _, p := range data.Pending {
			b.WriteString(fmt.Sprintf("- %s: %s\n", p.Module, p.Error))
		}
		b.WriteString("\n")
	}
	b.WriteString("# Token Usage\n\n")
	b.WriteString(fmt.Sprintf("- **Total**: %d\n", data.TokenAccounting.Total))
	agents := make([]string, 0, len(data.TokenAccounting.ByAgent))
	for a := range data.TokenAccounting.ByAgent {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	for _, a := range agents {
		b.WriteString(fmt.Sprintf("- %s: %d\n", a, data.TokenAccounting.ByAgent[a]))
	}
	if data.PipelineMetrics != nil {
		b.WriteString("\n# Pipeline Metrics\n\n")
		b.WriteString(fmt.Sprintf("- **Total duration**: %d ms\n- **Commands executed**: %d\n- **Files modified**: %d\n- **Retries**: %d\n- **Human interventions**: %d\n", data.PipelineMetrics.TotalDurationMS, data.PipelineMetrics.CommandsExecuted, data.PipelineMetrics.FilesModified, data.PipelineMetrics.Retries, data.PipelineMetrics.Interventions))
	}
	return b.String(), nil
}

// AfterPatches exposes patch reporting without coupling it to token accounting.
func (d *ReportData) AfterPatches() []inventory.Patch {
	if d.After == nil {
		return nil
	}
	return append([]inventory.Patch(nil), d.After.Patches...)
}
