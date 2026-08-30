package patch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0o644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestApply_Success(t *testing.T) {
	// Create a mock HTTP server serving a patch.
	patchContent := `diff --git a/test.txt b/test.txt
index 5626abf..f9c9a4a 100644
--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-hello
+hello world
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(patchContent))
	}))
	defer srv.Close()

	// Create a temp git repo with the file to patch.
	dir := t.TempDir()
	initGitRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello\n"), 0o644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "add test.txt")

	// Create a minimal composer.json.
	composer := map[string]interface{}{
		"name": "drupal/test",
		"extra": map[string]interface{}{
			"patches": map[string]interface{}{},
		},
	}
	data, _ := json.MarshalIndent(composer, "", "  ")
	os.WriteFile(filepath.Join(dir, "composer.json"), data, 0o644)

	// Override HTTP client and allowlist for testing.
	origClient := httpClient
	httpClient = srv.Client()
	defer func() { httpClient = origClient }()

	origCheck := checkAllowedURL
	checkAllowedURL = func(url string) bool { return true }
	defer func() { checkAllowedURL = origCheck }()

	result, err := Apply(srv.URL+"/test.patch", dir, "drupal/test", "Test patch")
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Applied {
		t.Errorf("expected Applied=true, got false. Error: %s", result.Error)
	}
	if len(result.ChangedFiles) == 0 {
		t.Error("expected changed files report, got none")
	}

	// Verify the file was patched.
	content, _ := os.ReadFile(filepath.Join(dir, "test.txt"))
	if string(content) != "hello world\n" {
		t.Errorf("file content = %q, want %q", string(content), "hello world\n")
	}

	// Verify composer.json was updated.
	var updated map[string]interface{}
	data, _ = os.ReadFile(filepath.Join(dir, "composer.json"))
	json.Unmarshal(data, &updated)
	extra := updated["extra"].(map[string]interface{})
	patches := extra["patches"].(map[string]interface{})
	if _, ok := patches["drupal/test"]; !ok {
		t.Error("composer.json not updated with patch entry")
	}
}

// defaultCheckAllowedURL must parse the URL and match its host exactly (or as
// a subdomain of an allowed domain), require https, and reject on parse
// failure. strings.Contains substring matching is bypassable via host-as-path,
// host-as-subdomain-of-attacker, and domain-in-query-string tricks.
func TestDefaultCheckAllowedURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "accepts legitimate www.drupal.org URL",
			url:  "https://www.drupal.org/files/issues/2024-01-01/token-1234.patch",
			want: true,
		},
		{
			name: "accepts legitimate git.drupal.org URL",
			url:  "https://git.drupal.org/project/example.git",
			want: true,
		},
		{
			name: "rejects host-appended-as-path bypass",
			url:  "https://evil.com/www.drupal.org/evil.patch",
			want: false,
		},
		{
			name: "rejects host-as-subdomain-of-attacker bypass",
			url:  "https://drupal.org.evil.com/evil.patch",
			want: false,
		},
		{
			name: "rejects allowlist-domain-in-query-string bypass",
			url:  "https://notdrupal.org/?x=git.drupal.org",
			want: false,
		},
		{
			name: "rejects non-https scheme",
			url:  "http://drupal.org/files/issues/token-1234.patch",
			want: false,
		},
		{
			name: "rejects URL that fails to parse",
			url:  "https://drupal.org/%gh",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultCheckAllowedURL(tt.url); got != tt.want {
				t.Errorf("defaultCheckAllowedURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestApply_AllowlistViolation(t *testing.T) {
	// Non-drupal.org URL should be rejected.
	_, err := Apply("https://evil.com/malicious.patch", t.TempDir(), "drupal/test", "test")
	if err == nil {
		t.Error("expected error for non-drupal.org URL, got nil")
	}
}

// composer-patches keys patches by description. Writing a list here makes
// composer ignore them, and clobbering the map discards the project's own
// patches for that package.
func TestRegisterPatch_PreservesExistingComposerPatches(t *testing.T) {
	dir := t.TempDir()
	composerFile := filepath.Join(dir, "composer.json")
	existing := `{
  "extra": {
    "patches": {
      "drupal/facets": {
        "Existing fix": "patches/facets/existing.patch"
      }
    }
  }
}`
	if err := os.WriteFile(composerFile, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := registerPatch(dir, "drupal/facets", "https://drupal.org/files/new.patch", "New fix"); err != nil {
		t.Fatalf("registerPatchInComposer error: %v", err)
	}

	var got struct {
		Extra struct {
			Patches map[string]map[string]string `json:"patches"`
		} `json:"extra"`
	}
	data, err := os.ReadFile(composerFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("composer.json is no longer in composer-patches format: %v\n%s", err, data)
	}

	entries := got.Extra.Patches["drupal/facets"]
	if entries["Existing fix"] != "patches/facets/existing.patch" {
		t.Errorf("existing patch lost: %+v", entries)
	}
	if entries["New fix"] != "https://drupal.org/files/new.patch" {
		t.Errorf("new patch not registered: %+v", entries)
	}
}

// Applying a patch is a mutation, not publication: an independent validator
// must bind its evidence to the diff before checkpoint_commit can add history.
func TestApply_LeavesChangesUncommittedForCheckpoint(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	modDir := filepath.Join(dir, "web", "modules", "contrib", "example")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(modDir, "example.info.yml"), []byte("name: old\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{\"extra\":{\"patches\":{}}}"), 0o644)
	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644)
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	// Work in progress that must survive the patch commit.
	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("work in progress\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "untracked.md"), []byte("notes\n"), 0o644)

	patchDir := filepath.Join(dir, "patches")
	os.MkdirAll(patchDir, 0o755)
	patchFile := filepath.Join(patchDir, "fix.patch")
	patchBody := "--- a/example.info.yml\n+++ b/example.info.yml\n@@ -1 +1 @@\n-name: old\n+name: new\n"
	os.WriteFile(patchFile, []byte(patchBody), 0o644)

	result, err := Apply(patchFile, dir, "drupal/example", "Fix")
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("patch not applied: %s", result.Error)
	}

	out, err := exec.Command("git", "-C", dir, "diff", "--name-only").Output()
	if err != nil {
		t.Fatal(err)
	}
	changed := string(out)
	if !strings.Contains(changed, "tracked.txt") {
		t.Errorf("expected tracked unrelated work to remain visible in diff: %s", changed)
	}
	status, statusErr := exec.Command("git", "-C", dir, "status", "--porcelain", "untracked.md").Output()
	if statusErr != nil || !strings.Contains(string(status), "untracked.md") {
		t.Errorf("expected untracked unrelated work to remain uncommitted: %q (%v)", status, statusErr)
	}
	if !strings.Contains(changed, "composer.json") {
		t.Errorf("composer.json missing from pending diff:\n%s", changed)
	}
	if result.ChangedFiles[0] != "composer.json" {
		t.Errorf("changed files = %v, want composer.json first", result.ChangedFiles)
	}

	// The work in progress must still be on disk, uncommitted.
	if data, _ := os.ReadFile(filepath.Join(dir, "tracked.txt")); string(data) != "work in progress\n" {
		t.Errorf("work in progress lost: %q", data)
	}
}
