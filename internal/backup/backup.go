// Package backup implements local, testing-only Drupal snapshots.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/envdetect"
	drupexec "github.com/nireneko/drup/internal/exec"
)

const version = 1

type Manifest struct {
	BackupID         string    `json:"backup_id"`
	CreatedAt        time.Time `json:"created_at"`
	ProjectPath      string    `json:"project_path"`
	DrupalRoot       string    `json:"drupal_root"`
	DatabaseCommand  []string  `json:"database_command"`
	FilesChecksum    string    `json:"files_checksum"`
	DatabaseChecksum string    `json:"database_checksum"`
	ArchiveVersion   int       `json:"archive_version"`
}

type Manager struct{ Root string }

func NewManager(project string) *Manager {
	return &Manager{Root: filepath.Join(project, ".drup", "backups")}
}

var run = drupexec.RunWithEnv
var runInput = drupexec.RunWithEnvInput
var detectEnv = func(path string) (*envdetect.Detection, error) { return envdetect.NewDetector().Detect(path, false) }

func validateProject(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid project: absolute project_path required")
	}
	path, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("invalid project: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid project: %s", path)
	}
	return path, nil
}

func (m *Manager) Create(project string) (Manifest, error) {
	project, err := validateProject(project)
	if err != nil {
		return Manifest{}, err
	}
	id := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + randomID()
	dir := filepath.Join(m.Root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Manifest{}, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()

	detection, err := detectEnv(project)
	if err != nil {
		return Manifest{}, err
	}
	dump, err := os.CreateTemp("", "drup-backup-*.sql")
	if err != nil {
		return Manifest{}, err
	}
	dumpPath := dump.Name()
	dump.Close()
	defer os.Remove(dumpPath)
	cmd := []string{"sql:dump", "--result-file=" + dumpPath, "--root=" + project}
	_, stderr, code, runErr := run(detection.CommandPrefix, "drush", cmd...)
	if runErr != nil || code != 0 {
		return Manifest{}, fmt.Errorf("database dump failure: %s", commandError(runErr, stderr, code))
	}

	filesPath := filepath.Join(dir, "files.tar.gz")
	if err = archive(project, filesPath, filepath.Join(project, ".drup", "backups")); err != nil {
		return Manifest{}, fmt.Errorf("filesystem backup failure: %w", err)
	}
	dbPath := filepath.Join(dir, "database.sql.gz")
	if err = gzipFile(dumpPath, dbPath); err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{id, time.Now().UTC(), project, project, append([]string(nil), cmd...), checksum(filesPath), checksum(dbPath), version}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m *Manager) List(project string) ([]Manifest, error) {
	project, err := validateProject(project)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(project, ".drup", "backups"))
	if os.IsNotExist(err) {
		return []Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Start from an empty slice so an empty backup directory marshals as []
	// instead of null.
	result := []Manifest{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, e := os.ReadFile(filepath.Join(project, ".drup", "backups", e.Name(), "manifest.json"))
		if e != nil {
			continue
		}
		var v Manifest
		if json.Unmarshal(data, &v) == nil {
			result = append(result, v)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (m *Manager) Delete(project, id string) error {
	project, err := validateProject(project)
	if err != nil {
		return err
	}
	if !safeID(id) {
		return fmt.Errorf("backup not found")
	}
	path := filepath.Join(project, ".drup", "backups", id)
	if _, err = os.Stat(filepath.Join(path, "manifest.json")); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", id)
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func (m *Manager) Restore(project, id string, confirm bool) error {
	project, err := validateProject(project)
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("restore confirmation missing")
	}
	if !safeID(id) {
		return fmt.Errorf("backup not found")
	}
	dir := filepath.Join(project, ".drup", "backups", id)
	var mfest Manifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", id)
	}
	if err != nil {
		return err
	}
	if json.Unmarshal(data, &mfest) != nil {
		return fmt.Errorf("checksum failure: invalid manifest")
	}
	files, db := filepath.Join(dir, "files.tar.gz"), filepath.Join(dir, "database.sql.gz")
	if mfest.FilesChecksum == "" || mfest.DatabaseChecksum == "" || checksum(files) != mfest.FilesChecksum || checksum(db) != mfest.DatabaseChecksum {
		return fmt.Errorf("checksum failure")
	}
	stage, err := os.MkdirTemp("", "drup-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = extract(files, stage); err != nil {
		return fmt.Errorf("filesystem restore failure: %w", err)
	}
	detection, err := detectEnv(project)
	if err != nil {
		return err
	}
	in, err := os.Open(db)
	if err != nil {
		return err
	}
	defer in.Close()
	_, stderr, code, runErr := runInput(detection.CommandPrefix, in, "drush", "sql:cli", "--root="+project)
	if runErr != nil || code != 0 {
		return fmt.Errorf("database restore failure: %s", commandError(runErr, stderr, code))
	}
	if err = replaceProject(project, stage); err != nil {
		return fmt.Errorf("filesystem restore failure: %w", err)
	}
	return nil
}

func randomID() string { b := make([]byte, 4); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func safeID(id string) bool {
	return id != "" && filepath.Base(id) == id && !strings.Contains(id, "..")
}
func commandError(err error, stderr string, code int) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("exit %d: %s", code, strings.TrimSpace(stderr))
}
func checksum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func archive(root, dest, excluded string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == excluded || strings.HasPrefix(path, excluded+string(filepath.Separator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// node_modules is a build artifact: huge, regenerable, and full of
		// symlinks. Backing it up made every real theme fail.
		if info.IsDir() && info.Name() == "node_modules" {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			h, hdrErr := tar.FileInfoHeader(info, target)
			if hdrErr != nil {
				return hdrErr
			}
			h.Name = filepath.ToSlash(rel)
			return tw.WriteHeader(h)
		}
		h, _ := tar.FileInfoHeader(info, "")
		h.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, in)
			in.Close()
			return err
		}
		return nil
	})
}

func extract(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeSymlink {
			return fmt.Errorf("unsupported archive entry: %s", h.Name)
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive path escape: %s", h.Name)
		}
		out := filepath.Join(dest, name)
		if !strings.HasPrefix(out, filepath.Clean(dest)+string(filepath.Separator)) && out != filepath.Clean(dest) {
			return fmt.Errorf("archive path escape: %s", h.Name)
		}
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(out, os.FileMode(h.Mode)); err != nil {
				return err
			}
			continue
		}
		if h.Typeflag == tar.TypeSymlink {
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			// Only recreate links that stay inside the restore target, so a
			// later entry cannot be written through one into the system.
			link := filepath.FromSlash(h.Linkname)
			if filepath.IsAbs(link) {
				return fmt.Errorf("archive symlink escape: %s -> %s", h.Name, h.Linkname)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(out), link))
			root := filepath.Clean(dest)
			if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
				return fmt.Errorf("archive symlink escape: %s -> %s", h.Name, h.Linkname)
			}
			os.Remove(out)
			if err := os.Symlink(link, out); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode))
		if err != nil {
			return err
		}
		_, err = io.Copy(w, tr)
		w.Close()
		if err != nil {
			return err
		}
	}
}
func gzipFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	_, err = io.Copy(gz, in)
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}
func replaceProject(project, stage string) error {
	entries, err := os.ReadDir(project)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".drup" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(project, e.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(stage)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(stage, e.Name()), filepath.Join(project, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, linkErr := os.Readlink(src)
		if linkErr != nil {
			return linkErr
		}
		os.Remove(dst)
		return os.Symlink(target, dst)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}
