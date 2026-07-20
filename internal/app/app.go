package app

import "fmt"

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
