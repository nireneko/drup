package update

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
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

	version, assetURL, err := CheckLatest("owner", "repo")
	if err != nil {
		t.Fatalf("CheckLatest error: %v", err)
	}
	if version != "0.2.0" {
		t.Errorf("version = %q, want %q", version, "0.2.0")
	}
	if assetURL == "" {
		t.Error("assetURL is empty")
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
