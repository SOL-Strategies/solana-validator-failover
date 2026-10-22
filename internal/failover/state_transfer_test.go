package failover

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVoteHistoryTransfer(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.bin")
	destination := filepath.Join(dir, "vote_history-test.bin")
	content := bytes.Repeat([]byte("signed vote history"), 150000)
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	sender := &Stream{encoder: gob.NewEncoder(&wire)}
	if err := sender.sendStateFile(source); err != nil {
		t.Fatal(err)
	}
	receiver := &Stream{decoder: gob.NewDecoder(&wire)}
	if err := receiver.receiveStateFile(destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("transferred history differs")
	}
}

func TestVoteHistoryTransferPreservesExistingOnTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vote_history-test.bin")
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	if err := gob.NewEncoder(&wire).Encode(stateHeader{Size: 100}); err != nil {
		t.Fatal(err)
	}
	receiver := &Stream{decoder: gob.NewDecoder(&wire)}
	if err := receiver.receiveStateFile(path); err == nil {
		t.Fatal("expected truncated transfer error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing" {
		t.Fatal("existing history changed")
	}
}

func TestVoteHistoryAfterDemotion(t *testing.T) {
	demotedAt := time.Now()
	write := func(t *testing.T, path string, mtime time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte("vote history"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		bin     *time.Time
		bak     *time.Time
		want    string
		wantErr bool
	}{
		{name: "agave keeps the bin", bin: ptr(demotedAt.Add(time.Second)), want: ".bin"},
		{name: "bin wins over a bak", bin: ptr(demotedAt.Add(time.Second)), bak: ptr(demotedAt.Add(-time.Hour)), want: ".bin"},
		{name: "firedancer saves a fresh bak", bak: ptr(demotedAt.Add(time.Millisecond)), want: ".bin.bak"},
		{name: "stale bak from an earlier switch", bak: ptr(demotedAt.Add(-time.Second)), wantErr: true},
		{name: "no vote history at all", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vote_history-test.bin")
			if tt.bin != nil {
				write(t, path, *tt.bin)
			}
			if tt.bak != nil {
				write(t, path+".bak", *tt.bak)
			}
			got, err := voteHistoryAfterDemotion(path, demotedAt)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != filepath.Join(filepath.Dir(path), "vote_history-test"+tt.want) {
				t.Fatalf("got %s, want suffix %s", got, tt.want)
			}
		})
	}
}

func ptr(t time.Time) *time.Time { return &t }
