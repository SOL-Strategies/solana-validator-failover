package failover

const (
	// ProtocolName is the name of the QUIC protocol
	ProtocolName = "solana-validator-failover"

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
)

// hookEnvMapParams is the parameters for the hook environment map
type hookEnvMapParams struct {
	isDryRunFailover bool
	isPreFailover    bool
	isPostFailover   bool
}
