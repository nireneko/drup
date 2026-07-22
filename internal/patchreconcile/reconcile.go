// Package patchreconcile provides deterministic, analysis-only checks for
// applied community patches: whether a newer revision exists on the source
// issue queue, and whether the patch's change has already been merged
// upstream (obsolete). It uses drupal.org's JSON api-d7 endpoint only — no
// HTML parsing — and performs no mutation.
package patchreconcile

import (
	"fmt"

	"github.com/nireneko/drup/internal/drupalorg"
)

// searchIssues is the drupal.org lookup used by Reconcile.
// Package-level var for testability — tests may override it directly, but
// production code goes through drupalorg's own exported HTTP seams
// (HTTPClient, APID7BaseURL) so behavior is exercised end-to-end via httptest.
var searchIssues = drupalorg.SearchIssuesAPI

// mergedStatuses lists drupal.org issue statuses that indicate the patch's
// change has already landed upstream and is safe to remove.
var mergedStatuses = map[string]bool{
	"Fixed":          true,
	"Closed (fixed)": true,
}

// Result is the outcome of reconciling a module's currently-applied patch
// against drupal.org's issue queue.
type Result struct {
	NewerPatches   []drupalorg.PatchInfo `json:"newer_patches"`
	IsStillNeeded  bool                  `json:"is_still_needed"`
	Recommendation string                `json:"recommendation"`
}

// Reconcile checks currentPatchURL (the issue/patch currently applied for
// module) against the module's issue queue on drupal.org.
//
//   - If the issue tied to currentPatchURL has a status in mergedStatuses, the
//     patch is reported obsolete (IsStillNeeded=false) — the change has landed
//     upstream and the patch entry is safe to remove from composer.json.
//   - Otherwise, any other open issue/patch found for the module is reported
//     as a newer candidate to review, and IsStillNeeded remains true.
func Reconcile(module, currentPatchURL string) (*Result, error) {
	if module == "" {
		return nil, fmt.Errorf("module must not be empty")
	}
	if currentPatchURL == "" {
		return nil, fmt.Errorf("current_patch_url must not be empty")
	}

	issues, err := searchIssues(module)
	if err != nil {
		return nil, fmt.Errorf("search issue queue for %s: %w", module, err)
	}

	var current *drupalorg.PatchInfo
	newer := []drupalorg.PatchInfo{}
	for i := range issues {
		issue := issues[i]
		if issue.URL == currentPatchURL {
			c := issue
			current = &c
			continue
		}
		newer = append(newer, issue)
	}

	if current != nil && mergedStatuses[current.Status] {
		return &Result{
			NewerPatches:   []drupalorg.PatchInfo{},
			IsStillNeeded:  false,
			Recommendation: fmt.Sprintf("remove: issue for %s is marked %q upstream; patch is obsolete", currentPatchURL, current.Status),
		}, nil
	}

	if len(newer) == 0 {
		return &Result{
			NewerPatches:   []drupalorg.PatchInfo{},
			IsStillNeeded:  true,
			Recommendation: "keep: no newer patch found on the issue queue; patch still required",
		}, nil
	}

	return &Result{
		NewerPatches:   newer,
		IsStillNeeded:  true,
		Recommendation: fmt.Sprintf("review: %d newer patch(es) available on the issue queue", len(newer)),
	}, nil
}
