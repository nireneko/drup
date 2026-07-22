package patchreconcile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/drupalorg"
)

// serveJSON starts an httptest server that always returns the given api-d7
// JSON body, and points drupalorg's exported HTTP seams at it for the
// duration of the test.
func serveJSON(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	origClient := drupalorg.HTTPClient
	drupalorg.HTTPClient = srv.Client()
	t.Cleanup(func() { drupalorg.HTTPClient = origClient })

	origBase := drupalorg.APID7BaseURL
	drupalorg.APID7BaseURL = srv.URL + "/api-d7/node.json?field_project_machine_name=%s"
	t.Cleanup(func() { drupalorg.APID7BaseURL = origBase })

	return srv
}

func TestReconcile_NewerPatchAvailable(t *testing.T) {
	serveJSON(t, `{
		"list": [
			{"node": {"nid": "111", "title": "Fix D11 deprecation", "status": "RTBC"}},
			{"node": {"nid": "222", "title": "Another issue", "status": "Needs review"}}
		],
		"next": ""
	}`)

	result, err := Reconcile("token", "https://www.drupal.org/node/222")
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if !result.IsStillNeeded {
		t.Error("expected IsStillNeeded=true when a newer patch is present and current is not merged")
	}
	if len(result.NewerPatches) != 1 {
		t.Fatalf("len(NewerPatches) = %d, want 1", len(result.NewerPatches))
	}
	if result.NewerPatches[0].IssueNID != "111" {
		t.Errorf("NewerPatches[0].IssueNID = %q, want %q", result.NewerPatches[0].IssueNID, "111")
	}
	if !strings.Contains(result.Recommendation, "review") {
		t.Errorf("Recommendation = %q, want it to mention review", result.Recommendation)
	}
}

func TestReconcile_ObsoleteWhenMerged(t *testing.T) {
	serveJSON(t, `{
		"list": [
			{"node": {"nid": "333", "title": "Fixed upstream", "status": "Fixed"}}
		],
		"next": ""
	}`)

	result, err := Reconcile("token", "https://www.drupal.org/node/333")
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if result.IsStillNeeded {
		t.Error("expected IsStillNeeded=false when the current patch's issue is Fixed upstream")
	}
	if len(result.NewerPatches) != 0 {
		t.Errorf("len(NewerPatches) = %d, want 0", len(result.NewerPatches))
	}
	if !strings.Contains(result.Recommendation, "remove") {
		t.Errorf("Recommendation = %q, want it to mention remove", result.Recommendation)
	}
}

func TestReconcile_StillNeeded_NoNewerPatch(t *testing.T) {
	serveJSON(t, `{
		"list": [
			{"node": {"nid": "444", "title": "Needs work", "status": "Needs work"}}
		],
		"next": ""
	}`)

	result, err := Reconcile("token", "https://www.drupal.org/node/444")
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if !result.IsStillNeeded {
		t.Error("expected IsStillNeeded=true when no newer patch exists and current is not merged")
	}
	if len(result.NewerPatches) != 0 {
		t.Errorf("len(NewerPatches) = %d, want 0", len(result.NewerPatches))
	}
	if !strings.Contains(result.Recommendation, "keep") {
		t.Errorf("Recommendation = %q, want it to mention keep", result.Recommendation)
	}
}

func TestReconcile_MissingModule(t *testing.T) {
	_, err := Reconcile("", "https://www.drupal.org/node/1")
	if err == nil {
		t.Fatal("expected error for empty module, got nil")
	}
}

func TestReconcile_MissingPatchURL(t *testing.T) {
	_, err := Reconcile("token", "")
	if err == nil {
		t.Fatal("expected error for empty current_patch_url, got nil")
	}
}
