package drupalorg

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestCheckRelease_HasD11(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "release_d11.xml"))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(data)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = orig }()

	// Override the base URL.
	origBase := releaseBaseURL
	releaseBaseURL = srv.URL + "/release-history/%s/current"
	defer func() { releaseBaseURL = origBase }()

	info, err := CheckRelease("token")
	if err != nil {
		t.Fatalf("CheckRelease error: %v", err)
	}
	if !info.HasD11 {
		t.Error("expected HasD11=true, got false")
	}
	if info.Module != "token" {
		t.Errorf("Module = %q, want %q", info.Module, "token")
	}
	if info.Latest == "" {
		t.Error("Latest is empty")
	}
}

func TestCheckRelease_NoD11(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "release_no_d11.xml"))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(data)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = orig }()

	origBase := releaseBaseURL
	releaseBaseURL = srv.URL + "/release-history/%s/current"
	defer func() { releaseBaseURL = origBase }()

	info, err := CheckRelease("oldmodule")
	if err != nil {
		t.Fatalf("CheckRelease error: %v", err)
	}
	if info.HasD11 {
		t.Error("expected HasD11=false, got true")
	}
}

func TestSearchPatches_FixtureHTML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "issue_with_patches.html"))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = orig }()

	origBase := issueBaseURL
	issueBaseURL = srv.URL + "/project/issues/%s"
	defer func() { issueBaseURL = origBase }()

	patches, err := SearchPatches("token")
	if err != nil {
		t.Fatalf("SearchPatches error: %v", err)
	}

	// Should find 3 patches (.patch and .diff), not the .png
	if len(patches) != 3 {
		t.Fatalf("len(patches) = %d, want 3", len(patches))
	}

	// First should be RTBC (highest priority).
	if patches[0].Status != "RTBC" {
		t.Errorf("first patch status = %q, want RTBC", patches[0].Status)
	}

	// Check that all have is_patch=true for .patch/.diff.
	for _, p := range patches {
		if !p.IsPatch {
			t.Errorf("patch %q should have is_patch=true", p.URL)
		}
	}
}
