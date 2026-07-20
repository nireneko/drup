package drupalorg

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestSearchIssuesAPI(t *testing.T) {
	apiD7Response := `{
		"list": [
			{"node": {"nid": "12345", "title": "Fix D11 deprecation", "status": "RTBC"}},
			{"node": {"nid": "12346", "title": "Another issue", "status": "Needs review"}}
		],
		"next": ""
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(apiD7Response))
	}))
	defer srv.Close()

	orig := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = orig }()

	origBase := apiD7BaseURL
	apiD7BaseURL = srv.URL + "/api-d7/node.json?field_project_machine_name=%s"
	defer func() { apiD7BaseURL = origBase }()

	patches, err := SearchIssuesAPI("token")
	if err != nil {
		t.Fatalf("SearchIssuesAPI error: %v", err)
	}
	if len(patches) != 2 {
		t.Fatalf("len(patches) = %d, want 2", len(patches))
	}
	if patches[0].IssueNID != "12345" {
		t.Errorf("patches[0].IssueNID = %q, want %q", patches[0].IssueNID, "12345")
	}
	if patches[0].Status != "RTBC" {
		t.Errorf("patches[0].Status = %q, want %q", patches[0].Status, "RTBC")
	}
}

func TestSearchPatches_API_D7Primary(t *testing.T) {
	// When api-d7 returns results, HTML scraping should NOT be called.
	apiCalled := false
	htmlCalled := false

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"list": [{"node": {"nid": "99", "title": "Fix", "status": "Fixed"}}]}`))
	}))
	defer apiSrv.Close()

	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		htmlCalled = true
		w.Write([]byte("<html></html>"))
	}))
	defer htmlSrv.Close()

	orig := httpClient
	httpClient = apiSrv.Client()
	defer func() { httpClient = orig }()

	origAPI := apiD7BaseURL
	apiD7BaseURL = apiSrv.URL + "/api-d7/node.json?field_project_machine_name=%s"
	defer func() { apiD7BaseURL = origAPI }()

	origIssue := issueBaseURL
	issueBaseURL = htmlSrv.URL + "/project/issues/%s"
	defer func() { issueBaseURL = origIssue }()

	patches, err := SearchPatches("token")
	if err != nil {
		t.Fatalf("SearchPatches error: %v", err)
	}
	if !apiCalled {
		t.Error("api-d7 was not called")
	}
	if htmlCalled {
		t.Error("HTML scraping should not be called when api-d7 returns results")
	}
	if len(patches) != 1 {
		t.Fatalf("len(patches) = %d, want 1", len(patches))
	}
	if patches[0].IssueNID != "99" {
		t.Errorf("IssueNID = %q, want %q", patches[0].IssueNID, "99")
	}
}

func TestSearchPatches_FixtureHTML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "issue_with_patches.html"))
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "api-d7") {
			// Return empty api-d7 results so it falls back to HTML.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"list": []}`))
			return
		}
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

	origAPI := apiD7BaseURL
	apiD7BaseURL = srv.URL + "/api-d7/node.json?field_project_machine_name=%s"
	defer func() { apiD7BaseURL = origAPI }()

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
