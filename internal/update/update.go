package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

// Download downloads a binary from url, verifies its SHA256 checksum against
// checksumURL, and returns the path to the downloaded file.
func Download(url, checksumURL, expectedFilename string) (string, error) {
	// Download the binary.
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download binary: HTTP %d", resp.StatusCode)
	}

	// Read and hash the binary.
	h := sha256.New()
	tmpFilePattern := "drup-update-*"
	if strings.HasSuffix(expectedFilename, ".tar.gz") {
		tmpFilePattern += ".tar.gz"
	} else if strings.HasSuffix(expectedFilename, ".zip") {
		tmpFilePattern += ".zip"
	}
	tmpFile, err := os.CreateTemp("", tmpFilePattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(io.MultiWriter(tmpFile, h), resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write binary: %w", err)
	}

	actualDigest := hex.EncodeToString(h.Sum(nil))

	// Fetch checksums.txt.
	checksumResp, err := httpClient.Get(checksumURL)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer checksumResp.Body.Close()

	checksumData, err := io.ReadAll(checksumResp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("read checksums: %w", err)
	}

	// Find expected checksum.
	expectedDigest, err := findChecksum(string(checksumData), expectedFilename)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("checksum verification failed: %w", err)
	}

	if actualDigest != expectedDigest {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedDigest, actualDigest)
	}

	return tmpFile.Name(), nil
}

func findChecksum(content, filename string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%q not found in checksums.txt", filename)
}
