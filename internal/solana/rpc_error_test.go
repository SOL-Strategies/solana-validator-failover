package solana

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/solana-foundation/solana-go/v2/rpc"
	"github.com/solana-foundation/solana-go/v2/rpc/jsonrpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWrapLocalRPCMethodError(t *testing.T) {
	for _, method := range []string{"getAccountInfo", "getClusterNodes", "getAgGenesisCert"} {
		t.Run(method, func(t *testing.T) {
			original := &jsonrpc.RPCError{Code: -32601, Message: "Method not found"}
			err := WrapLocalRPCMethodError(method, fmt.Errorf("request failed: %w", original))
			require.ErrorContains(t, err, "local RPC method "+method+" is unavailable")
			require.ErrorContains(t, err, "--full-rpc-api")
			require.ErrorContains(t, err, "[rpc] full_api = true")
			require.ErrorContains(t, err, "native Firedancer or an RPC proxy")
			require.ErrorIs(t, err, original)
			var rpcErr *jsonrpc.RPCError
			require.ErrorAs(t, err, &rpcErr)
			require.Same(t, original, rpcErr)
			if method == "getAgGenesisCert" {
				require.ErrorContains(t, err, "validator version supports the Alpenglow")
			}
		})
	}

	for _, original := range []error{
		nil,
		context.DeadlineExceeded,
		errors.New("HTTP 401 Unauthorized"),
		errors.New("Method not found"),
		&jsonrpc.RPCError{Code: -32602, Message: "Invalid params"},
		&jsonrpc.RPCError{Code: -32005, Message: "Node is unhealthy"},
	} {
		require.Equal(t, original, WrapLocalRPCMethodError("getAccountInfo", original))
	}
}

func TestGossipClient_MissingLocalMethodStillFallsBack(t *testing.T) {
	client, localMock, networkMock := createTestClient()
	original := &jsonrpc.RPCError{Code: -32601, Message: "Method not found"}
	localMock.On("GetClusterNodes", mock.Anything).Return([]*rpc.GetClusterNodesResult{}, original)
	networkMock.On("GetClusterNodes", mock.Anything).Return([]*rpc.GetClusterNodesResult{{
		Pubkey: createTestPublicKey(1), Gossip: stringPtr("192.168.1.100:8001"),
	}}, nil)

	node, err := client.NodeFromIP("192.168.1.100")
	require.NoError(t, err)
	require.NotNil(t, node)
	localMock.AssertExpectations(t)
	networkMock.AssertExpectations(t)
}
