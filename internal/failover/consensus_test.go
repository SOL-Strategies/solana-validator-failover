package failover

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/solana-foundation/solana-go/v2/rpc/jsonrpc"
	"github.com/stretchr/testify/require"
)

type rpcRequest struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func consensusRPC(t *testing.T, featureData, certificateData []byte, certMethod bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "getAccountInfo":
			var key string
			if err := json.Unmarshal(request.Params[0], &key); err != nil {
				t.Errorf("decode account key: %v", err)
				return
			}
			var value any
			switch key {
			case AlpenglowFeatureAccount:
				if featureData != nil {
					value = map[string]any{"lamports": 1, "owner": "11111111111111111111111111111111", "data": []any{base64.StdEncoding.EncodeToString(featureData), "base64"}, "executable": false, "rentEpoch": 0}
				}
			case AlpenglowGenesisCertificateAccount:
				if certificateData != nil {
					value = map[string]any{"lamports": 100000000, "owner": "11111111111111111111111111111111", "data": []any{base64.StdEncoding.EncodeToString(certificateData), "base64"}, "executable": false, "rentEpoch": uint64(math.MaxUint64), "space": len(certificateData)}
				}
			}
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","result":{"context":{"slot":100},"value":%s},"id":1}`, mustJSON(t, value))
		case "getAgGenesisCert":
			if len(certificateData) == 0 {
				t.Error("getAgGenesisCert called without certificate data")
			}
			if !certMethod {
				_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":1}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","result":{"block":{"slot":99,"blockId":"11111111111111111111111111111111"},"signature":{}},"id":1}`)
		default:
			t.Errorf("unexpected RPC method %s", request.Method)
		}
	}))
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDetectConsensus(t *testing.T) {
	activeFeature := make([]byte, 9)
	activeFeature[0] = 1
	binary.LittleEndian.PutUint64(activeFeature[1:], 100)
	for _, tc := range []struct {
		name                 string
		feature, certificate []byte
		certMethod           bool
		wantMode             string
		wantGenesis          uint64
		wantErr              bool
	}{
		{name: "tower missing feature", wantMode: ConsensusTower},
		{name: "tower inactive feature", feature: []byte{0}, wantMode: ConsensusTower},
		{name: "mainnet prefunded certificate", certificate: []byte{}, wantMode: ConsensusTower},
		{name: "inactive feature with prefunded certificate", feature: []byte{0}, certificate: []byte{}, wantMode: ConsensusTower},
		{name: "migrating", feature: activeFeature, wantMode: "migrating"},
		{name: "migrating with prefunded certificate", feature: activeFeature, certificate: []byte{}, wantMode: "migrating"},
		{name: "alpenglow", feature: activeFeature, certificate: []byte{1}, certMethod: true, wantMode: ConsensusAlpenglow, wantGenesis: 99},
		{name: "contradictory certificate missing feature", certificate: []byte{1}, wantErr: true},
		{name: "contradictory certificate inactive feature", feature: []byte{0}, certificate: []byte{1}, wantErr: true},
		{name: "certificate rpc unsupported", feature: activeFeature, certificate: []byte{1}, wantErr: true},
		{name: "prefunded feature", feature: []byte{}, wantMode: ConsensusTower},
		{name: "mainnet prefunded feature and certificate", feature: []byte{}, certificate: []byte{}, wantMode: ConsensusTower},
		{name: "contradictory certificate prefunded feature", feature: []byte{}, certificate: []byte{1}, wantErr: true},
		{name: "malformed feature", feature: []byte{2}, wantErr: true},
		{name: "pending feature", feature: make([]byte, 9), wantMode: ConsensusTower},
		{name: "pending feature with prefunded certificate", feature: make([]byte, 9), certificate: []byte{}, wantMode: ConsensusTower},
		{name: "contradictory certificate pending feature", feature: make([]byte, 9), certificate: []byte{1}, wantErr: true},
		{name: "active feature with trailing bytes", feature: append(append([]byte{}, activeFeature...), 0), wantMode: "migrating"},
		{name: "truncated active feature", feature: []byte{1, 0, 0}, wantErr: true},
		{name: "malformed feature with padding", feature: append([]byte{2}, make([]byte, 8)...), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := consensusRPC(t, tc.feature, tc.certificate, tc.certMethod)
			defer srv.Close()
			state, err := DetectConsensus(srv.URL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DetectConsensus() error = %v; wantErr %v", err, tc.wantErr)
			}
			if tc.name == "certificate rpc unsupported" {
				require.ErrorContains(t, err, "local RPC method getAgGenesisCert is unavailable")
				require.ErrorContains(t, err, "validator version supports the Alpenglow")
			}
			if err == nil && (state.Mode != tc.wantMode || state.GenesisSlot != tc.wantGenesis) {
				t.Fatalf("state = %+v, want mode %q genesis %d", state, tc.wantMode, tc.wantGenesis)
			}
		})
	}
}

func TestValidateConsensusMissingAccountMethod(t *testing.T) {
	for _, mode := range []string{ConsensusAlpenglow} {
		for _, failOnRequest := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/account%d", mode, failOnRequest), func(t *testing.T) {
				requests := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request rpcRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					require.Equal(t, "getAccountInfo", request.Method)
					requests++
					w.Header().Set("Content-Type", "application/json")
					if requests == failOnRequest {
						_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":1}`)
						return
					}
					_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","result":{"context":{"slot":100},"value":null},"id":1}`)
				}))
				defer srv.Close()

				err := ValidateConsensus(srv.URL, mode)
				require.ErrorContains(t, err, "local RPC method getAccountInfo is unavailable")
				require.ErrorContains(t, err, "--full-rpc-api")
				require.ErrorContains(t, err, "[rpc] full_api = true")
				var rpcErr *jsonrpc.RPCError
				require.ErrorAs(t, err, &rpcErr)
				require.Equal(t, -32601, rpcErr.Code)
				require.Equal(t, failOnRequest, requests)
			})
		}
	}
}

func TestValidateConsensus(t *testing.T) {
	activeFeature := make([]byte, 9)
	activeFeature[0] = 1
	binary.LittleEndian.PutUint64(activeFeature[1:], 100)
	for _, tc := range []struct {
		name, mode           string
		feature, certificate []byte
		wantErr              bool
	}{
		{name: "tower", mode: ConsensusTower, wantErr: true},
		{name: "mainnet prefunded certificate", mode: ConsensusTower, certificate: []byte{}, wantErr: true},
		{name: "prefunded certificate mode mismatch", mode: ConsensusAlpenglow, certificate: []byte{}, wantErr: true},
		{name: "tower mismatch", mode: ConsensusAlpenglow, wantErr: true},
		{name: "alpenglow", mode: ConsensusAlpenglow, feature: activeFeature, certificate: []byte{1}},
		{name: "alpenglow mismatch", mode: ConsensusTower, feature: activeFeature, certificate: []byte{1}, wantErr: true},
		{name: "migration rejected", mode: ConsensusAlpenglow, feature: activeFeature, wantErr: true},
		{name: "prefunded certificate migration rejected for tower", mode: ConsensusTower, feature: activeFeature, certificate: []byte{}, wantErr: true},
		{name: "prefunded certificate migration rejected for alpenglow", mode: ConsensusAlpenglow, feature: activeFeature, certificate: []byte{}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := consensusRPC(t, tc.feature, tc.certificate, true)
			defer srv.Close()
			err := ValidateConsensus(srv.URL, tc.mode)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateConsensus() error = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}
