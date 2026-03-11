package failover

import "fmt"

// ProtocolName is the QUIC ALPN identifier for this protocol.
// It encodes WireProtocolVersion so that nodes running different wire versions
// are rejected at the TLS handshake level before any stream data is exchanged.
// When WireProtocolVersion is bumped, ProtocolName changes automatically.
var ProtocolName = fmt.Sprintf("solana-validator-failover/%d", WireProtocolVersion)

const (
	// DefaultPort is the default port for the QUIC server
	DefaultPort = 9898

	// DefaultHeartbeatIntervalDurationStr is the default heartbeat interval duration string
	DefaultHeartbeatIntervalDurationStr = "5s"

	// DefaultStreamTimeoutDurationStr is the default stream timeout duration string
	DefaultStreamTimeoutDurationStr = "10m"

	// MessageTypeFailoverInitiateRequest is the message type for initiating a failover
	MessageTypeFailoverInitiateRequest byte = 1

	// MessageTypeFileTransfer is the message type for file transfer
	MessageTypeFileTransfer byte = 2

	// WireProtocolVersion is the binary framing version for QUIC streams.
	// Bump this whenever the stream framing or gob types change in a
	// backward-incompatible way (e.g. adding GobEncode/GobDecode to a type).
	// Both nodes must agree on this version before any gob work starts.
	//
	// History:
	//   1 = original (pre-v0.1.18) — no version byte, implicit
	//   2 = version byte added after msg_type / before first gob frame (v0.1.18+)
	WireProtocolVersion byte = 2
)

// hookEnvMapParams is the parameters for the hook environment map
type hookEnvMapParams struct {
	isDryRunFailover bool
	isPreFailover    bool
	isPostFailover   bool
}
