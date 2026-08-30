package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/envdetect"
)

func TestArchiveExtractRoundTrip(t *testing.T) {
	root, dest := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "settings.php"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "files.tar.gz")
	if err := archive(root, archivePath, filepath.Join(root, "excluded")); err != nil {
		t.Fatal(err)
	}
	if err := extract(archivePath, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "settings.php"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test" {
		t.Fatalf("content = %q", data)
	}
}

func TestManagerCreateRestoreListDelete(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalRun, originalInput, originalDetect := run, runInput, detectEnv
	defer func() { run, runInput, detectEnv = originalRun, originalInput, originalDetect }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	run = func(dir string, _ []string, _ string, args ...string) (string, string, int, error) {
		for _, arg := range args {
			if len(arg) > len("--result-file=") && arg[:len("--result-file=")] == "--result-file=" {
				// drush resolves a relative --result-file against its working
				// directory, exactly as the real command does.
				target := arg[len("--result-file="):]
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				if err := os.WriteFile(target, []byte("-- database dump\nCREATE TABLE x;\n"), 0o600); err != nil {
					return "", "", -1, err
				}
			}
		}
		return "", "", 0, nil
	}
	runInput = func([]string, io.Reader, string, ...string) (string, string, int, error) {
		return "", "", 0, nil
	}

	manifest, err := NewManager(project).Create(project)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BackupID == "" || manifest.FilesChecksum == "" || manifest.DatabaseChecksum == "" {
		t.Fatalf("incomplete manifest: %+v", manifest)
	}
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(project).Restore(project, manifest.BackupID, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(project, "settings.php"))
	if err != nil || string(data) != "original" {
		t.Fatalf("restored settings = %q, err = %v", data, err)
	}
	backups, err := NewManager(project).List(project)
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups = %+v, err = %v; expected original plus independent rescue", backups, err)
	}
	foundOriginal := false
	for _, candidate := range backups {
		foundOriginal = foundOriginal || candidate.BackupID == manifest.BackupID
	}
	if !foundOriginal {
		t.Fatalf("original backup %s disappeared", manifest.BackupID)
	}
	if err := NewManager(project).Delete(project, manifest.BackupID); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRejectsTraversalAndSymlink(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute"} {
		path := maliciousArchive(t, &tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}, []byte("x"))
		if err := extract(path, t.TempDir()); err == nil {
			t.Errorf("extract(%q) accepted traversal", name)
		}
	}
	path := maliciousArchive(t, &tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/escape"}, nil)
	if err := extract(path, t.TempDir()); err == nil {
		t.Error("extract accepted symlink")
	}
}

func maliciousArchive(t *testing.T, header *tar.Header, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bad.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if len(data) > 0 {
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	return path
}

func TestValidateProject_ResolvesSymlinkedPath(t *testing.T) {
	real := t.TempDir()
	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	resolved, err := validateProject(link)
	if err != nil {
		t.Fatalf("validateProject error: %v", err)
	}
	wantResolved, evalErr := filepath.EvalSymlinks(real)
	if evalErr != nil {
		t.Fatalf("EvalSymlinks error: %v", evalErr)
	}
	if resolved != wantResolved {
		t.Errorf("validateProject(%q) = %q, want %q (the real target)", link, resolved, wantResolved)
	}
}

func TestValidateProject_MissingPathErrorUnchanged(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := validateProject(missing); err == nil {
		t.Fatal("expected error for a missing project path")
	}
}

func TestValidateProject_NonDirErrorUnchanged(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateProject(file); err == nil {
		t.Fatal("expected error for a non-directory project path")
	}
}

func TestRestoreRequiresConfirmation(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".drup", "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(project).Restore(project, "missing", false); err == nil {
		t.Fatal("restore without confirmation succeeded")
	}
}

// A backup that includes vendor/ and web/core/ runs to gigabytes, which stops
// anyone from taking one before each risky step. Both rebuild from lockfiles.
func TestArchive_SkipsRegenerableTrees(t *testing.T) {
	project := t.TempDir()
	for _, dir := range []string{
		filepath.Join("web", "sites", "default", "files"),
		".git",
		filepath.Join("web", "core", "lib"),
		filepath.Join("web", "modules", "custom", "mine"),
		filepath.Join("web", "themes", "custom", "mine", "node_modules", "pkg"),
		"vendor/drupal",
	} {
		if err := os.MkdirAll(filepath.Join(project, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, dir, "f.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "composer.lock"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "files.tar.gz")
	if err := archive(project, dest, filepath.Join(project, ".drup")); err != nil {
		t.Fatalf("archive error: %v", err)
	}

	out := t.TempDir()
	if err := extract(dest, out); err != nil {
		t.Fatalf("extract error: %v", err)
	}

	mustExist := []string{
		filepath.Join("web", "modules", "custom", "mine", "f.txt"),
		"composer.lock",
	}
	for _, p := range mustExist {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("%s missing from backup: %v", p, err)
		}
	}
	mustSkip := []string{
		filepath.Join("web", "core", "lib", "f.txt"),
		filepath.Join("vendor", "drupal", "f.txt"),
		filepath.Join("web", "themes", "custom", "mine", "node_modules", "pkg", "f.txt"),
		filepath.Join("web", "sites", "default", "files", "f.txt"),
		filepath.Join(".git", "f.txt"),
	}
	for _, p := range mustSkip {
		if _, err := os.Stat(filepath.Join(out, p)); !os.IsNotExist(err) {
			t.Errorf("%s should not be archived", p)
		}
	}
}

// drush prints "[success] Database dump saved" even when mysqldump refused —
// a missing PROCESS privilege, for instance. Trusting that message produced a
// 23-byte backup, recorded a checksum of it, and left the run with no
// database rollback point at all.
func TestCreate_RejectsAnEmptyDatabaseDump(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalRun, originalDetect := run, detectEnv
	defer func() { run, detectEnv = originalRun, originalDetect }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	// Report success without writing anything, exactly as drush does.
	run = func(string, []string, string, ...string) (string, string, int, error) {
		return "[success] Database dump saved", "", 0, nil
	}

	if _, err := NewManager(project).Create(project); err == nil {
		t.Fatal("an empty dump was accepted as a valid backup")
	} else if !strings.Contains(err.Error(), "database dump failure") {
		t.Errorf("error = %v, want a database dump failure", err)
	}
}

func TestVerifyDump(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if err := verifyDump(filepath.Join(dir, "absent.sql")); err == nil {
		t.Error("a missing dump was accepted")
	}
	if err := verifyDump(write("empty.sql", "")); err == nil {
		t.Error("an empty dump was accepted")
	}
	if err := verifyDump(write("html.sql", "<html>error page</html>")); err == nil {
		t.Error("a non-SQL payload was accepted")
	}
	if err := verifyDump(write("real.sql", "-- MySQL dump\nCREATE TABLE node;")); err != nil {
		t.Errorf("a valid dump was rejected: %v", err)
	}
}

// drush resolves a relative --result-file against the Drupal root, not the
// working directory. Passing a bare filename dropped a full database dump
// inside the web-served docroot and left drup looking for it one level up.
func TestDumpTarget_ResolvesAgainstTheContainerProjectRoot(t *testing.T) {
	originalRun := run
	defer func() { run = originalRun }()
	run = func(_ string, _ []string, _ string, args ...string) (string, string, int, error) {
		return "/var/www/html/web\n", "", 0, nil
	}

	got, err := dumpTarget("/home/dev/site", []string{"ddev", "exec"}, ".drup-dump-1.sql")
	if err != nil {
		t.Fatalf("dumpTarget error: %v", err)
	}
	if got != "/var/www/html/.drup-dump-1.sql" {
		t.Errorf("target = %q, want the container project root, not the docroot", got)
	}
	if strings.Contains(got, "/web/") {
		t.Error("the dump would land inside the web-served docroot")
	}
}

func TestDumpTarget_HostRunsUseTheProjectPath(t *testing.T) {
	got, err := dumpTarget("/home/dev/site", nil, ".drup-dump-1.sql")
	if err != nil {
		t.Fatalf("dumpTarget error: %v", err)
	}
	if got != filepath.Join("/home/dev/site", ".drup-dump-1.sql") {
		t.Errorf("target = %q, want the host project path", got)
	}
}

func TestDumpTarget_FailsWhenTheRootCannotBeResolved(t *testing.T) {
	originalRun := run
	defer func() { run = originalRun }()
	run = func(string, []string, string, ...string) (string, string, int, error) {
		return "", "no bootstrap", 1, nil
	}

	if _, err := dumpTarget("/home/dev/site", []string{"ddev", "exec"}, "d.sql"); err == nil {
		t.Error("an unresolvable Drupal root was accepted; the dump would go somewhere unknown")
	}
}

func TestRestoreFailureKeepsOriginalAndRescueWithJournal(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRun, originalInput, originalDetect := run, runInput, detectEnv
	defer func() { run, runInput, detectEnv = originalRun, originalInput, originalDetect }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	run = testDumpRun(t)
	runInput = func([]string, io.Reader, string, ...string) (string, string, int, error) {
		return "", "database unavailable", 1, nil
	}
	m := NewManager(project)
	manifest, err := m.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Restore(project, manifest.BackupID, true); err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	data, err := os.ReadFile(filepath.Join(project, "settings.php"))
	if err != nil || string(data) != "changed" {
		t.Fatalf("current tree = %q, err = %v", data, err)
	}
	if !HasIncompleteRestore(project) {
		t.Fatal("failed restore did not leave a recoverable journal")
	}
	backups, err := m.List(project)
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups = %d, err = %v; original and rescue must remain", len(backups), err)
	}
}

func TestRestoreSuccessCompletesJournalAndPreservesPriorTree(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRun, originalInput, originalDetect := run, runInput, detectEnv
	defer func() { run, runInput, detectEnv = originalRun, originalInput, originalDetect }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	run = testDumpRun(t)
	runInput = func([]string, io.Reader, string, ...string) (string, string, int, error) { return "", "", 0, nil }
	m := NewManager(project)
	manifest, err := m.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Restore(project, manifest.BackupID, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(project, "settings.php"))
	if err != nil || string(data) != "original" {
		t.Fatalf("restored tree = %q, err = %v", data, err)
	}
	if HasIncompleteRestore(project) {
		t.Fatal("completed restore is still blocking")
	}
	journals, err := ListRestoreJournals(project)
	if err != nil || len(journals) != 1 || journals[0].State != RestoreCompleted {
		t.Fatalf("journals = %+v, err = %v", journals, err)
	}
	prior, err := os.ReadFile(filepath.Join(journals[0].PreviousPath, "settings.php"))
	if err != nil || string(prior) != "changed" {
		t.Fatalf("preserved tree = %q, err = %v", prior, err)
	}
}

func testDumpRun(t *testing.T) func(string, []string, string, ...string) (string, string, int, error) {
	t.Helper()
	return func(dir string, _ []string, _ string, args ...string) (string, string, int, error) {
		for _, arg := range args {
			if strings.HasPrefix(arg, "--result-file=") {
				if err := os.WriteFile(filepath.Join(dir, filepath.Base(strings.TrimPrefix(arg, "--result-file="))), []byte("-- dump\nCREATE TABLE x;"), 0o600); err != nil {
					return "", "", -1, err
				}
			}
		}
		return "", "", 0, nil
	}
}

func TestRestoreFilesystemSwapFailureRollsBackCurrentTree(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRun, originalInput, originalDetect, originalMove := run, runInput, detectEnv, movePath
	defer func() { run, runInput, detectEnv, movePath = originalRun, originalInput, originalDetect, originalMove }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	run = testDumpRun(t)
	runInput = func([]string, io.Reader, string, ...string) (string, string, int, error) { return "", "", 0, nil }
	m := NewManager(project)
	manifest, err := m.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	movePath = func(source, destination string) error {
		if strings.Contains(source, ".drup-restore-stage-") && destination == filepath.Join(project, "settings.php") {
			return os.ErrPermission
		}
		return originalMove(source, destination)
	}
	if err := m.Restore(project, manifest.BackupID, true); err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	data, err := os.ReadFile(filepath.Join(project, "settings.php"))
	if err != nil || string(data) != "changed" {
		t.Fatalf("rollback tree = %q, err = %v", data, err)
	}
	if !HasIncompleteRestore(project) {
		t.Fatal("swap failure did not block follow-up mutations")
	}
	backups, err := m.List(project)
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups = %d, err = %v; rescue must remain", len(backups), err)
	}
}

func TestRestoreCheckRejectsIncompleteJournalAndReportsPlan(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "settings.php"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalRun, originalDetect := run, detectEnv
	defer func() { run, detectEnv = originalRun, originalDetect }()
	detectEnv = func(string) (*envdetect.Detection, error) { return &envdetect.Detection{}, nil }
	run = testDumpRun(t)
	m := NewManager(project)
	manifest, err := m.Create(project)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := m.RestoreCheck(project, manifest.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Confirmed || plan.DatabaseMode != "non_atomic" || plan.PlanID == "" {
		t.Fatalf("plan = %+v", plan)
	}
	if err := os.MkdirAll(filepath.Join(project, ".drup", "restores"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".drup", "restores", "blocked.json"), []byte(`{"version":1,"id":"blocked","backup_id":"x","state":"recovery_required","database_mode":"non_atomic","continuation":"recover","updated_at":"2026-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreCheck(project, manifest.BackupID); err == nil {
		t.Fatal("restore check accepted incomplete journal")
	}
	if err := m.Restore(project, manifest.BackupID, true); err == nil {
		t.Fatal("restore retried despite incomplete journal")
	}
}
