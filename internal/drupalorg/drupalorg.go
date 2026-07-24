package drupalorg

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HTTPClient is the HTTP client for Drupal.org requests.
// Exported package-level var so other internal packages (e.g. patchreconcile)
// can drive real httptest-backed integration tests through this package's
// existing HTTP + parsing logic instead of duplicating it.
var HTTPClient = &http.Client{Timeout: 30 * time.Second}

// SetHTTPClientForTest overrides the package-level HTTP client for testing.
// Returns a cleanup function that restores the original client.
func SetHTTPClientForTest(c *http.Client) func() {
	orig := HTTPClient
	HTTPClient = c
	return func() { HTTPClient = orig }
}

// releaseBaseURL is the template for release-history lookups.
// Package-level var for testability.
var releaseBaseURL = "https://updates.drupal.org/release-history/%s/current"

// issueBaseURL is the template for issue queue scraping.
// Package-level var for testability.
var issueBaseURL = "https://www.drupal.org/project/issues/%s"

// APID7BaseURL is the template for api-d7 node queries (JSON API — no HTML
// parsing). Exported for the same cross-package testability reason as
// HTTPClient.
var APID7BaseURL = "https://www.drupal.org/api-d7/node.json?field_project_machine_name=%s"

// ReleaseInfo contains D11 compatibility data for a module.
type ReleaseInfo struct {
	Module   string   `json:"module"`
	HasD11   bool     `json:"has_d11_release"`
	Latest   string   `json:"latest_version"`
	Branches []string `json:"compatible_branches"`
}

// PatchInfo contains data about a patch/diff/MR from an issue.
type PatchInfo struct {
	URL      string `json:"url"`
	Status   string `json:"status"`
	Date     string `json:"date"`
	IsPatch  bool   `json:"is_patch"`
	IssueNID string `json:"issue_nid"`
}

// PatchSearchResult is the structured response from SearchPatches.
// It always includes status, module, message, and suggestion — never a bare empty array.
type PatchSearchResult struct {
	Status     string      `json:"status"`     // "patches_found" | "no_patches_found" | "error"
	Module     string      `json:"module"`
	Searched   string      `json:"searched"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion"`
	Patches    []PatchInfo `json:"patches"`
}

// XML structures for release-history parsing.
type releaseHistory struct {
	XMLName  xml.Name  `xml:"project"`
	Name     string    `xml:"name"`
	Releases []release `xml:"releases>release"`
}

type release struct {
	Name    string `xml:"name"`
	Version string `xml:"version"`
	Tag     string `xml:"tag"`
	Terms   []term `xml:"terms>term"`
}

type term struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

// apiD7Response is the JSON response from api-d7 node listing.
type apiD7Response struct {
	Nodes []apiD7Node `json:"list"`
	Next  string      `json:"next"`
}

type apiD7Node struct {
	Node apiD7NodeDetail `json:"node"`
}

type apiD7NodeDetail struct {
	NID    string `json:"nid"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// CheckRelease fetches the release-history XML for a module and determines
// D11 compatibility.
func CheckRelease(module string) (*ReleaseInfo, error) {
	url := fmt.Sprintf(releaseBaseURL, module)
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch release history for %s: %w", module, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &ReleaseInfo{Module: module, HasD11: false}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release history for %s: HTTP %d", module, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read release history: %w", err)
	}

	return parseReleaseXML(module, data)
}

func parseReleaseXML(module string, data []byte) (*ReleaseInfo, error) {
	var rh releaseHistory
	if err := xml.Unmarshal(data, &rh); err != nil {
		return nil, fmt.Errorf("parse release XML: %w", err)
	}

	info := &ReleaseInfo{
		Module:   module,
		Branches: []string{},
	}

	branchSet := make(map[string]bool)
	for _, rel := range rh.Releases {
		if info.Latest == "" {
			info.Latest = rel.Version
		}
		for _, t := range rel.Terms {
			if t.Name == "Core compatibility" && strings.Contains(t.Value, "Drupal 11") {
				info.HasD11 = true
			}
			if t.Name == "Core compatibility" {
				branchSet[t.Value] = true
			}
		}
	}

	for b := range branchSet {
		info.Branches = append(info.Branches, b)
	}

	// If HasD11 not set from terms, try to check core_version_requirement from info.yml
	if !info.HasD11 && info.Latest != "" {
		// Derive branch name from latest version (e.g., "6.3.0" -> "6.x", "1.13.0" -> "1.x")
		branch := deriveBranch(info.Latest)
		if branch != "" {
			infoYML, err := fetchInfoYML(module, branch)
			if err == nil {
				constraint := parseCoreVersionRequirement(infoYML)
				if constraint != "" {
					info.HasD11 = constraintMatchesDrupal(constraint, 11)
				}
			}
		}
	}

	return info, nil
}

// deriveBranch extracts the branch name from a version string.
// e.g., "6.3.0" -> "6.x", "1.13.0" -> "1.x"
func deriveBranch(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0] + ".x"
}

// SearchIssuesAPI queries the Drupal api-d7 endpoint for issue nodes.
// Returns patch info entries extracted from the API response.
func SearchIssuesAPI(module string) ([]PatchInfo, error) {
	url := fmt.Sprintf(APID7BaseURL, module)
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch api-d7 issues for %s: %w", module, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api-d7 for %s: HTTP %d", module, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read api-d7 response: %w", err)
	}

	return parseAPI_D7(data)
}

func parseAPI_D7(data []byte) ([]PatchInfo, error) {
	var resp apiD7Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse api-d7 JSON: %w", err)
	}

	var patches []PatchInfo
	for _, item := range resp.Nodes {
		nid := item.Node.NID
		if nid == "" {
			continue
		}
		patches = append(patches, PatchInfo{
			URL:      fmt.Sprintf("https://www.drupal.org/node/%s", nid),
			Status:   item.Node.Status,
			IssueNID: nid,
		})
	}
	return patches, nil
}

// SearchPatches searches Drupal.org for patches related to a module.
// It tries api-d7 as primary source, then falls back to HTML scraping.
// Results are returned as a structured PatchSearchResult with status, message, and suggestion.
func SearchPatches(query string) (*PatchSearchResult, error) {
	searchURL := fmt.Sprintf(issueBaseURL, query)

	// Try api-d7 first.
	patches, err := SearchIssuesAPI(query)
	if err == nil && len(patches) > 0 {
		sort.Slice(patches, func(i, j int) bool {
			return priority(patches[i].Status) < priority(patches[j].Status)
		})
		return &PatchSearchResult{
			Status:     "patches_found",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("%d patches found", len(patches)),
			Suggestion: "Apply highest-date RTBC patch first",
			Patches:    patches,
		}, nil
	}

	// Fall back to HTML scraping.
	resp, err := HTTPClient.Get(searchURL)
	if err != nil {
		return &PatchSearchResult{
			Status:     "error",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("fetch issues: %v", err),
			Suggestion: "Retry later or check manually",
			Patches:    []PatchInfo{},
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &PatchSearchResult{
			Status:     "error",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("issues for %s: HTTP %d", query, resp.StatusCode),
			Suggestion: "Retry later or check manually",
			Patches:    []PatchInfo{},
		}, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &PatchSearchResult{
			Status:     "error",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("read issues: %v", err),
			Suggestion: "Retry later or check manually",
			Patches:    []PatchInfo{},
		}, nil
	}

	htmlPatches, err := parseIssueHTML(string(data))
	if err != nil {
		return &PatchSearchResult{
			Status:     "error",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("parse issues: %v", err),
			Suggestion: "Retry later or check manually",
			Patches:    []PatchInfo{},
		}, nil
	}

	if len(htmlPatches) > 0 {
		return &PatchSearchResult{
			Status:     "patches_found",
			Module:     query,
			Searched:   searchURL,
			Message:    fmt.Sprintf("%d patches found", len(htmlPatches)),
			Suggestion: "Apply highest-date RTBC patch first",
			Patches:    htmlPatches,
		}, nil
	}

	return &PatchSearchResult{
		Status:     "no_patches_found",
		Module:     query,
		Searched:   searchURL,
		Message:    "No patches found on Drupal.org",
		Suggestion: fmt.Sprintf("Create a custom patch or check issue queue manually at %s", searchURL),
		Patches:    []PatchInfo{},
	}, nil
}

func parseIssueHTML(html string) ([]PatchInfo, error) {
	var patches []PatchInfo

	// Simple HTML parsing — look for file links with .patch/.diff extensions.
	// This is a basic parser; a production version would use html.Parse.
	lines := strings.Split(html, "\n")
	for i, line := range lines {
		// Look for file links.
		if !strings.Contains(line, "href=\"/files/issue/") {
			continue
		}

		// Extract URL.
		urlStart := strings.Index(line, "href=\"")
		if urlStart == -1 {
			continue
		}
		urlStart += 6
		urlEnd := strings.Index(line[urlStart:], "\"")
		if urlEnd == -1 {
			continue
		}
		fileURL := "https://www.drupal.org" + line[urlStart:urlStart+urlEnd]

		// Check if it's a patch/diff file.
		isPatch := strings.HasSuffix(fileURL, ".patch") || strings.HasSuffix(fileURL, ".diff")
		isMR := strings.Contains(fileURL, "git.drupal.org")
		if !isPatch && !isMR {
			continue
		}

		// Try to find status in nearby lines.
		status := "Unknown"
		for j := max(0, i-5); j < min(len(lines), i+10); j++ {
			if strings.Contains(lines[j], "class=\"status\"") {
				statusStart := strings.Index(lines[j], "\">")
				if statusStart != -1 {
					statusEnd := strings.Index(lines[j][statusStart+2:], "</")
					if statusEnd != -1 {
						status = lines[j][statusStart+2 : statusStart+2+statusEnd]
					}
				}
				break
			}
		}

		// Try to find date in nearby lines.
		date := ""
		for j := max(0, i-5); j < min(len(lines), i+10); j++ {
			if strings.Contains(lines[j], "<td>") && looksLikeDate(lines[j]) {
				date = extractDate(lines[j])
				break
			}
		}

		patches = append(patches, PatchInfo{
			URL:     fileURL,
			Status:  status,
			Date:    date,
			IsPatch: isPatch,
		})
	}

	// Sort by RTBC priority.
	sort.Slice(patches, func(i, j int) bool {
		return priority(patches[i].Status) < priority(patches[j].Status)
	})

	return patches, nil
}

func priority(status string) int {
	switch status {
	case "RTBC":
		return 0
	case "Fixed":
		return 1
	case "Needs review":
		return 2
	case "Needs work":
		return 3
	default:
		return 4
	}
}

func looksLikeDate(line string) bool {
	// Crude check for date pattern like 2024-01-15.
	return strings.Contains(line, "20") && strings.Count(line, "-") >= 2
}

func extractDate(line string) string {
	// Extract date from <td>2024-01-15</td>.
	start := strings.Index(line, "<td>")
	if start == -1 {
		return ""
	}
	start += 4
	end := strings.Index(line[start:], "</td>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(line[start : start+end])
}

// Release represents a single release from Drupal.org release-history XML.
type Release struct {
	Version      string   `json:"version"`
	DrupalCompat []string `json:"drupal_compatibility"`
	ReleaseDate  string   `json:"release_date"`
	IsStable     bool     `json:"is_stable"`
}

// UpgradeRecommendation is the recommended version for a target Drupal major.
type UpgradeRecommendation struct {
	Module       string    `json:"module"`
	Recommended  *Release  `json:"recommended_upgrade"`
	Alternatives []Release `json:"alternative_versions"`
}

// releaseHistoryFull includes status and date for upgrade path parsing.
type releaseHistoryFull struct {
	XMLName  xml.Name      `xml:"project"`
	Name     string        `xml:"name"`
	Releases []releaseFull `xml:"releases>release"`
}

type releaseFull struct {
	Name        string `xml:"name"`
	Version     string `xml:"version"`
	Tag         string `xml:"tag"`
	Status      string `xml:"status"`
	ReleaseDate string `xml:"release_date"`
	Terms       []term `xml:"terms>term"`
}

// releaseHistoryVersionURL is the template for version-specific release-history lookups.
var releaseHistoryVersionURL = "https://updates.drupal.org/release-history/%s/%s"

// FetchReleaseHistory fetches the full release-history XML for a module at a specific Drupal version.
func FetchReleaseHistory(module, drupalVersion string) (*releaseHistoryFull, error) {
	url := fmt.Sprintf(releaseHistoryVersionURL, module, drupalVersion)
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch release history for %s/%s: %w", module, drupalVersion, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release history for %s/%s: HTTP %d", module, drupalVersion, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read release history: %w", err)
	}

	var rh releaseHistoryFull
	if err := xml.Unmarshal(data, &rh); err != nil {
		return nil, fmt.Errorf("parse release XML: %w", err)
	}
	return &rh, nil
}

// UpgradePath finds the recommended version for target Drupal major.
func UpgradePath(module, currentDrupal, targetDrupal string) (*UpgradeRecommendation, error) {
	rec := &UpgradeRecommendation{
		Module:       module,
		Alternatives: []Release{},
	}

	rh, err := FetchReleaseHistory(module, targetDrupal)
	if err != nil {
		return nil, err
	}

	// Fallback: try current version if target returns nothing.
	if rh == nil || len(rh.Releases) == 0 {
		rh, err = FetchReleaseHistory(module, currentDrupal)
		if err != nil {
			return nil, err
		}
		if rh == nil {
			return rec, nil
		}
	}

	var allReleases []Release
	for _, rel := range rh.Releases {
		r := Release{
			Version:     rel.Version,
			ReleaseDate: rel.ReleaseDate,
			IsStable:    rel.Status == "published",
		}
		for _, t := range rel.Terms {
			if t.Name == "Core compatibility" {
				r.DrupalCompat = append(r.DrupalCompat, t.Value)
			}
		}
		// Filter: must be compatible with target Drupal version.
		compatible := false
		targetKey := "Drupal " + targetDrupal
		for _, c := range r.DrupalCompat {
			if c == targetKey {
				compatible = true
				break
			}
		}
		if !compatible {
			continue
		}
		allReleases = append(allReleases, r)
	}

	if len(allReleases) == 0 {
		return rec, nil
	}

	// Sort by date descending (latest first).
	sort.Slice(allReleases, func(i, j int) bool {
		return allReleases[i].ReleaseDate > allReleases[j].ReleaseDate
	})

	// Prefer latest stable.
	for i, r := range allReleases {
		if r.IsStable {
			rec.Recommended = &allReleases[i]
			break
		}
	}
	// If no stable, use latest.
	if rec.Recommended == nil {
		rec.Recommended = &allReleases[0]
	}

	// Alternatives: up to 5, excluding the recommended.
	for _, r := range allReleases {
		if rec.Recommended != nil && r.Version == rec.Recommended.Version {
			continue
		}
		if len(rec.Alternatives) >= 5 {
			break
		}
		rec.Alternatives = append(rec.Alternatives, r)
	}

	return rec, nil
}

// ModuleMetadata holds module info from Drupal.org.
type ModuleMetadata struct {
	Module      string   `json:"module"`
	Title       string   `json:"title"`
	Maintainers []string `json:"maintainers"`
	Downloads   int      `json:"downloads"`
	LastRelease string   `json:"last_release"`
	OpenIssues  int      `json:"open_issues"`
}

// apiD7NodeFull is the full node detail from api-d7.
type apiD7NodeFull struct {
	NID            string       `json:"nid"`
	Title          string       `json:"title"`
	FieldDownloads int          `json:"field_download_count"`
	Maintainers    []maintainer `json:"maintainers"`
}

type maintainer struct {
	Name string `json:"name"`
}

// moduleNodeURL is the template for api-d7 node lookup by name.
var moduleNodeURL = "https://www.drupal.org/api-d7/node.json?name=%s"

// ModuleInfo fetches module metadata from Drupal.org.
func ModuleInfo(module string) (*ModuleMetadata, error) {
	meta := &ModuleMetadata{
		Module:      module,
		Maintainers: []string{},
	}

	// Fetch node data.
	url := fmt.Sprintf(moduleNodeURL, module)
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch module info for %s: %w", module, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("module %q not found", module)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("module info for %s: HTTP %d", module, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read module info: %w", err)
	}

	var node apiD7NodeFull
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse module info JSON: %w", err)
	}

	meta.Title = node.Title
	meta.Downloads = node.FieldDownloads
	for _, m := range node.Maintainers {
		meta.Maintainers = append(meta.Maintainers, m.Name)
	}

	// Fetch latest release from release-history.
	rh, err := FetchReleaseHistory(module, "current")
	if err == nil && rh != nil && len(rh.Releases) > 0 {
		meta.LastRelease = rh.Releases[0].Version
	}

	return meta, nil
}

// constraintMatchesDrupal checks if a core_version_requirement constraint
// (e.g., "^10.3 || ^11.0") is satisfied by the given Drupal major version.
func constraintMatchesDrupal(constraint string, drupalMajor int) bool {
	// Split on || for OR conditions
	parts := strings.Split(constraint, "||")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if matchesConstraint(part, drupalMajor) {
			return true
		}
	}
	return false
}

// matchesConstraint checks if a single constraint (without ||) matches the Drupal major version.
func matchesConstraint(constraint string, drupalMajor int) bool {
	constraint = strings.TrimSpace(constraint)

	// Handle caret constraints like ^10.3 or ^11.0
	if strings.HasPrefix(constraint, "^") {
		version := strings.TrimPrefix(constraint, "^")
		major, _ := parseMajor(version)
		return major == drupalMajor
	}

	// Handle range constraints like >=10 <12
	if strings.Contains(constraint, ">=") && strings.Contains(constraint, "<") {
		parts := strings.Fields(constraint)
		var minMajor, maxMajor int
		for _, part := range parts {
			if strings.HasPrefix(part, ">=") {
				minMajor, _ = parseMajor(strings.TrimPrefix(part, ">="))
			} else if strings.HasPrefix(part, "<") {
				maxMajor, _ = parseMajor(strings.TrimPrefix(part, "<"))
			}
		}
		return drupalMajor >= minMajor && drupalMajor < maxMajor
	}

	// Handle simple version like 11.0
	major, _ := parseMajor(constraint)
	return major == drupalMajor
}

// parseMajor extracts the major version from a version string like "10.3.0" or "11.0".
func parseMajor(version string) (int, error) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid version: %s", version)
	}
	return strconv.Atoi(parts[0])
}

// fetchInfoYML fetches the .info.yml file for a module from git.drupalcode.org.
func fetchInfoYML(module, branch string) (string, error) {
	url := fmt.Sprintf("https://git.drupalcode.org/project/%s/-/raw/%s/%s.info.yml", module, branch, module)
	resp, err := HTTPClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch info.yml for %s: %w", module, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("info.yml for %s/%s: HTTP %d", module, branch, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read info.yml: %w", err)
	}

	return string(data), nil
}

// parseCoreVersionRequirement extracts core_version_requirement from info.yml content.
func parseCoreVersionRequirement(infoYML string) string {
	lines := strings.Split(infoYML, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "core_version_requirement:") {
			value := strings.TrimPrefix(line, "core_version_requirement:")
			value = strings.TrimSpace(value)
			// Remove quotes if present
			value = strings.Trim(value, `"'`)
			return value
		}
	}
	return ""
}
