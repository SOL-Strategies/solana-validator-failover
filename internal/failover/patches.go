package failover

import (
	"fmt"
	"strings"
)

// IdentityTransitionPatchURL returns the release-specific patch index. Client
// family and version detection can be ambiguous, so operators select the exact
// Agave or Jito-Solana patch matching the validator source they build.
func IdentityTransitionPatchURL(_ string, releaseVersion string) string {
	if releaseVersion == "" || releaseVersion == "dev" {
		releaseVersion = "main"
	}
	if releaseVersion != "main" && !strings.HasPrefix(releaseVersion, "v") {
		releaseVersion = "v" + releaseVersion
	}
	return fmt.Sprintf("https://github.com/sol-strategies/solana-validator-failover/tree/%s/patches", releaseVersion)
}
