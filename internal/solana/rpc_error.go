package solana

import (
	"context"
	"errors"
	"fmt"

	"github.com/solana-foundation/solana-go/v2/rpc/jsonrpc"
)

// jsonRPCMethodNotFound is the JSON-RPC 2.0 code for "Method not found".
const jsonRPCMethodNotFound = -32601

// WrapLocalRPCMethodError adds configuration guidance only when the local RPC
// reports an unavailable method. It preserves the original error for inspection.
func WrapLocalRPCMethodError(method string, err error) error {
	var rpcErr *jsonrpc.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonRPCMethodNotFound {
		return err
	}

	hint := "for Agave/Jito, start the local validator with --full-rpc-api; for native Firedancer or an RPC proxy, verify the configured local endpoint supports this method"
	if method == "getAgGenesisCert" {
		hint += "; ensure the validator version supports the Alpenglow getAgGenesisCert RPC method"
	}
	return fmt.Errorf("local RPC method %s is unavailable: %w; %s", method, err, hint)
}

// networkEndpointError strips untrusted transport and server messages for secret URLs.
// Raw errors must not be wrapped: their messages and unwrap chains may expose secrets.
func (c *Client) networkEndpointError(index int, err error) error {
	label := ""
	if index < len(c.clusterRPCURLs) {
		label = c.clusterRPCURLLabels[c.clusterRPCURLs[index]]
	}
	if label == "" {
		return fmt.Errorf("endpoint %d: %w", index+1, err)
	}
	var safe error
	var rpcErr *jsonrpc.RPCError
	switch {
	case errors.Is(err, context.Canceled):
		safe = context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		safe = context.DeadlineExceeded
	case errors.As(err, &rpcErr):
		safe = fmt.Errorf("JSON-RPC failure (code %d)", rpcErr.Code)
	default:
		safe = errors.New("RPC failure")
	}
	return fmt.Errorf("endpoint %d (%s): %w", index+1, label, safe)
}
