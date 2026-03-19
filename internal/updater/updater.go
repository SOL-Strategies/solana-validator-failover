package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/pkg/constants"
)

const (
	githubReleaseURL = "https://api.github.com/repos/sol-strategies/solana-validator-failover/releases/latest"
	checkTimeout     = 3 * time.Second
	waitTimeout      = 1500 * time.Millisecond
)

// StartBackgroundCheck fires a goroutine and returns a channel that will receive
// the latest version string if a newer one is available, or "" otherwise.
// Returns a closed channel if the check is skipped (dev build).
func StartBackgroundCheck(currentVersion string) chan string {
	ch := make(chan string, 1)

	if currentVersion == "dev" {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		latest, err := fetchLatestVersion(githubReleaseURL, currentVersion)
		if err == nil && latest != "" {
			ch <- latest
		}
	}()

	return ch
}

// PrintWarningIfAvailable waits up to waitTimeout for the channel and logs a
// warning if a newer version is available.
func PrintWarningIfAvailable(ch chan string) {
	if ch == nil {
		return
	}

	select {
	case latest, ok := <-ch:
		if ok && latest != "" {
			log.Warnf(
				"update available: v%s → %s/releases/latest",
				latest,
				constants.GitHubRepoURL,
			)
		}
	case <-time.After(waitTimeout):
		// timed out — skip silently
	}
}

// fetchLatestVersion calls the given URL (GitHub releases API endpoint), decodes
// the tag_name, and returns the version string (without "v" prefix) if it is
// newer than currentVersion. Returns an empty string and no error if up to date.
func fetchLatestVersion(url, currentVersion string) (string, error) {
	client := &http.Client{Timeout: checkTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", constants.AppName, currentVersion))
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	if isNewerVersion(currentVersion, latest) {
		return latest, nil
	}
	return "", nil
}

// isNewerVersion returns true if candidate is strictly greater than current
// using semver major.minor.patch ordering.
func isNewerVersion(current, candidate string) bool {
	c := parseSemver(current)
	n := parseSemver(candidate)
	for i := range c {
		if n[i] > c[i] {
			return true
		}
		if n[i] < c[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		// strip any pre-release suffix (e.g. "1-rc1")
		p, _, _ = strings.Cut(p, "-")
		out[i], _ = strconv.Atoi(p)
	}
	return out
}
