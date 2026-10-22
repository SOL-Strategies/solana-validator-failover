package failover

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
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

func TestDryRunHistoryDestinationUsesTemporarySibling(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "vote_history-active.bin")
	if err := os.WriteFile(destination, []byte("existing history"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.bin")
	content := []byte("current dry-run snapshot")
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}

	dryRunPath, err := createDryRunHistoryDestination(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(dryRunPath) }()

	if filepath.Dir(dryRunPath) != dir {
		t.Fatalf("dry-run path directory = %q, want %q", filepath.Dir(dryRunPath), dir)
	}
	if dryRunPath == destination {
		t.Fatal("dry-run path must not replace the configured destination")
	}
	var wire bytes.Buffer
	sender := &Stream{encoder: gob.NewEncoder(&wire)}
	if err := sender.sendStateFile(source); err != nil {
		t.Fatal(err)
	}
	receiver := &Stream{decoder: gob.NewDecoder(&wire)}
	if err := receiver.receiveStateFile(dryRunPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dryRunPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("dry-run transfer did not install the source snapshot")
	}
	if err := os.Remove(dryRunPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dryRunPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run destination still exists after cleanup: %v", err)
	}
	got, err = os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing history" {
		t.Fatalf("configured destination changed: %q", got)
	}
}
