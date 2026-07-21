package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// executableFn resolves the path to the currently running executable.
// Package-level var for testability.
var executableFn = os.Executable

// homeDirFn resolves the user's home directory (for the backups directory).
// Package-level var for testability.
var homeDirFn = os.UserHomeDir

// resolveAssetURLFn and resolveChecksumURLFn build the download URLs used by
// Upgrade. Package-level vars (wrapping the exported resolvers) so tests can
// redirect them to a local httptest server without a real GitHub round trip.
var resolveAssetURLFn = ResolveAssetURL
var resolveChecksumURLFn = ResolveChecksumURL

// UpgradeOptions configures a single self-upgrade run.
type UpgradeOptions struct {
	Owner   string
	Repo    string
	Binary  string
	Version string
}

// ResolveArchiveName returns the deterministic GoReleaser archive filename
// for the given repo/version/os/arch combination.
//
// Convention: {repo}_{version}_{goos}_{goarch}.tar.gz
func ResolveArchiveName(repo, version, goos, goarch string) string {
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", repo, version, goos, goarch)
}

// ResolveAssetURL constructs the GitHub Releases asset download URL for the
// given repo/version/os/arch combination.
func ResolveAssetURL(owner, repo, version, goos, goarch string) string {
	filename := ResolveArchiveName(repo, version, goos, goarch)
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/%s", owner, repo, version, filename)
}

// ResolveChecksumURL constructs the GitHub Releases URL for a release's
// checksums.txt file.
func ResolveChecksumURL(owner, repo, version string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/checksums.txt", owner, repo, version)
}

// Download downloads the archive at assetURL directly to archivePath and
// verifies its SHA256 checksum against the checksums.txt entry for
// archiveName at checksumURL. On any verification failure the partially
// downloaded archivePath is deleted and an error is returned; the caller
// MUST treat this as a security warning, not a transient failure.
func Download(assetURL, checksumURL, archiveName, archivePath string) error {
	resp, err := httpClient.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", archiveName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", archiveName, resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create %s: %w", archivePath, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		os.Remove(archivePath)
		return fmt.Errorf("write %s: %w", archivePath, err)
	}
	actualDigest := hex.EncodeToString(h.Sum(nil))

	checksumsContent, err := fetchChecksums(checksumURL)
	if err != nil {
		os.Remove(archivePath)
		return fmt.Errorf("checksum verification failed: checksums.txt unavailable: %w", err)
	}

	expectedDigest, err := expectedChecksumFor(checksumsContent, archiveName)
	if err != nil {
		os.Remove(archivePath)
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if actualDigest != expectedDigest {
		os.Remove(archivePath)
		return fmt.Errorf("security warning: checksum mismatch for %s: expected %s, got %s",
			archiveName, expectedDigest, actualDigest)
	}

	return nil
}

// fetchChecksums downloads checksums.txt from url and returns its content.
func fetchChecksums(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch checksums.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums.txt: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read checksums.txt: %w", err)
	}
	return string(data), nil
}

// expectedChecksumFor parses checksums.txt content (BSD-style: "<digest>
// <filename>" per line) and returns the SHA256 hex digest for filename.
func expectedChecksumFor(content, filename string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%q not listed in checksums.txt", filename)
}

// ExtractBinaryFromTarGz reads a .tar.gz stream in a single pass and
// extracts the first regular-file entry whose base name matches binaryName,
// writing it directly to outPath with executable permissions.
//
// Matching is by base name only, so archives that nest the binary inside a
// subdirectory (e.g. "drup_1.0.0_linux_amd64/drup") are handled the same as
// a root-level layout. hdr.Name is used only for this basename comparison —
// it is never joined into a filesystem path, so path-traversal entries in
// the archive are structurally inert. Only tar.TypeReg/TypeRegA entries are
// accepted; symlinks, hardlinks, and directories are rejected even if their
// base name matches, so a malicious or unexpected archive cannot redirect
// the write.
func ExtractBinaryFromTarGz(r io.Reader, binaryName, outPath string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		if err := writeExecutable(tr, outPath); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// writeExecutable writes the content from r to outPath with executable
// permissions. The executable bit is set at write time via the file mode
// passed to OpenFile — there is no trailing chmod pass.
func writeExecutable(r io.Reader, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	return nil
}

// atomicReplace moves src to dst atomically using os.Rename. This is safe on
// the same filesystem even when dst is the currently running executable:
// rename replaces the directory entry while the old inode stays alive for
// the running process (avoids ETXTBSY from a direct overwrite).
func atomicReplace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", src, dst, err)
	}
	return nil
}

// copyFile copies src to dst by reading and writing (not os.Rename), so it
// works across filesystem boundaries. The destination's executable bits are
// preserved from the source's mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", dst, err)
	}

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}

	if fi, err := in.Stat(); err == nil {
		if err := out.Chmod(fi.Mode()); err != nil {
			return fmt.Errorf("chmod %s: %w", dst, err)
		}
	}

	return out.Close()
}

// BackupBinary copies currentBin to backupPath, preserving its executable
// permission. backupPath may live on a different filesystem than currentBin
// (e.g. $HOME/.drup/backups vs. an arbitrary install dir), so this uses
// copyFile rather than a rename.
func BackupBinary(currentBin, backupPath string) error {
	if err := copyFile(currentBin, backupPath); err != nil {
		return fmt.Errorf("backup %s to %s: %w", currentBin, backupPath, err)
	}
	return nil
}

// RestoreBinary restores backupPath to currentBin. It copies the backup to a
// temporary file next to currentBin, then renames it into place atomically —
// the same ETXTBSY-avoidance strategy used for the original replacement —
// rather than overwriting currentBin directly.
func RestoreBinary(backupPath, currentBin string) error {
	tmpRestorePath := currentBin + ".restore"
	if err := copyFile(backupPath, tmpRestorePath); err != nil {
		return fmt.Errorf("copy backup %s to restore path: %w", backupPath, err)
	}
	if err := atomicReplace(tmpRestorePath, currentBin); err != nil {
		os.Remove(tmpRestorePath)
		return fmt.Errorf("rename backup into place at %s: %w", currentBin, err)
	}
	return nil
}

// ReplaceBinary atomically replaces currentBin with newBinaryPath.
//
// currentBin is first backed up to backupPath. If the atomic replace then
// fails, the backup is unconditionally restored to currentBin — regardless
// of whether currentBin appears intact — per the spec's literal restore
// requirement. If the restore also fails, both failure causes and
// backupPath are named in the returned error so the caller can recover
// manually; ReplaceBinary never returns nil after a failed replace.
func ReplaceBinary(newBinaryPath, currentBin, backupPath string) error {
	if err := BackupBinary(currentBin, backupPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	replaceErr := atomicReplace(newBinaryPath, currentBin)
	if replaceErr == nil {
		return nil
	}

	if restoreErr := RestoreBinary(backupPath, currentBin); restoreErr != nil {
		return fmt.Errorf(
			"replace binary failed (%v) and restore from backup also failed (%v); manual recovery required from backup at %s",
			replaceErr, restoreErr, backupPath,
		)
	}

	return fmt.Errorf("replace binary failed, restored previous binary from backup at %s: %w", backupPath, replaceErr)
}

// Upgrade runs the full self-update flow for the running binary: resolve
// deterministic download URLs from the runtime's actual GOOS/GOARCH (never
// an environment override), download and checksum-verify the release
// archive, extract the target binary in a single pass, and atomically
// replace the current binary with backup-restore-on-failure.
func Upgrade(opts UpgradeOptions) error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	archiveName := ResolveArchiveName(opts.Repo, opts.Version, goos, goarch)
	assetURL := resolveAssetURLFn(opts.Owner, opts.Repo, opts.Version, goos, goarch)
	checksumURL := resolveChecksumURLFn(opts.Owner, opts.Repo, opts.Version)

	tmpDir, err := os.MkdirTemp("", "drup-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := Download(assetURL, checksumURL, archiveName, archivePath); err != nil {
		return fmt.Errorf("download update: %w", err)
	}

	currentBin, err := executableFn()
	if err != nil {
		return fmt.Errorf("get current binary path: %w", err)
	}
	currentBin, err = filepath.EvalSymlinks(currentBin)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	newBinaryPath := currentBin + ".new"
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer archiveFile.Close()

	if err := ExtractBinaryFromTarGz(archiveFile, opts.Binary, newBinaryPath); err != nil {
		return fmt.Errorf("extract update: %w", err)
	}

	homeDir, err := homeDirFn()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	backupDir := filepath.Join(homeDir, ".drup", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	backupPath := filepath.Join(backupDir, filepath.Base(currentBin)+".bak")

	if err := ReplaceBinary(newBinaryPath, currentBin, backupPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}
