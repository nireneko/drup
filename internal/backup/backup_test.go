package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
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
	run = func(_ []string, _ string, args ...string) (string, string, int, error) {
		for _, arg := range args {
			if len(arg) > len("--result-file=") && arg[:len("--result-file=")] == "--result-file=" {
				if err := os.WriteFile(arg[len("--result-file="):], []byte("database"), 0o600); err != nil {
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
	if err != nil || len(backups) != 1 || backups[0].BackupID != manifest.BackupID {
		t.Fatalf("backups = %+v, err = %v", backups, err)
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
	}
	for _, p := range mustSkip {
		if _, err := os.Stat(filepath.Join(out, p)); !os.IsNotExist(err) {
			t.Errorf("%s should not be archived", p)
		}
	}
}
