package app

import "fmt"

// Version is set at build time via ldflags.
// Default value is "dev-version" so any binary not produced by the release
// pipeline (go build, local installs, dev workflows) self-identifies as a
// development build. The release pipeline (see .goreleaser.yaml) overrides
// this with the git tag stripped of the "v" prefix, e.g. tag "v0.2.0"
// injects "0.2.0".
var Version = "dev-version"

// Run dispatches CLI commands based on args[0].
func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	// A --help anywhere in the arguments prints usage instead of running.
	// "drup preflight --help" used to execute the command, and "drup report
	// --help" passed --help through as a path.
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" || arg == "help" {
			printUsage()
			return nil
		}
	}

	switch args[0] {
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "version", "--version", "-v":
		fmt.Printf("drup %s\n", Version)
		return nil
	case "init":
		return RunInit(args[1:])
	case "scan":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup scan <path>")
		}
		return RunScan(args[1])
	case "fix":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup fix <path>")
		}
		return RunFix(args[1])
	case "contrib":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup contrib <module>")
		}
		return RunContrib(args[1])
	case "issue":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup issue <module_or_nid>")
		}
		return RunIssue(args[1])
	case "report":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup report <path>")
		}
		return RunReport(args[1])
	case "mcp":
		return RunMCP()
	case "install":
		return RunInstall()
	case "uninstall":
		return RunUninstall(args[1:])
	case "sync":
		return RunSync()
	case "upgrade":
		return RunUpgrade()
	case "preflight":
		return RunPreflight(args[1:])
	case "validate":
		return RunValidate(args[1:])
	case "apply-patch":
		return RunApplyPatch(args[1:])
	case "compat-fix":
		return RunCompatFix(args[1:])
	case "upgrade-core":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup upgrade-core <target-version> [--dry-run]")
		}
		return RunUpgradeCore(args[1:])
	case "test-backup-create":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup test-backup-create <path>")
		}
		return RunTestBackupCreate(args[1])
	case "test-backup-list":
		return RunTestBackupList(args[1:])
	case "test-backup-restore":
		if len(args) < 3 || args[2] != "--confirm" {
			return fmt.Errorf("usage: drup test-backup-restore <path> <backup-id> --confirm")
		}
		return RunTestBackupRestore(args[1], args[2])
	case "test-backup-delete":
		if len(args) < 3 {
			return fmt.Errorf("usage: drup test-backup-delete <path> <backup-id>")
		}
		return RunTestBackupDelete(args[1], args[2])
	case "cleanup":
		if len(args) < 2 {
			return fmt.Errorf("usage: drup cleanup <project-path> [--validate-passed|--validate-failed]")
		}
		return RunCleanup(args[1:])
	default:
		return fmt.Errorf("unknown command %q — run 'drup help' for available commands", args[0])
	}
}

func printUsage() {
	fmt.Printf("drup %s — Drupal Upgrade Automation\n\n", Version)
	fmt.Println(`Usage:
  drup <command> [arguments]

Commands:
  init                  Initialize a Drupal project for upgrade automation
  scan <path>           Run upgrade_status:checkstyle and output structured JSON
  fix <path>            Run drupal-rector on the target project
  contrib <module>      Check Drupal.org for D11 compatibility
  issue <module_or_nid> Extract patch/diff/MR links from Drupal.org issues
  report <path>         Generate JSON and markdown reports
  mcp                   Start MCP stdio server
  install               Detect agents and write skill files
  uninstall             Remove drup from all installed agents
  sync                  Re-apply agent assets
  upgrade               Self-update binary
  preflight [path] [--allow-dirty]  Check project readiness for upgrade automation
  validate <path> [mod] Re-run scan and return error state (exit 1 if errors)
  compat-fix <path>     Declare Drupal 11 support in custom modules and themes
  apply-patch <url> <p> Download and apply a patch to the project
	upgrade-core <ver>    Upgrade Drupal core to target major version
	 test-backup-create <p> Create a testing backup before mutations
	 test-backup-list <p>   List testing backups for a project
	 test-backup-restore <p> <id> --confirm Restore a testing backup
	 test-backup-delete <p> <id> Delete a testing backup after success
  cleanup <path>        Post-validation cleanup (remove upgrade_status)
  version               Print version
  help                  Show this help message

Exit codes:
  0  success
  1  errors found
  2  usage error
  3  network/external tool failure`)
}
