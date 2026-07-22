package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tarEntry describes one entry to write into a fake .tar.gz fixture.
type tarEntry struct {
	name     string
	content  []byte
	typeflag byte
	linkname string
}

// makeTarGz builds a .tar.gz in memory from entries and returns its bytes.
func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o755,
			Size:     int64(len(e.content)),
			Typeflag: typeflag,
			Linkname: e.linkname,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.name, err)
		}
		if len(e.content) > 0 {
			if _, err := tw.Write(e.content); err != nil {
				t.Fatalf("write tar content %q: %v", e.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// --- ResolveArchiveName / ResolveAssetURL / ResolveChecksumURL ---

func TestResolveArchiveName(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "linux amd64",
			repo:    "drup",
			version: "0.2.0",
			goos:    "linux",
			goarch:  "amd64",
			want:    "drup_0.2.0_linux_amd64.tar.gz",
		},
		{
			name:    "linux arm64",
			repo:    "drup",
			version: "1.0.0",
			goos:    "linux",
			goarch:  "arm64",
			want:    "drup_1.0.0_linux_arm64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveArchiveName(tt.repo, tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("ResolveArchiveName(%s, %s, %s, %s) = %q, want %q",
					tt.repo, tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

func TestResolveAssetURL(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "linux amd64",
			owner:   "nireneko",
			repo:    "drup",
			version: "0.2.0",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://github.com/nireneko/drup/releases/download/v0.2.0/drup_0.2.0_linux_amd64.tar.gz",
		},
		{
			name:    "linux arm64",
			owner:   "nireneko",
			repo:    "drup",
			version: "1.0.0",
			goos:    "linux",
			goarch:  "arm64",
			want:    "https://github.com/nireneko/drup/releases/download/v1.0.0/drup_1.0.0_linux_arm64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAssetURL(tt.owner, tt.repo, tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("ResolveAssetURL(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveChecksumURL(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		version string
		want    string
	}{
		{
			name:    "standard release",
			owner:   "nireneko",
			repo:    "drup",
			version: "0.2.0",
			want:    "https://github.com/nireneko/drup/releases/download/v0.2.0/checksums.txt",
		},
		{
			name:    "different version",
			owner:   "nireneko",
			repo:    "drup",
			version: "1.5.3",
			want:    "https://github.com/nireneko/drup/releases/download/v1.5.3/checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveChecksumURL(tt.owner, tt.repo, tt.version)
			if got != tt.want {
				t.Errorf("ResolveChecksumURL(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Download ---

// TestUpgradeDownload_ChecksumVerification exercises the four checksum
// failure modes for the new Download signature: match, mismatch, missing
// checksums.txt, and archive not listed.
func TestUpgradeDownload_ChecksumVerification(t *testing.T) {
	archiveContent := []byte("fake archive content")
	archiveName := "drup_0.2.0_linux_amd64.tar.gz"

	digest := sha256.Sum256(archiveContent)
	realDigest := hex.EncodeToString(digest[:])

	tests := []struct {
		name            string
		checksumsBody   string
		checksumsStatus int
		wantErr         bool
		errContains     string
	}{
		{
			name:            "matching checksum succeeds",
			checksumsBody:   realDigest + "  " + archiveName + "\n",
			checksumsStatus: http.StatusOK,
			wantErr:         false,
		},
		{
			name:            "checksum mismatch returns error",
			checksumsBody:   "deadbeefdeadbeef  " + archiveName + "\n",
			checksumsStatus: http.StatusOK,
			wantErr:         true,
			errContains:     "checksum mismatch",
		},
		{
			name:            "missing checksums.txt returns error",
			checksumsBody:   "",
			checksumsStatus: http.StatusNotFound,
			wantErr:         true,
			errContains:     "checksums.txt unavailable",
		},
		{
			name:            "archive not listed returns error",
			checksumsBody:   "abc123  other-tool_0.2.0_linux_amd64.tar.gz\n",
			checksumsStatus: http.StatusOK,
			wantErr:         true,
			errContains:     "not listed in checksums.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/asset":
					w.WriteHeader(http.StatusOK)
					w.Write(archiveContent)
				case "/checksums.txt":
					w.WriteHeader(tt.checksumsStatus)
					w.Write([]byte(tt.checksumsBody))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			origClient := httpClient
			httpClient = srv.Client()
			defer func() { httpClient = origClient }()

			archivePath := filepath.Join(t.TempDir(), archiveName)
			err := Download(srv.URL+"/asset", srv.URL+"/checksums.txt", archiveName, archivePath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Download() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Download() error = %v, want containing %q", err, tt.errContains)
				}
			}
			if tt.wantErr {
				if _, statErr := os.Stat(archivePath); statErr == nil {
					t.Errorf("archivePath %s should be removed on verification failure", archivePath)
				}
			} else {
				if _, statErr := os.Stat(archivePath); statErr != nil {
					t.Errorf("archivePath %s should exist after successful download: %v", archivePath, statErr)
				}
			}
		})
	}
}

// --- ExtractBinaryFromTarGz ---

func TestExtractBinaryFromTarGz(t *testing.T) {
	rootContent := []byte("#!/bin/sh\necho root binary")
	nestedContent := []byte("#!/bin/sh\necho nested binary")

	tests := []struct {
		name        string
		entries     []tarEntry
		binaryName  string
		wantContent []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "binary at archive root",
			entries: []tarEntry{
				{name: "README.md", content: []byte("readme")},
				{name: "drup", content: rootContent},
			},
			binaryName:  "drup",
			wantContent: rootContent,
		},
		{
			name: "binary nested in a subdirectory",
			entries: []tarEntry{
				{name: "README.md", content: []byte("readme")},
				{name: "drup-linux-amd64/drup", content: nestedContent},
			},
			binaryName:  "drup",
			wantContent: nestedContent,
		},
		{
			name: "symlink entry matching basename is rejected",
			entries: []tarEntry{
				{name: "drup", typeflag: tar.TypeSymlink, linkname: "/usr/bin/drup"},
			},
			binaryName:  "drup",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "binary not found returns error",
			entries: []tarEntry{
				{name: "README.md", content: []byte("readme")},
			},
			binaryName:  "drup",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archiveBytes := makeTarGz(t, tt.entries)
			outPath := filepath.Join(t.TempDir(), "drup")

			err := ExtractBinaryFromTarGz(bytes.NewReader(archiveBytes), tt.binaryName, outPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractBinaryFromTarGz() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ExtractBinaryFromTarGz() error = %v, want containing %q", err, tt.errContains)
				}
			}

			if tt.wantErr {
				if _, statErr := os.Stat(outPath); statErr == nil {
					t.Errorf("outPath %s should not exist after extraction failure", outPath)
				}
				return
			}

			got, readErr := os.ReadFile(outPath)
			if readErr != nil {
				t.Fatalf("read extracted binary: %v", readErr)
			}
			if !bytes.Equal(got, tt.wantContent) {
				t.Errorf("extracted content = %q, want %q", got, tt.wantContent)
			}

			info, statErr := os.Stat(outPath)
			if statErr != nil {
				t.Fatalf("stat extracted binary: %v", statErr)
			}
			if info.Mode()&0o111 == 0 {
				t.Errorf("extracted binary mode = %v, want executable bit set", info.Mode())
			}
		})
	}
}

// --- atomicReplace ---

func TestAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dst := filepath.Join(dir, "existing-binary")

	if err := os.WriteFile(src, []byte("new content"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old content"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := atomicReplace(src, dst); err != nil {
		t.Fatalf("atomicReplace: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst after replace: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("dst content = %q, want %q", content, "new content")
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file should no longer exist after atomic replace")
	}
}

func TestAtomicReplace_MissingSourceReturnsError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "does-not-exist")
	dst := filepath.Join(dir, "existing-binary")

	if err := os.WriteFile(dst, []byte("old content"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := atomicReplace(src, dst); err == nil {
		t.Fatal("expected error for missing source, got nil")
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(content) != "old content" {
		t.Errorf("dst content = %q, want untouched %q", content, "old content")
	}
}

// --- ReplaceBinary ---

func TestReplaceBinary_Success(t *testing.T) {
	binDir := t.TempDir()
	backupDir := t.TempDir()

	currentBin := filepath.Join(binDir, "drup")
	newBinaryPath := filepath.Join(binDir, "drup.new")
	backupPath := filepath.Join(backupDir, "drup.bak")

	if err := os.WriteFile(currentBin, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}
	if err := os.WriteFile(newBinaryPath, []byte("new binary"), 0o755); err != nil {
		t.Fatalf("write newBinaryPath: %v", err)
	}

	if err := ReplaceBinary(newBinaryPath, currentBin, backupPath); err != nil {
		t.Fatalf("ReplaceBinary: %v", err)
	}

	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read currentBin: %v", err)
	}
	if string(got) != "new binary" {
		t.Errorf("currentBin content = %q, want %q", got, "new binary")
	}

	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backupPath: %v", err)
	}
	if string(backup) != "old binary" {
		t.Errorf("backupPath content = %q, want %q", backup, "old binary")
	}
}

func TestReplaceBinary_ReplaceFailsRestoreSucceeds(t *testing.T) {
	binDir := t.TempDir()
	backupDir := t.TempDir()

	currentBin := filepath.Join(binDir, "drup")
	// newBinaryPath is intentionally never created, forcing atomicReplace to fail.
	newBinaryPath := filepath.Join(binDir, "drup.new")
	backupPath := filepath.Join(backupDir, "drup.bak")

	if err := os.WriteFile(currentBin, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	err := ReplaceBinary(newBinaryPath, currentBin, backupPath)
	if err == nil {
		t.Fatal("expected error when new binary is missing, got nil")
	}
	if !strings.Contains(err.Error(), backupPath) {
		t.Errorf("error = %v, want it to mention backup path %q", err, backupPath)
	}

	got, readErr := os.ReadFile(currentBin)
	if readErr != nil {
		t.Fatalf("read currentBin after failed replace: %v", readErr)
	}
	if string(got) != "old binary" {
		t.Errorf("currentBin content = %q, want restored %q", got, "old binary")
	}

	info, statErr := os.Stat(currentBin)
	if statErr != nil {
		t.Fatalf("stat currentBin: %v", statErr)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("currentBin mode = %v, want executable bit set after restore", info.Mode())
	}
}

func TestReplaceBinary_ReplaceFailsRestoreAlsoFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — permission-based failure injection does not apply")
	}

	binDir := t.TempDir()
	backupDir := t.TempDir()

	currentBin := filepath.Join(binDir, "drup")
	// newBinaryPath is intentionally never created, forcing atomicReplace to fail.
	newBinaryPath := filepath.Join(binDir, "drup.new")
	backupPath := filepath.Join(backupDir, "drup.bak")

	if err := os.WriteFile(currentBin, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	// Remove write permission on binDir so RestoreBinary's rename-into-place
	// step fails too, after BackupBinary has already succeeded.
	if err := os.Chmod(binDir, 0o555); err != nil {
		t.Fatalf("chmod binDir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(binDir, 0o755) })

	err := ReplaceBinary(newBinaryPath, currentBin, backupPath)
	if err == nil {
		t.Fatal("expected error when both replace and restore fail, got nil")
	}
	if !strings.Contains(err.Error(), backupPath) {
		t.Errorf("error = %v, want it to mention backup path %q", err, backupPath)
	}
	// Both the original replace failure and the restore failure must be reported.
	if !strings.Contains(err.Error(), "restore") {
		t.Errorf("error = %v, want it to mention the restore failure", err)
	}
}

// --- Upgrade ---

// TestUpgrade_FullFlow exercises the complete Upgrade() orchestration via the
// httpClient/executableFn/homeDirFn/resolveAssetURLFn/resolveChecksumURLFn
// package-var seams: resolve URLs, download+verify, extract, and replace.
func TestUpgrade_FullFlow(t *testing.T) {
	binaryName := "drup"
	newContent := []byte("#!/bin/sh\necho new version")
	archiveBytes := makeTarGz(t, []tarEntry{
		{name: binaryName + "_1.0.0_linux_amd64/" + binaryName, content: newContent},
	})

	digest := sha256.Sum256(archiveBytes)
	archiveName := "drup_1.0.0_linux_amd64.tar.gz"
	checksumsBody := hex.EncodeToString(digest[:]) + "  " + archiveName + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.WriteHeader(http.StatusOK)
			w.Write(archiveBytes)
		case "/checksums.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	origAssetURLFn := resolveAssetURLFn
	origChecksumURLFn := resolveChecksumURLFn
	defer func() {
		resolveAssetURLFn = origAssetURLFn
		resolveChecksumURLFn = origChecksumURLFn
	}()
	resolveAssetURLFn = func(owner, repo, version, goos, goarch string) string {
		return srv.URL + "/asset"
	}
	resolveChecksumURLFn = func(owner, repo, version string) string {
		return srv.URL + "/checksums.txt"
	}

	binDir := t.TempDir()
	currentBin := filepath.Join(binDir, binaryName)
	if err := os.WriteFile(currentBin, []byte("old version"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	origExecutableFn := executableFn
	defer func() { executableFn = origExecutableFn }()
	executableFn = func() (string, error) { return currentBin, nil }

	homeDir := t.TempDir()
	origHomeDirFn := homeDirFn
	defer func() { homeDirFn = origHomeDirFn }()
	homeDirFn = func() (string, error) { return homeDir, nil }

	opts := UpgradeOptions{
		Owner:   "nireneko",
		Repo:    "drup",
		Binary:  binaryName,
		Version: "1.0.0",
	}

	if err := Upgrade(opts); err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read currentBin after upgrade: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("currentBin content = %q, want %q", got, newContent)
	}

	backupPath := filepath.Join(homeDir, ".drup", "backups", binaryName+".bak")
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "old version" {
		t.Errorf("backup content = %q, want %q", backup, "old version")
	}
}

// TestUpgrade_IgnoresGOOSGOARCHEnvOverride verifies that Upgrade resolves the
// archive name from the running binary's actual runtime.GOOS/runtime.GOARCH,
// even when GOOS/GOARCH environment variables are set to different, fake
// values. checksums.txt on the fake server is built for the real platform
// only; if Upgrade regressed to reading os.Getenv("GOOS"/"GOARCH"), it would
// compute a mismatched archive name and fail checksum lookup.
func TestUpgrade_IgnoresGOOSGOARCHEnvOverride(t *testing.T) {
	fakeGOOS := "plan9"
	if fakeGOOS == runtime.GOOS {
		fakeGOOS = "solaris"
	}
	fakeGOARCH := "386"
	if fakeGOARCH == runtime.GOARCH {
		fakeGOARCH = "mips"
	}
	t.Setenv("GOOS", fakeGOOS)
	t.Setenv("GOARCH", fakeGOARCH)

	binaryName := "drup"
	newContent := []byte("#!/bin/sh\necho new version")
	realArchiveName := ResolveArchiveName("drup", "1.0.0", runtime.GOOS, runtime.GOARCH)
	archiveBytes := makeTarGz(t, []tarEntry{
		{name: binaryName + "_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + "/" + binaryName, content: newContent},
	})

	digest := sha256.Sum256(archiveBytes)
	checksumsBody := hex.EncodeToString(digest[:]) + "  " + realArchiveName + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.WriteHeader(http.StatusOK)
			w.Write(archiveBytes)
		case "/checksums.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	origAssetURLFn := resolveAssetURLFn
	origChecksumURLFn := resolveChecksumURLFn
	defer func() {
		resolveAssetURLFn = origAssetURLFn
		resolveChecksumURLFn = origChecksumURLFn
	}()
	resolveAssetURLFn = func(owner, repo, version, goos, goarch string) string {
		return srv.URL + "/asset"
	}
	resolveChecksumURLFn = func(owner, repo, version string) string {
		return srv.URL + "/checksums.txt"
	}

	binDir := t.TempDir()
	currentBin := filepath.Join(binDir, binaryName)
	if err := os.WriteFile(currentBin, []byte("old version"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	origExecutableFn := executableFn
	defer func() { executableFn = origExecutableFn }()
	executableFn = func() (string, error) { return currentBin, nil }

	homeDir := t.TempDir()
	origHomeDirFn := homeDirFn
	defer func() { homeDirFn = origHomeDirFn }()
	homeDirFn = func() (string, error) { return homeDir, nil }

	opts := UpgradeOptions{
		Owner:   "nireneko",
		Repo:    "drup",
		Binary:  binaryName,
		Version: "1.0.0",
	}

	if err := Upgrade(opts); err != nil {
		t.Fatalf("Upgrade() error = %v (want success — Upgrade must ignore GOOS=%s/GOARCH=%s env overrides and resolve the archive using runtime.GOOS=%s/runtime.GOARCH=%s)",
			err, fakeGOOS, fakeGOARCH, runtime.GOOS, runtime.GOARCH)
	}

	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read currentBin after upgrade: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("currentBin content = %q, want %q", got, newContent)
	}
}

// TestUpgrade_DownloadErrorPropagates verifies that a Download failure (e.g.
// checksum mismatch) aborts the flow and is reported, without touching
// currentBin.
func TestUpgrade_DownloadErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("some archive bytes"))
		case "/checksums.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("deadbeef  drup_1.0.0_linux_amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	origAssetURLFn := resolveAssetURLFn
	origChecksumURLFn := resolveChecksumURLFn
	defer func() {
		resolveAssetURLFn = origAssetURLFn
		resolveChecksumURLFn = origChecksumURLFn
	}()
	resolveAssetURLFn = func(owner, repo, version, goos, goarch string) string {
		return srv.URL + "/asset"
	}
	resolveChecksumURLFn = func(owner, repo, version string) string {
		return srv.URL + "/checksums.txt"
	}

	binDir := t.TempDir()
	currentBin := filepath.Join(binDir, "drup")
	if err := os.WriteFile(currentBin, []byte("old version"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	origExecutableFn := executableFn
	defer func() { executableFn = origExecutableFn }()
	executableFn = func() (string, error) { return currentBin, nil }

	homeDir := t.TempDir()
	origHomeDirFn := homeDirFn
	defer func() { homeDirFn = origHomeDirFn }()
	homeDirFn = func() (string, error) { return homeDir, nil }

	opts := UpgradeOptions{Owner: "nireneko", Repo: "drup", Binary: "drup", Version: "1.0.0"}

	err := Upgrade(opts)
	if err == nil {
		t.Fatal("expected error from checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "download update") {
		t.Errorf("error = %v, want it to mention download failure", err)
	}

	got, readErr := os.ReadFile(currentBin)
	if readErr != nil {
		t.Fatalf("read currentBin: %v", readErr)
	}
	if string(got) != "old version" {
		t.Errorf("currentBin content = %q, want untouched %q", got, "old version")
	}
}

// TestBackupBinary_WritesToHomeBackupsDir verifies that BackupBinary copies the
// current binary to the specified backup path (which in production is
// ~/.drup/backups/drup.bak), NOT adjacent to the source binary.
func TestBackupBinary_WritesToHomeBackupsDir(t *testing.T) {
	homeDir := t.TempDir()
	binDir := t.TempDir()

	currentBin := filepath.Join(binDir, "drup")
	if err := os.WriteFile(currentBin, []byte("#!/bin/sh\necho binary"), 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}

	// Backup path simulates ~/.drup/backups/drup.bak.
	backupDir := filepath.Join(homeDir, ".drup", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backupDir: %v", err)
	}
	backupPath := filepath.Join(backupDir, "drup.bak")

	if err := BackupBinary(currentBin, backupPath); err != nil {
		t.Fatalf("BackupBinary: %v", err)
	}

	// Verify backup exists at the home-based path.
	got, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != "#!/bin/sh\necho binary" {
		t.Errorf("backup content = %q, want %q", got, "#!/bin/sh\necho binary")
	}

	// Verify NO adjacent backup exists next to source binary.
	adjacentBackup := filepath.Join(binDir, "drup.bak")
	if _, err := os.Stat(adjacentBackup); err == nil {
		t.Errorf("adjacent backup %s should NOT exist — backup must go to ~/.drup/backups/", adjacentBackup)
	}

	// Verify backup preserves executable bit.
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("backup mode = %v, want executable bit set", info.Mode())
	}
}

// TestReplaceBinary_CrossDeviceCopy verifies that ReplaceBinary correctly
// handles the case where the backup path is on a different filesystem than
// the binary. BackupBinary uses copyFile (not rename) so it works across
// device boundaries.
func TestReplaceBinary_CrossDeviceCopy(t *testing.T) {
	// Use two separate TempDirs to simulate different filesystems.
	// (On most systems these are on the same FS, but copyFile works
	// regardless — the key test is that content is correctly copied.)
	binDir := t.TempDir()
	backupDir := t.TempDir()

	currentBin := filepath.Join(binDir, "drup")
	newBinaryPath := filepath.Join(binDir, "drup.new")
	backupPath := filepath.Join(backupDir, "drup.bak")

	oldContent := []byte("#!/bin/sh\necho old version")
	newContent := []byte("#!/bin/sh\necho new version")

	if err := os.WriteFile(currentBin, oldContent, 0o755); err != nil {
		t.Fatalf("write currentBin: %v", err)
	}
	if err := os.WriteFile(newBinaryPath, newContent, 0o755); err != nil {
		t.Fatalf("write newBinaryPath: %v", err)
	}

	if err := ReplaceBinary(newBinaryPath, currentBin, backupPath); err != nil {
		t.Fatalf("ReplaceBinary: %v", err)
	}

	// Verify current binary has new content.
	got, err := os.ReadFile(currentBin)
	if err != nil {
		t.Fatalf("read currentBin: %v", err)
	}
	if string(got) != string(newContent) {
		t.Errorf("currentBin content = %q, want %q", got, newContent)
	}

	// Verify backup has old content on the separate directory.
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != string(oldContent) {
		t.Errorf("backup content = %q, want %q", backup, oldContent)
	}

	// Verify new binary was consumed (moved).
	if _, err := os.Stat(newBinaryPath); !os.IsNotExist(err) {
		t.Errorf("newBinaryPath %s should no longer exist after replace", newBinaryPath)
	}
}
