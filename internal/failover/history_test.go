package failover

import (
	"crypto/ed25519"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/stretchr/testify/require"
)

func signedHistory(t *testing.T, votes []uint64, root uint64) ([]byte, string) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	identity := solanago.PublicKeyFromBytes(key.Public().(ed25519.PublicKey))
	body := append([]byte{}, identity[:]...)
	put := func(n uint64) { body = binary.LittleEndian.AppendUint64(body, n) }
	put(uint64(len(votes)))
	for _, slot := range votes {
		put(slot)
	}
	for i := 0; i < 8; i++ {
		put(0)
	}
	put(root)
	history := make([]byte, 76)
	copy(history[4:68], ed25519.Sign(key, body))
	binary.LittleEndian.PutUint64(history[68:], uint64(len(body)))
	return append(history, body...), identity.String()
}

func TestHistoryWatermark(t *testing.T) {
	large := make([]uint64, 5000)
	for i := range large {
		large[i] = uint64(i + 1)
	}
	for _, tc := range []struct {
		name       string
		votes      []uint64
		root, want uint64
	}{
		{"native root", nil, 123, 123}, {"highest own vote", []uint64{9, 50, 12}, 20, 50}, {"larger than old limit", large, 100, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, identity := signedHistory(t, tc.votes, tc.root)
			path := filepath.Join(t.TempDir(), "history.bin")
			require.NoError(t, os.WriteFile(path, data, 0600))
			got, err := ReadHistoryWatermark(path, identity)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
func TestHistoryRejectsMalformedAndWrongIdentity(t *testing.T) {
	data, identity := signedHistory(t, nil, 123)
	for _, tc := range []struct {
		name     string
		change   func([]byte) []byte
		identity string
	}{
		{"truncated header", func(b []byte) []byte { return b[:40] }, identity},
		{"version", func(b []byte) []byte { b[0] = 1; return b }, identity},
		{"bad signed length", func(b []byte) []byte { binary.LittleEndian.PutUint64(b[68:], 1<<40); return b }, identity},
		{"collection overflow", func(b []byte) []byte { binary.LittleEndian.PutUint64(b[108:], ^uint64(0)); return b }, identity},
		{"wrong identity", func(b []byte) []byte { return b }, "11111111111111111111111111111111"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.bin")
			require.NoError(t, os.WriteFile(path, tc.change(append([]byte{}, data...)), 0600))
			_, err := ReadHistoryWatermark(path, tc.identity)
			require.Error(t, err)
		})
	}
}
func TestOutgoingHistorySelectsOnlyDemotedNativeFile(t *testing.T) {
	n := NodeInfo{IsNativeFiredancer: true, VoteHistoryFile: filepath.Join(t.TempDir(), "history.bin")}
	require.NoError(t, os.WriteFile(n.VoteHistoryFile, []byte("old live file"), 0600))
	_, err := captureStateFile(n.OutgoingHistoryFile(false))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Equal(t, n.VoteHistoryFile, n.OutgoingHistoryFile(true))
	n.IsNativeFiredancer = false
	require.Equal(t, n.VoteHistoryFile, n.OutgoingHistoryFile(false))
}
func TestMinimumFiredancerVersion(t *testing.T) {
	for _, version := range []string{"26.10.3", "v26.10.3", "26.10.3+build", "26.10.4 (commit)", "27.1.0"} {
		require.NoError(t, ValidateFiredancerVersion(version), version)
	}
	for _, version := range []string{"26.10.2", "26.10.0", "26.9.9", "26.10.3-rc1", "26.10.4-dev", "", "agave 4.3.2"} {
		require.Error(t, ValidateFiredancerVersion(version), version)
	}
}

func TestRepeatedNativeDemotionCapturesLatestHistory(t *testing.T) {
	n := NodeInfo{IsNativeFiredancer: true, VoteHistoryFile: filepath.Join(t.TempDir(), "history.bin")}
	for _, root := range []uint64{100, 200} {
		data, identity := signedHistory(t, nil, root)
		require.NoError(t, os.WriteFile(n.VoteHistoryFile, data, 0600))
		require.NoError(t, os.Rename(n.VoteHistoryFile, n.OutgoingHistoryFile(false)))
		snapshot, err := captureStateFile(n.OutgoingHistoryFile(false))
		require.NoError(t, err)
		got, err := readHistoryWatermark(snapshot, identity)
		require.NoError(t, err)
		require.Equal(t, root, got)
		require.NoError(t, snapshot.Close())
		require.NoError(t, os.Remove(snapshot.Name()))
	}
}

func TestCaptureOutgoingHistoryFreshness(t *testing.T) {
	demotedAt := time.Unix(1700000000, 0)
	for _, tc := range []struct {
		name           string
		native, dryRun bool
		modified       time.Time
		wantErr        bool
	}{
		{"native fresh backup", true, false, demotedAt.Add(time.Second), false},
		{"native backup at demotion", true, false, demotedAt, false},
		{"native stale backup", true, false, demotedAt.Add(-time.Second), true},
		{"native dry run uses existing live history", true, true, demotedAt.Add(-time.Hour), false},
		{"agave uses existing live history", false, false, demotedAt.Add(-time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := NodeInfo{IsNativeFiredancer: tc.native, VoteHistoryFile: filepath.Join(t.TempDir(), "history.bin")}
			data, identity := signedHistory(t, nil, 100)
			path := n.OutgoingHistoryFile(tc.dryRun)
			require.NoError(t, os.WriteFile(path, data, 0600))
			require.NoError(t, os.Chtimes(path, tc.modified, tc.modified))
			snapshot, err := n.captureOutgoingHistory(tc.dryRun, demotedAt)
			if tc.wantErr {
				require.ErrorContains(t, err, "refusing stale vote history")
				require.Nil(t, snapshot)
				return
			}
			require.NoError(t, err)
			defer os.Remove(snapshot.Name())
			defer snapshot.Close()
			got, err := readHistoryWatermark(snapshot, identity)
			require.NoError(t, err)
			require.Equal(t, uint64(100), got)
		})
	}
}
