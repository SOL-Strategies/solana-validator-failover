package identities

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

// Identity holds the information for an identity.
// In full keypair mode, Key is populated from a file. In pubkey-only mode,
// only PubKeyStr is set and Key is nil.
type Identity struct {
	KeyFile   string // path to the identity key file (empty in pubkey-only mode)
	Key       solana.PrivateKey
	PubKeyStr string // base58 public key string (always populated)
}

// NewIdentityFromFile creates an Identity from a keypair file
func NewIdentityFromFile(keyFile string) (identity *Identity, err error) {
	logger := log.With().Str("component", "identities").Logger()
	// resolve path
	keyFileAbsolutePath, err := utils.ResolvePath(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	identity = &Identity{
		KeyFile: keyFileAbsolutePath,
	}

	logger.Debug().
		Str("file", keyFileAbsolutePath).
		Msg("reading solana keygen file")

	identity.Key, err = solana.PrivateKeyFromSolanaKeygenFile(keyFileAbsolutePath)
	if err != nil {
		err = fmt.Errorf("failed to parse keygen file: %w", err)
		return
	}

	identity.PubKeyStr = identity.Key.PublicKey().String()

	logger.Debug().
		Str("pubkey", identity.PubKeyStr).
		Str("file", keyFileAbsolutePath).
		Msg("parsed solana keygen file")

	return identity, nil
}

// NewIdentityFromPubkey creates an Identity from a base58 public key string.
// The identity operates in pubkey-only mode: no keypair file, no private key.
func NewIdentityFromPubkey(pubkey string) (identity *Identity, err error) {
	logger := log.With().Str("component", "identities").Logger()

	if _, err := solana.PublicKeyFromBase58(pubkey); err != nil {
		return nil, fmt.Errorf("failed to parse pubkey as base58 public key: %w", err)
	}

	identity = &Identity{
		PubKeyStr: pubkey,
	}

	logger.Debug().
		Str("pubkey", pubkey).
		Msg("loaded identity from pubkey string (pubkey-only mode)")

	return identity, nil
}

// Pubkey returns the public key of the identity - prefer its PascalCase counterpart PubKey
func (i *Identity) Pubkey() string {
	log.Warn().Msg("Pubkey is deprecated (but still works) in favour of PubKey - using it for you...")
	return i.PubKey()
}

// PubKey returns the public key string. If a keypair is loaded, derives from
// the private key; otherwise returns the configured pubkey string.
func (i *Identity) PubKey() string {
	if i.Key != nil {
		return i.Key.PublicKey().String()
	}
	return i.PubKeyStr
}

// identityWire is the gob-safe representation of Identity.
// It intentionally omits the Key field (solana.PrivateKey) to prevent
// private key material from being serialized over the wire (QUIC stream).
type identityWire struct {
	KeyFile   string
	PubKeyStr string
}

// GobEncode implements gob.GobEncoder to prevent private key serialization.
// Only KeyFile and PubKeyStr are transmitted; the Key field is excluded.
func (i Identity) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(identityWire{
		KeyFile:   i.KeyFile,
		PubKeyStr: i.PubKeyStr,
	}); err != nil {
		return nil, fmt.Errorf("failed to gob-encode identity: %w", err)
	}
	return buf.Bytes(), nil
}

// GobDecode implements gob.GobDecoder to reconstruct Identity without private key.
// After decoding, Key is nil — PubKey() will use PubKeyStr instead.
func (i *Identity) GobDecode(data []byte) error {
	var w identityWire
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("failed to gob-decode identity: %w", err)
	}
	i.KeyFile = w.KeyFile
	i.PubKeyStr = w.PubKeyStr
	i.Key = nil // explicitly nil — private key never comes from the wire
	return nil
}
