package failover

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWriteReadWireVersion_Match(t *testing.T) {
	var buf bytes.Buffer
	if err := writeWireVersion(&buf); err != nil {
		t.Fatalf("writeWireVersion: %v", err)
	}
	if err := readAndCheckWireVersion(&buf); err != nil {
		t.Fatalf("readAndCheckWireVersion: unexpected error: %v", err)
	}
}

func TestWriteReadWireVersion_Mismatch(t *testing.T) {
	// Simulate a peer running a different wire protocol version.
	oldVersion := WireProtocolVersion - 1
	buf := bytes.NewReader([]byte{oldVersion})

	err := readAndCheckWireVersion(buf)
	if err == nil {
		t.Fatal("expected an error on version mismatch, got nil")
	}

	var mismatch *WireVersionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("expected *WireVersionMismatchError, got %T: %v", err, err)
	}
	if mismatch.Ours != WireProtocolVersion {
		t.Errorf("Ours: got %d, want %d", mismatch.Ours, WireProtocolVersion)
	}
	if mismatch.Theirs != oldVersion {
		t.Errorf("Theirs: got %d, want %d", mismatch.Theirs, oldVersion)
	}
}

func TestReadWireVersion_EOF(t *testing.T) {
	err := readAndCheckWireVersion(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected an error on empty reader, got nil")
	}
	// io.ReadFull returns io.EOF when zero bytes are read (our case: 1-byte
	// read from an empty reader). io.ErrUnexpectedEOF is only used when some
	// but not all bytes are read.
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF in error chain, got: %v", err)
	}
}

func TestWireVersionMismatchError_Message(t *testing.T) {
	e := &WireVersionMismatchError{Ours: 2, Theirs: 1}
	msg := e.Error()

	for _, want := range []string{"v1", "v2", "solana-validator-failover"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not contain %q", msg, want)
		}
	}
}
