package solana

import (
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

	hint := "for Agave/Jito, start the local validator with --full-rpc-api; for Frankendancer (fdctl), set [rpc] full_api = true; for native Firedancer or an RPC proxy, verify the configured local endpoint supports this method"
	if method == "getAgGenesisCert" {
		hint += "; ensure the validator version supports the Alpenglow getAgGenesisCert RPC method"
	}
	return fmt.Errorf("local RPC method %s is unavailable: %w; %s", method, err, hint)
}
