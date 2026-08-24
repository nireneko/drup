package envdetect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		wantEnv    Environment
		wantPrefix []string
	}{
		{
			name: "ddev environment",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)
			},
			wantEnv:    EnvDdev,
			wantPrefix: []string{"ddev", "exec"},
		},
		{
			name: "lando environment",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, ".lando.yml"), []byte("name: test\n"), 0o644)
			},
			wantEnv:    EnvLando,
			wantPrefix: []string{"lando"},
		},
		{
			name: "docker4drupal environment",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  drupal:\n    image: drupal\n"), 0o644)
			},
			wantEnv:    EnvDocker4Drupal,
			wantPrefix: []string{"docker", "compose", "exec", "php"},
		},
		{
			name: "direct environment (composer.json only)",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644)
			},
			wantEnv:    EnvDirect,
			wantPrefix: []string{},
		},
		{
			name:       "unsupported environment (empty dir, no markers)",
			setup:      func(t *testing.T, dir string) {},
			wantEnv:    EnvUnsupported,
			wantPrefix: []string{},
		},
		{
			name: "ambiguous markers — ddev wins over lando",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)
				os.WriteFile(filepath.Join(dir, ".lando.yml"), []byte("name: test\n"), 0o644)
			},
			wantEnv:    EnvDdev,
			wantPrefix: []string{"ddev", "exec"},
		},
		{
			name: "docker-compose without drupal → direct if composer.json",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644)
				os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644)
			},
			wantEnv:    EnvDirect,
			wantPrefix: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			d := NewDetector()
			result, err := d.Detect(dir, false)
			if err != nil {
				t.Fatalf("Detect error: %v", err)
			}
			if result.Environment != tt.wantEnv {
				t.Errorf("Environment = %q, want %q", result.Environment, tt.wantEnv)
			}
			if len(result.CommandPrefix) != len(tt.wantPrefix) {
				t.Errorf("CommandPrefix = %v, want %v", result.CommandPrefix, tt.wantPrefix)
			} else {
				for i := range tt.wantPrefix {
					if result.CommandPrefix[i] != tt.wantPrefix[i] {
						t.Errorf("CommandPrefix[%d] = %q, want %q", i, result.CommandPrefix[i], tt.wantPrefix[i])
					}
				}
			}
		})
	}
}

func TestDetect_NonExistentPath(t *testing.T) {
	d := NewDetector()
	result, err := d.Detect("/nonexistent/path/xyz", false)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if result.Environment != EnvUnknown {
		t.Errorf("Environment = %q, want %q", result.Environment, EnvUnknown)
	}
}

func TestDetect_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	os.WriteFile(file, []byte("hello"), 0o644)

	d := NewDetector()
	result, err := d.Detect(file, false)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if result.Environment != EnvUnknown {
		t.Errorf("Environment = %q, want %q", result.Environment, EnvUnknown)
	}
}

func TestDetect_EmptyPath(t *testing.T) {
	d := NewDetector()
	_, err := d.Detect("", false)
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}

func TestDetect_RelativePath(t *testing.T) {
	d := NewDetector()
	_, err := d.Detect("relative/path", false)
	if err == nil {
		t.Error("expected error for relative path, got nil")
	}
}

func TestDetect_CacheHit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)

	d := NewDetector()
	first, _ := d.Detect(dir, false)
	second, _ := d.Detect(dir, false)

	if first != second {
		t.Error("expected cache hit to return same pointer")
	}
}

func TestDetect_SymlinkedPathResolvesToRealTargetAndSharesCache(t *testing.T) {
	real := t.TempDir()
	os.WriteFile(filepath.Join(real, "composer.json"), []byte("{}"), 0o644)

	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	d := NewDetector()
	viaLink, err := d.Detect(link, false)
	if err != nil {
		t.Fatalf("Detect(link) error: %v", err)
	}
	if viaLink.Environment != EnvDirect {
		t.Errorf("Environment via symlink = %q, want %q", viaLink.Environment, EnvDirect)
	}

	viaReal, err := d.Detect(real, false)
	if err != nil {
		t.Fatalf("Detect(real) error: %v", err)
	}
	// Both paths resolve to the same real project, so they must hit the
	// same cache entry (design Open Question 2: the cache is keyed on the
	// resolved path, not the raw caller-supplied one).
	if viaLink != viaReal {
		t.Error("expected the symlinked and real path to share the same cache entry")
	}
}

func TestDetect_UnsupportedEnvironmentStaysGracefulAfterSymlinkResolution(t *testing.T) {
	dir := t.TempDir() // no markers at all
	d := NewDetector()
	result, err := d.Detect(dir, false)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if result.Environment != EnvUnsupported {
		t.Errorf("Environment = %q, want %q", result.Environment, EnvUnsupported)
	}
}

func TestDetect_NonExistentPathStaysGracefulAfterSymlinkResolution(t *testing.T) {
	d := NewDetector()
	result, err := d.Detect("/nonexistent/path/xyz-envdetect", false)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if result.Environment != EnvUnknown {
		t.Errorf("Environment = %q, want %q", result.Environment, EnvUnknown)
	}
}

func TestDetect_ForceDetectBypassesCache(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)

	d := NewDetector()
	first, _ := d.Detect(dir, false)
	second, _ := d.Detect(dir, true)

	if first == second {
		t.Error("expected force_detect to return different pointer")
	}
	if second.Environment != EnvDdev {
		t.Errorf("Environment = %q, want %q", second.Environment, EnvDdev)
	}
}
