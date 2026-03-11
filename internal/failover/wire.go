package failover

import (
	"errors"
	"fmt"
	"io"

	"github.com/quic-go/quic-go"
)

// cryptoNoApplicationProtocol is the QUIC transport error code for TLS alert 120
// ("no_application_protocol"), sent when ALPN negotiation fails because the peer
// does not support any of the offered protocol names.
// QUIC maps TLS alerts to error codes as 0x100 + alert_code (RFC 9001 §4.8).
const cryptoNoApplicationProtocol = quic.TransportErrorCode(0x178)

// isALPNMismatch reports whether err is a QUIC ALPN negotiation failure.
// This happens when the client and server advertise different ProtocolName
// values, meaning they are running incompatible wire protocol versions.
func isALPNMismatch(err error) bool {
	var te *quic.TransportError
	return errors.As(err, &te) && te.ErrorCode == cryptoNoApplicationProtocol
}

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
