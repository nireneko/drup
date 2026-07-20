package drupalorg

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// httpClient is the HTTP client for Drupal.org requests.
// Package-level var for testability.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// releaseBaseURL is the template for release-history lookups.
// Package-level var for testability.
var releaseBaseURL = "https://updates.drupal.org/release-history/%s/current"

// issueBaseURL is the template for issue queue scraping.
// Package-level var for testability.
var issueBaseURL = "https://www.drupal.org/project/issues/%s"

// apiD7BaseURL is the template for api-d7 node queries.
// Package-level var for testability.
var apiD7BaseURL = "https://www.drupal.org/api-d7/node.json?field_project_machine_name=%s"

// ReleaseInfo contains D11 compatibility data for a module.
type ReleaseInfo struct {
	Module   string   `json:"module"`
	HasD11   bool     `json:"has_d11_release"`
	Latest   string   `json:"latest_version"`
	Branches []string `json:"compatible_branches"`
}

// PatchInfo contains data about a patch/diff/MR from an issue.
type PatchInfo struct {
	URL     string `json:"url"`
	Status  string `json:"status"`
	Date    string `json:"date"`
	IsPatch bool   `json:"is_patch"`
	IssueNID string `json:"issue_nid"`
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
	resp, err := httpClient.Get(url)
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

	return info, nil
}

// SearchIssuesAPI queries the Drupal api-d7 endpoint for issue nodes.
// Returns patch info entries extracted from the API response.
func SearchIssuesAPI(module string) ([]PatchInfo, error) {
	url := fmt.Sprintf(apiD7BaseURL, module)
	resp, err := httpClient.Get(url)
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
// Results are sorted by RTBC priority.
func SearchPatches(query string) ([]PatchInfo, error) {
	// Try api-d7 first.
	patches, err := SearchIssuesAPI(query)
	if err == nil && len(patches) > 0 {
		sort.Slice(patches, func(i, j int) bool {
			return priority(patches[i].Status) < priority(patches[j].Status)
		})
		return patches, nil
	}

	// Fall back to HTML scraping.
	url := fmt.Sprintf(issueBaseURL, query)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch issues for %s: %w", query, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("issues for %s: HTTP %d", query, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read issues: %w", err)
	}

	return parseIssueHTML(string(data))
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


