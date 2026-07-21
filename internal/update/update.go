package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient is the HTTP client for GitHub API and downloads.
// Package-level var for testability.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// releasesURL is the template for GitHub Releases API.
// Package-level var for testability.
var releasesURL = "https://api.github.com/repos/%s/%s/releases/latest"

// githubRelease is the JSON response from GitHub Releases API.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckLatest checks GitHub Releases for the latest version.
// goos/goarch determine which asset to select (e.g. "linux", "amd64").
// Returns version (without "v" prefix), asset download URL, and error.
func CheckLatest(owner, repo, goos, goarch string) (version, assetURL string, err error) {
	url := fmt.Sprintf(releasesURL, owner, repo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return "", "", fmt.Errorf("parse release: %w", err)
	}

	version = strings.TrimPrefix(release.TagName, "v")

	// Filter assets by OS/arch pattern (infix: _{goos}_{goarch}.)
	pattern := fmt.Sprintf("_%s_%s.", goos, goarch)
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, pattern) {
			return version, asset.BrowserDownloadURL, nil
		}
	}

	return "", "", fmt.Errorf("no release asset found for %s/%s", goos, goarch)
}
