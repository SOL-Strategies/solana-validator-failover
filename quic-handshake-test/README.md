# QUIC Handshake Test

This is a minimal test program to debug QUIC handshake issues with quic-go 0.57.1 on Ubuntu 24.

## Purpose

Test whether quic-go 0.57.1 can establish QUIC connections on Ubuntu 24 servers, where versions 0.44.0+ have been failing with "timeout: no recent network activity" errors.

## Usage

### On Server 1 (Passive/Server):
```bash
cd quic-handshake-test
go mod tidy
go run server.go
```

### On Server 2 (Active/Client):

**Standard client (DialAddr):**
```bash
cd quic-handshake-test
go mod tidy
go run client.go <server1-address>:9898
```

**Alternative client (explicit UDP connection):**
```bash
go run client-alt.go <server1-address>:9898
```

Example:
```bash
go run client.go solana-testnet-pengu-london-latitude.tailbd8d12.ts.net:9898
```

## Alternative Test Methods

If the standard `client.go` fails, try the alternative version:
- `client-alt.go` - Uses explicit UDP connection with `Dial()` instead of `DialAddr()`. This gives more control over the UDP socket and might work around Ubuntu 24 network stack issues.

These test different connection methods to see if any work around the Ubuntu 24 issue.

## What it tests

- Basic QUIC connection establishment
- Stream opening and data transfer
- Uses the same protocol name and port (9898) as the main application
- Uses similar TLS configuration (self-signed cert, InsecureSkipVerify)
- Tests with explicit quic.Config timeouts

## Expected behavior

If working correctly:
- Server should accept connection and stream
- Client should connect, send data, receive response
- Both should complete without errors

If failing:
- Client will timeout during handshake with "timeout: no recent network activity"
- This indicates the Ubuntu 24 compatibility issue persists

## Notes

- Uses quic-go v0.57.1 (latest)
- Port 9898 matches the main application
- Protocol name "solana-validator-failover" matches the main application
- Self-signed TLS certificates (like the main application)

