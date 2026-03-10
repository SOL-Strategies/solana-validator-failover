package failover

import (
	"fmt"
	"io"
)

// WireVersionMismatchError is returned when the peer's wire protocol version
// does not match ours. It carries both versions for a clear error message.
type WireVersionMismatchError struct {
	Ours   byte
	Theirs byte
}

func (e *WireVersionMismatchError) Error() string {
	return fmt.Sprintf(
		"wire protocol version mismatch: peer uses v%d, we use v%d — "+
			"both nodes must run the same version of solana-validator-failover",
		e.Theirs, e.Ours,
	)
}

// writeWireVersion writes WireProtocolVersion to w.
// Call this once per stream direction, before any gob encoding.
func writeWireVersion(w io.Writer) error {
	_, err := w.Write([]byte{WireProtocolVersion})
	return err
}

// readAndCheckWireVersion reads one byte from r and returns a
// *WireVersionMismatchError if it does not match WireProtocolVersion.
// Call this once per stream direction, before any gob decoding.
func readAndCheckWireVersion(r io.Reader) error {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("failed to read wire protocol version: %w", err)
	}
	if buf[0] != WireProtocolVersion {
		return &WireVersionMismatchError{Ours: WireProtocolVersion, Theirs: buf[0]}
	}
	return nil
}
