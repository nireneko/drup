package app

import (
	"fmt"

	"github.com/nireneko/drup/internal/coreupgrade"
	"github.com/nireneko/drup/internal/upgradeplan"
)

// coreUpgradeCatalog is deliberately keyed by the exact major jump. The
// 10-to-11 data is not a default for later upgrades: missing 11-to-12 data
// blocks planning until its independently versioned catalog is supplied.
func buildCoreUpgradePlan(projectPath, targetVersion string) (upgradeplan.Plan, error) {
	current, err := coreupgrade.CurrentMajor(projectPath)
	if err != nil {
		return upgradeplan.Plan{}, err
	}
	target, err := upgradeplan.ParseMajor(targetVersion)
	if err != nil {
		return upgradeplan.Plan{}, fmt.Errorf("parse target version %q: %w", targetVersion, err)
	}
	return upgradeplan.Build(current, target, upgradeplan.KnownCatalog())
}
