package identities

import (
	"fmt"

	"github.com/charmbracelet/log"
)

// Identities holds the information for the identities
type Identities struct {
	Active  *Identity
	Passive *Identity
}

// NewFromConfig creates a new identities from a config.
// Each identity can be loaded from a keypair file or from a base58 pubkey string.
// Keypair file takes precedence if both are provided.
func NewFromConfig(cfg *Config) (identities *Identities, err error) {
	logger := log.WithPrefix("identities")
	identities = &Identities{}

	// load active identity
	if cfg.Active != "" {
		logger.Debug("loading active identity from keypair file", "file", cfg.Active)
		identities.Active, err = NewIdentityFromFile(cfg.Active)
		if err != nil {
			return nil, fmt.Errorf("failed to load active identity: %w", err)
		}
	} else if cfg.ActivePubkey != "" {
		logger.Debug("loading active identity from pubkey string", "pubkey", cfg.ActivePubkey)
		identities.Active, err = NewIdentityFromPubkey(cfg.ActivePubkey)
		if err != nil {
			return nil, fmt.Errorf("failed to load active identity: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either identities.active (keypair file) or identities.active_pubkey (base58 pubkey) must be set")
	}

	// load passive identity
	if cfg.Passive != "" {
		logger.Debug("loading passive identity from keypair file", "file", cfg.Passive)
		identities.Passive, err = NewIdentityFromFile(cfg.Passive)
		if err != nil {
			return nil, fmt.Errorf("failed to load passive identity: %w", err)
		}
	} else if cfg.PassivePubkey != "" {
		logger.Debug("loading passive identity from pubkey string", "pubkey", cfg.PassivePubkey)
		identities.Passive, err = NewIdentityFromPubkey(cfg.PassivePubkey)
		if err != nil {
			return nil, fmt.Errorf("failed to load passive identity: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either identities.passive (keypair file) or identities.passive_pubkey (base58 pubkey) must be set")
	}

	// public keys must be different
	if identities.Active.PubKey() == identities.Passive.PubKey() {
		return nil, fmt.Errorf("active and passive identities must be different")
	}

	return
}
