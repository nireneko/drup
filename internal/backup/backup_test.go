package backup

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
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
