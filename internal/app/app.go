package app

import "fmt"

// Version is set at build time via ldflags.
var Version = "dev"

// Run dispatches CLI commands based on args[0].
func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
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
	case "sync":
		return RunSync()
	case "upgrade":
		return RunUpgrade()
	default:
		return fmt.Errorf("unknown command %q — run 'drup help' for available commands", args[0])
	}
}

func printUsage() {
	fmt.Println(`drup — Drupal Upgrade Automation

Usage:
  drup <command> [arguments]

Commands:
  init                  Initialize a Drupal project for upgrade automation
  scan <path>           Run upgrade_status:analyze and output structured JSON
  fix <path>            Run drupal-rector on the target project
  contrib <module>      Check Drupal.org for D11 compatibility
  issue <module_or_nid> Extract patch/diff/MR links from Drupal.org issues
  report <path>         Generate JSON and markdown reports
  mcp                   Start MCP stdio server
  install               Detect agents and write skill files
  sync                  Re-apply agent assets
  upgrade               Self-update binary
  version               Print version
  help                  Show this help message

Exit codes:
  0  success
  1  errors found
  2  usage error
  3  network/external tool failure`)
}
