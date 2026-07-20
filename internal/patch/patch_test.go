package patch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	if result.CommitHash == "" {
		t.Error("expected commit hash, got empty")
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

func TestApply_AllowlistViolation(t *testing.T) {
	// Non-drupal.org URL should be rejected.
	_, err := Apply("https://evil.com/malicious.patch", t.TempDir(), "drupal/test", "test")
	if err == nil {
		t.Error("expected error for non-drupal.org URL, got nil")
	}
}
