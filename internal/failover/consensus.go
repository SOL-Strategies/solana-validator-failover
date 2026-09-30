package failover

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// ConsensusTower and ConsensusAlpenglow are the supported validator consensus modes.
const (
	ConsensusTower     = "tower"
	ConsensusAlpenglow = "alpenglow"

	// AlpenglowFeatureAccount is the feature-gate account used to identify migration.
	AlpenglowFeatureAccount = "A1pengvuM6JEcyNuTnMqepBKhwHE3N6PmUrdATGawhJS"
	// AlpenglowGenesisCertificateAccount is the account checked by Agave's bank for Alpenglow state.
	AlpenglowGenesisCertificateAccount = "42Ym56TQ7AKFThRv5nyoSABq5ZPgE2TLKo8BALZEtoU1"
)

// ConsensusState is the consensus phase detected from a validator's finalized local bank.
type ConsensusState struct {
	Mode        string
	GenesisSlot uint64
}

// DetectConsensus determines Tower, migration, or Alpenglow from the local validator RPC.
func DetectConsensus(rpcURL string) (ConsensusState, error) {
	client := rpc.New(rpcURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	featureKey, err := solanago.PublicKeyFromBase58(AlpenglowFeatureAccount)
	if err != nil {
		return ConsensusState{}, err
	}
	featureData, featureExists, featureSlot, err := getConsensusAccount(ctx, client, featureKey, nil)
	if err != nil {
		return ConsensusState{}, fmt.Errorf("cannot read Alpenglow feature account from local RPC: %w", err)
	}
	featureActive, err := decodeFeatureActivation(featureData, featureExists)
	if err != nil {
		return ConsensusState{}, fmt.Errorf("cannot decode Alpenglow feature account: %w", err)
	}

	certificateKey, err := solanago.PublicKeyFromBase58(AlpenglowGenesisCertificateAccount)
	if err != nil {
		return ConsensusState{}, err
	}
	certificateData, certificateExists, _, err := getConsensusAccount(ctx, client, certificateKey, &featureSlot)
	if err != nil {
		return ConsensusState{}, fmt.Errorf("cannot read Alpenglow certificate account from local RPC: %w", err)
	}
	if certificateExists && len(certificateData) == 0 {
		return ConsensusState{}, fmt.Errorf("Alpenglow certificate account exists but has no data")
	}

	switch {
	case !featureActive && !certificateExists:
		return ConsensusState{Mode: ConsensusTower}, nil
	case featureActive && !certificateExists:
		return ConsensusState{Mode: "migrating"}, nil
	case !featureActive && certificateExists:
		return ConsensusState{}, fmt.Errorf("Alpenglow certificate exists while its feature account is inactive")
	default:
		var response json.RawMessage
		if err := client.RPCCallForInto(ctx, &response, "getAgGenesisCert", nil); err != nil {
			return ConsensusState{}, fmt.Errorf("cannot read Alpenglow genesis slot from local RPC: %w", err)
		}
		if bytes.Equal(bytes.TrimSpace(response), []byte("null")) {
			return ConsensusState{}, fmt.Errorf("Alpenglow certificate account exists but getAgGenesisCert returned null")
		}
		var certificate struct {
			Block *struct {
				Slot *uint64 `json:"slot"`
			} `json:"block"`
		}
		if err := json.Unmarshal(response, &certificate); err != nil {
			return ConsensusState{}, fmt.Errorf("cannot decode Alpenglow genesis certificate: %w", err)
		}
		if certificate.Block == nil || certificate.Block.Slot == nil {
			return ConsensusState{}, fmt.Errorf("Alpenglow genesis certificate has no block slot")
		}
		return ConsensusState{Mode: ConsensusAlpenglow, GenesisSlot: *certificate.Block.Slot}, nil
	}
}

// ValidateConsensus confirms that the declared mode matches the detected local phase.
func ValidateConsensus(rpcURL, declaredMode string) error {
	state, err := DetectConsensus(rpcURL)
	if err != nil {
		return err
	}
	if state.Mode == "migrating" {
		return fmt.Errorf("local validator is migrating to Alpenglow; failover is disabled during migration")
	}
	if state.Mode != declaredMode {
		return fmt.Errorf("configured consensus %q disagrees with local RPC consensus %q", declaredMode, state.Mode)
	}
	return nil
}

func getConsensusAccount(ctx context.Context, client *rpc.Client, key solanago.PublicKey, minContextSlot *uint64) ([]byte, bool, uint64, error) {
	result, err := client.GetAccountInfoWithOpts(ctx, key, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentFinalized, Encoding: solanago.EncodingBase64, MinContextSlot: minContextSlot,
	})
	if errors.Is(err, rpc.ErrNotFound) {
		return nil, false, 0, nil
	}
	if err != nil {
		return nil, false, 0, err
	}
	if result == nil || result.Value == nil || result.Value.Data == nil {
		return nil, false, 0, fmt.Errorf("RPC returned an incomplete account response")
	}
	return result.Value.Data.GetBinary(), true, result.Context.Slot, nil
}

func decodeFeatureActivation(data []byte, exists bool) (bool, error) {
	if !exists {
		return false, nil
	}
	switch {
	case len(data) == 1 && data[0] == 0:
		return false, nil
	case len(data) == 9 && data[0] == 1:
		_ = binary.LittleEndian.Uint64(data[1:]) // Validate the bincode Option<u64> payload.
		return true, nil
	default:
		var tag byte
		if len(data) > 0 {
			tag = data[0]
		}
		return false, fmt.Errorf("unexpected feature account data length/tag: %d/%d", len(data), tag)
	}
}
