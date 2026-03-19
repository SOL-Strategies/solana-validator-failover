package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// serveRelease starts a test server that returns the given tag_name.
func serveRelease(t *testing.T, tagName string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			json.NewEncoder(w).Encode(map[string]string{"tag_name": tagName}) //nolint:errcheck
		}
	}))
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current   string
		candidate string
		want      bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.2.3", "1.2.2", false},
		{"2.0.0", "1.9.9", false},
		{"1.0.0", "1.0.0-rc1", false},
		{"1.0.0-rc1", "1.0.0", false}, // pre-release strips suffix, treats as 1.0.0 == 1.0.0
		{"dev", "1.0.0", true},        // dev parses as 0.0.0
	}

	for _, tt := range tests {
		got := isNewerVersion(tt.current, tt.candidate)
		assert.Equal(t, tt.want, got, "isNewerVersion(%q, %q)", tt.current, tt.candidate)
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.0.0", [3]int{0, 0, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.2.3-rc1", [3]int{1, 2, 3}},
		{"1.2", [3]int{1, 2, 0}},
		{"1", [3]int{1, 0, 0}},
		{"dev", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		got := parseSemver(tt.input)
		assert.Equal(t, tt.want, got, "parseSemver(%q)", tt.input)
	}
}

func TestStartBackgroundCheck_DevSkips(t *testing.T) {
	ch := StartBackgroundCheck("dev")
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed for dev builds")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel was not closed promptly for dev build")
	}
}

func TestFetchLatestVersion_NewerAvailable(t *testing.T) {
	srv := serveRelease(t, "v1.5.0", http.StatusOK)
	defer srv.Close()

	latest, err := fetchLatestVersion(srv.URL, "1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "1.5.0", latest)
}

func TestFetchLatestVersion_AlreadyLatest(t *testing.T) {
	srv := serveRelease(t, "v1.0.0", http.StatusOK)
	defer srv.Close()

	latest, err := fetchLatestVersion(srv.URL, "1.0.0")
	assert.NoError(t, err)
	assert.Empty(t, latest)
}

func TestFetchLatestVersion_OlderThanCurrent(t *testing.T) {
	srv := serveRelease(t, "v0.9.0", http.StatusOK)
	defer srv.Close()

	latest, err := fetchLatestVersion(srv.URL, "1.0.0")
	assert.NoError(t, err)
	assert.Empty(t, latest)
}

func TestFetchLatestVersion_NonOKStatus(t *testing.T) {
	srv := serveRelease(t, "", http.StatusNotFound)
	defer srv.Close()

	latest, err := fetchLatestVersion(srv.URL, "1.0.0")
	assert.Error(t, err)
	assert.Empty(t, latest)
}

func TestFetchLatestVersion_Unreachable(t *testing.T) {
	latest, err := fetchLatestVersion("http://127.0.0.1:1", "1.0.0")
	assert.Error(t, err)
	assert.Empty(t, latest)
}

func TestPrintWarningIfAvailable_NilChannel(t *testing.T) {
	// should not panic
	PrintWarningIfAvailable(nil)
}

func TestPrintWarningIfAvailable_NoUpdate(t *testing.T) {
	ch := make(chan string, 1)
	close(ch)
	// should not panic or block
	PrintWarningIfAvailable(ch)
}

func TestPrintWarningIfAvailable_Timeout(t *testing.T) {
	ch := make(chan string) // never sends
	start := time.Now()
	PrintWarningIfAvailable(ch)
	assert.WithinDuration(t, start.Add(waitTimeout), time.Now(), 500*time.Millisecond)
}
