package failover

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	identityTransitionPatchDirectory = "patches/agave/admin-rpc-identity-transition-status"
	identityTransitionPatchFile      = "0001-add-identity-transition-status-rpc.patch"
)

var validatorVersionPattern = regexp.MustCompile(`(?:^|[^0-9])v?([0-9]+)\.([0-9]+)(?:[^0-9]|$)`)

// IdentityTransitionPatchURL returns the immutable release URL for the
// Agave/Jito patch matching a validator's RPC version. Unknown versions point
// to the hosted patch index so operators can select the appropriate source.
func IdentityTransitionPatchURL(clientVersion, releaseVersion string) string {
	base := "https://raw.githubusercontent.com/sol-strategies/solana-validator-failover"
	if releaseVersion == "" || releaseVersion == "dev" {
		releaseVersion = "main"
	}
	if releaseVersion != "main" && !strings.HasPrefix(releaseVersion, "v") {
		releaseVersion = "v" + releaseVersion
	}
	match := validatorVersionPattern.FindStringSubmatch(clientVersion)
	if len(match) != 3 || match[1] != "4" || (match[2] != "2" && match[2] != "3") {
		return fmt.Sprintf("https://github.com/sol-strategies/solana-validator-failover/tree/%s/%s", releaseVersion, identityTransitionPatchDirectory)
	}
	line := fmt.Sprintf("v%s.x", match[1]+"."+match[2])
	return fmt.Sprintf("%s/%s/%s/%s/%s", base, releaseVersion, identityTransitionPatchDirectory, line, identityTransitionPatchFile)
}
