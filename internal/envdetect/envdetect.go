package envdetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Environment represents the detected development environment.
type Environment string

const (
	EnvDdev          Environment = "ddev"
	EnvLando         Environment = "lando"
	EnvDocker4Drupal Environment = "docker4drupal"
	EnvDirect        Environment = "direct"
	EnvUnknown       Environment = "unknown"
	// EnvUnsupported is the terminal state reported when a project directory
	// exists but contains none of the recognized environment markers
	// (.ddev, .lando.yml, a Drupal docker-compose.yml, or composer.json).
	// Callers (preflight) MUST treat this as a hard stop and MUST NOT proceed
	// to later pipeline stages.
	EnvUnsupported Environment = "unsupported"
)

// Detection holds the result of environment detection.
type Detection struct {
	Environment   Environment `json:"environment"`
	CommandPrefix []string    `json:"command_prefix"`
	DetectedAt    time.Time   `json:"detected_at"`
}

// Detector detects the development environment for a project path.
type Detector interface {
	Detect(projectPath string, forceDetect bool) (*Detection, error)
}

// DefaultDetector checks marker files with in-memory cache.
type DefaultDetector struct {
	mu    sync.Mutex
	cache map[string]*Detection
}

// NewDetector creates a new DefaultDetector.
func NewDetector() *DefaultDetector {
	return &DefaultDetector{
		cache: make(map[string]*Detection),
	}
}

// Detect detects the environment for projectPath, using cache unless forceDetect is true.
func (d *DefaultDetector) Detect(projectPath string, forceDetect bool) (*Detection, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project_path must not be empty")
	}

	if !filepath.IsAbs(projectPath) {
		return nil, fmt.Errorf("project_path must be an absolute path: %s", projectPath)
	}

	info, err := os.Stat(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Detection{
				Environment:   EnvUnknown,
				CommandPrefix: []string{},
				DetectedAt:    time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("stat %s: %w", projectPath, err)
	}
	if !info.IsDir() {
		return &Detection{
			Environment:   EnvUnknown,
			CommandPrefix: []string{},
			DetectedAt:    time.Now(),
		}, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if !forceDetect {
		if cached, ok := d.cache[projectPath]; ok {
			// Check if project root mtime is newer than detection time.
			if !info.ModTime().After(cached.DetectedAt) {
				return cached, nil
			}
		}
	}

	result := detect(projectPath)
	d.cache[projectPath] = result
	return result, nil
}

func detect(projectPath string) *Detection {
	now := time.Now()

	// 1. .ddev/ directory
	if info, err := os.Stat(filepath.Join(projectPath, ".ddev")); err == nil && info.IsDir() {
		return &Detection{
			Environment:   EnvDdev,
			CommandPrefix: []string{"ddev"},
			DetectedAt:    now,
		}
	}

	// 2. .lando.yml file
	if _, err := os.Stat(filepath.Join(projectPath, ".lando.yml")); err == nil {
		return &Detection{
			Environment:   EnvLando,
			CommandPrefix: []string{"lando"},
			DetectedAt:    now,
		}
	}

	// 3. docker-compose.yml with drupal reference
	if data, err := os.ReadFile(filepath.Join(projectPath, "docker-compose.yml")); err == nil {
		if containsDrupal(string(data)) {
			return &Detection{
				Environment:   EnvDocker4Drupal,
				CommandPrefix: []string{"docker", "compose", "exec", "php"},
				DetectedAt:    now,
			}
		}
	}

	// 4. composer.json exists → direct
	if _, err := os.Stat(filepath.Join(projectPath, "composer.json")); err == nil {
		return &Detection{
			Environment:   EnvDirect,
			CommandPrefix: []string{},
			DetectedAt:    now,
		}
	}

	// 5. No recognized marker found — terminal unsupported state.
	return &Detection{
		Environment:   EnvUnsupported,
		CommandPrefix: []string{},
		DetectedAt:    now,
	}
}

func containsDrupal(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "drupal")
}
