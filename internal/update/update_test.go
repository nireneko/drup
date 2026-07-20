package update

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"assets": [
				{"name": "drup_0.2.0_linux_amd64.tar.gz", "browser_download_url": "http://example.com/drup_0.2.0_linux_amd64.tar.gz"}
			]
		}`))
	}))
	defer srv.Close()

	orig := releasesURL
	releasesURL = srv.URL + "/repos/%s/%s/releases/latest"
	defer func() { releasesURL = orig }()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	version, assetURL, err := CheckLatest("owner", "repo", "linux", "amd64")
	if err != nil {
		t.Fatalf("CheckLatest error: %v", err)
	}
	if version != "0.2.0" {
		t.Errorf("version = %q, want %q", version, "0.2.0")
	}
	if assetURL != "http://example.com/drup_0.2.0_linux_amd64.tar.gz" {
		t.Errorf("assetURL = %q, want linux_amd64 URL", assetURL)
	}
}

func TestCheckLatest_PlatformFilter(t *testing.T) {
	multiAssetJSON := `{
		"tag_name": "v0.2.0",
		"assets": [
			{"name": "drup_0.2.0_linux_amd64.tar.gz", "browser_download_url": "http://example.com/drup_0.2.0_linux_amd64.tar.gz"},
			{"name": "drup_0.2.0_linux_arm64.tar.gz", "browser_download_url": "http://example.com/drup_0.2.0_linux_arm64.tar.gz"},
			{"name": "drup_0.2.0_darwin_amd64.tar.gz", "browser_download_url": "http://example.com/drup_0.2.0_darwin_amd64.tar.gz"},
			{"name": "drup_0.2.0_darwin_arm64.tar.gz", "browser_download_url": "http://example.com/drup_0.2.0_darwin_arm64.tar.gz"},
			{"name": "drup_0.2.0_windows_amd64.zip", "browser_download_url": "http://example.com/drup_0.2.0_windows_amd64.zip"},
			{"name": "drup_0.2.0_windows_arm64.zip", "browser_download_url": "http://example.com/drup_0.2.0_windows_arm64.zip"}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(multiAssetJSON))
	}))
	defer srv.Close()

	orig := releasesURL
	releasesURL = srv.URL + "/repos/%s/%s/releases/latest"
	defer func() { releasesURL = orig }()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	tests := []struct {
		name        string
		goos        string
		goarch      string
		wantURL     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "linux/amd64 matches tar.gz",
			goos:    "linux",
			goarch:  "amd64",
			wantURL: "http://example.com/drup_0.2.0_linux_amd64.tar.gz",
		},
		{
			name:    "darwin/arm64 matches tar.gz",
			goos:    "darwin",
			goarch:  "arm64",
			wantURL: "http://example.com/drup_0.2.0_darwin_arm64.tar.gz",
		},
		{
			name:    "windows/amd64 matches zip",
			goos:    "windows",
			goarch:  "amd64",
			wantURL: "http://example.com/drup_0.2.0_windows_amd64.zip",
		},
		{
			name:        "no match returns error",
			goos:        "freebsd",
			goarch:      "mips",
			wantErr:     true,
			errContains: "no release asset found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, assetURL, err := CheckLatest("owner", "repo", tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if version != "0.2.0" {
				t.Errorf("version = %q, want %q", version, "0.2.0")
			}
			if assetURL != tt.wantURL {
				t.Errorf("assetURL = %q, want %q", assetURL, tt.wantURL)
			}
		})
	}
}

func TestDownload_AndVerify(t *testing.T) {
	// Create a fake binary and its checksum.
	fakeBinary := []byte("fake binary content")
	h := sha256.Sum256(fakeBinary)
	checksum := hex.EncodeToString(h[:])

	checksumsContent := checksum + "  drup_0.2.0_linux_amd64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Write(fakeBinary)
		case "/checksums.txt":
			w.Write([]byte(checksumsContent))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	// Download and verify.
	path, err := Download(srv.URL+"/binary", srv.URL+"/checksums.txt", "drup_0.2.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if path == "" {
		t.Error("path is empty")
	}
	if !strings.HasSuffix(path, ".tar.gz") {
		t.Errorf("temp file = %q, want .tar.gz suffix", path)
	}
}

func TestDownload_ChecksumMismatch(t *testing.T) {
	fakeBinary := []byte("fake binary content")
	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"
	checksumsContent := wrongChecksum + "  drup_0.2.0_linux_amd64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.Write(fakeBinary)
		case "/checksums.txt":
			w.Write([]byte(checksumsContent))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	_, err := Download(srv.URL+"/binary", srv.URL+"/checksums.txt", "drup_0.2.0_linux_amd64.tar.gz")
	if err == nil {
		t.Error("expected checksum mismatch error, got nil")
	}
}
