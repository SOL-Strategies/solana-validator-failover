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
