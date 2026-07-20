package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReportData contains all data needed to generate upgrade reports.
type ReportData struct {
	ProjectPath    string          `json:"project_path"`
	TotalErrors    int             `json:"total_errors"`
	Resolved       []ResolvedItem  `json:"resolved"`
	Pending        []PendingItem   `json:"pending"`
	TokenAccounting TokenAccounting `json:"token_accounting"`
}

// ResolvedItem represents a successfully resolved error.
type ResolvedItem struct {
	Module string `json:"module"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// PendingItem represents an unresolved error requiring human review.
type PendingItem struct {
	Module          string `json:"module"`
	Type            string `json:"type"`
	Error           string `json:"error"`
	SuggestedAction string `json:"suggested_action"`
}

// TokenAccounting tracks token usage across sub-agent invocations.
type TokenAccounting struct {
	Total   int            `json:"total"`
	ByAgent map[string]int `json:"by_agent"`
}

// GenerateJSON produces a JSON report.
func GenerateJSON(data *ReportData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// GenerateMarkdown produces a human-readable markdown report.
func GenerateMarkdown(data *ReportData) (string, error) {
	var b strings.Builder

	// Summary.
	b.WriteString("# Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Project**: %s\n", data.ProjectPath))
	b.WriteString(fmt.Sprintf("- **Total errors**: %d\n", data.TotalErrors))
	b.WriteString(fmt.Sprintf("- **Resolved**: %d\n", len(data.Resolved)))
	b.WriteString(fmt.Sprintf("- **Pending**: %d\n", len(data.Pending)))
	b.WriteString("\n")

	// Resolved.
	b.WriteString("# Resolved\n\n")
	if len(data.Resolved) == 0 {
		b.WriteString("_No items resolved._\n\n")
	} else {
		b.WriteString("| Module | Type | Detail |\n")
		b.WriteString("|--------|------|--------|\n")
		for _, r := range data.Resolved {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Module, r.Type, r.Detail))
		}
		b.WriteString("\n")
	}

	// Pending.
	b.WriteString("# Pending Human Review\n\n")
	if len(data.Pending) == 0 {
		b.WriteString("_No pending items._\n\n")
	} else {
		b.WriteString("| Module | Type | Error | Suggested Action |\n")
		b.WriteString("|--------|------|-------|------------------|\n")
		for _, p := range data.Pending {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", p.Module, p.Type, p.Error, p.SuggestedAction))
		}
		b.WriteString("\n")
	}

	// Token Usage.
	b.WriteString("# Token Usage\n\n")
	b.WriteString(fmt.Sprintf("- **Total**: %d\n", data.TokenAccounting.Total))
	if len(data.TokenAccounting.ByAgent) > 0 {
		b.WriteString("- **By agent**:\n")
		for agent, tokens := range data.TokenAccounting.ByAgent {
			b.WriteString(fmt.Sprintf("  - %s: %d\n", agent, tokens))
		}
	}

	return b.String(), nil
}
