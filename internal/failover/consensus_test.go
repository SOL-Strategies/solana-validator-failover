package failover

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type rpcRequest struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

type consensusRPCOpts struct {
	feature, certificate, emptyCertificate, certMethod bool
}

func consensusRPC(t *testing.T, opts consensusRPCOpts) *httptest.Server {
	t.Helper()
	feature, certificate, emptyCertificate, certMethod := opts.feature, opts.certificate, opts.emptyCertificate, opts.certMethod
	featureData := []byte{0}
	if feature {
		featureData = make([]byte, 9)
		featureData[0] = 1
		binary.LittleEndian.PutUint64(featureData[1:], 100)
	}
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
				value = map[string]any{"lamports": 1, "owner": "11111111111111111111111111111111", "data": []any{base64.StdEncoding.EncodeToString(featureData), "base64"}, "executable": false, "rentEpoch": 0}
			case AlpenglowGenesisCertificateAccount:
				switch {
				case emptyCertificate:
					value = map[string]any{"lamports": 100000000, "owner": "11111111111111111111111111111111", "data": []any{"", "base64"}, "executable": false, "rentEpoch": 0}
				case certificate:
					value = map[string]any{"lamports": 1, "owner": "11111111111111111111111111111111", "data": []any{"AQ==", "base64"}, "executable": false, "rentEpoch": 0}
				}
			}
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","result":{"context":{"slot":100},"value":%s},"id":1}`, mustJSON(t, value))
		case "getAgGenesisCert":
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
	for _, tc := range []struct {
		name                                               string
		feature, certificate, emptyCertificate, certMethod bool
		wantMode                                           string
		wantGenesis                                        uint64
		wantErr                                            bool
	}{
		{name: "tower", certMethod: true, wantMode: ConsensusTower},
		{name: "migrating", feature: true, certMethod: true, wantMode: "migrating"},
		{name: "alpenglow", feature: true, certificate: true, certMethod: true, wantMode: ConsensusAlpenglow, wantGenesis: 99},
		{name: "contradictory certificate", certificate: true, certMethod: true, wantErr: true},
		{name: "certificate rpc unsupported", feature: true, certificate: true, wantErr: true},
		{name: "empty funded certificate account", emptyCertificate: true, certMethod: true, wantMode: ConsensusTower},
		{name: "empty funded certificate account while migrating", feature: true, emptyCertificate: true, certMethod: true, wantMode: "migrating"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := consensusRPC(t, consensusRPCOpts{feature: tc.feature, certificate: tc.certificate, emptyCertificate: tc.emptyCertificate, certMethod: tc.certMethod})
			defer srv.Close()
			state, err := DetectConsensus(srv.URL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DetectConsensus() error = %v; wantErr %v", err, tc.wantErr)
			}
			if err == nil && (state.Mode != tc.wantMode || state.GenesisSlot != tc.wantGenesis) {
				t.Fatalf("state = %+v, want mode %q genesis %d", state, tc.wantMode, tc.wantGenesis)
			}
		})
	}
}

func TestValidateConsensus(t *testing.T) {
	for _, tc := range []struct {
		name, mode                                      string
		feature, certificate, emptyCertificate, wantErr bool
	}{
		{name: "tower", mode: ConsensusTower},
		{name: "tower mismatch", mode: ConsensusAlpenglow, wantErr: true},
		{name: "alpenglow", mode: ConsensusAlpenglow, feature: true, certificate: true},
		{name: "alpenglow mismatch", mode: ConsensusTower, feature: true, certificate: true, wantErr: true},
		{name: "migration rejected", mode: ConsensusAlpenglow, feature: true, wantErr: true},
		{name: "tower with empty funded certificate account", mode: ConsensusTower, emptyCertificate: true},
		{name: "migration rejected with empty funded certificate account", mode: ConsensusAlpenglow, feature: true, emptyCertificate: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := consensusRPC(t, consensusRPCOpts{feature: tc.feature, certificate: tc.certificate, emptyCertificate: tc.emptyCertificate, certMethod: true})
			defer srv.Close()
			err := ValidateConsensus(srv.URL, tc.mode)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateConsensus() error = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}
