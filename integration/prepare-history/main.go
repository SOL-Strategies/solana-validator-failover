// prepare-history writes signed Alpenglow history fixtures for the local demos.
package main

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	solanago "github.com/solana-foundation/solana-go/v2"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func prepare() error {
	key, err := solanago.PrivateKeyFromSolanaKeygenFile("integration/test-keys/active-identity.json")
	if err != nil {
		return err
	}
	pub := key.PublicKey()
	body := append([]byte{}, pub[:]...)
	for i := 0; i < 9; i++ {
		body = binary.LittleEndian.AppendUint64(body, 0)
	}
	body = binary.LittleEndian.AppendUint64(body, 290000000)
	history := make([]byte, 76)
	copy(history[4:68], ed25519.Sign(ed25519.PrivateKey(key), body))
	binary.LittleEndian.PutUint64(history[68:], uint64(len(body)))
	history = append(history, body...)
	for _, node := range []string{"chicago", "london"} {
		dir := filepath.Join("integration/test-ledgers", node)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "vote_history-"+pub.String()+".bin"), history, 0600); err != nil {
			return err
		}
	}
	return nil
}
