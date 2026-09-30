package failover

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const stateChunkSize = 1024 * 1024

func stateLabel(consensus string) string {
	if consensus == "alpenglow" {
		return "vote history"
	}
	return "tower"
}

func preflightStateDestination(path string, size int64) error {
	if size <= 0 {
		return fmt.Errorf("invalid source state size %d", size)
	}
	dir := filepath.Dir(path)
	var fs syscall.Statfs_t
	if err := syscall.Statfs(dir, &fs); err != nil {
		return err
	}
	if uint64(size) > fs.Bavail*uint64(fs.Bsize) {
		return fmt.Errorf("insufficient free space in %s for %d-byte state file", dir, size)
	}
	f, err := os.CreateTemp(dir, ".vote-history-preflight-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

type stateHeader struct {
	Size   int64
	Digest [32]byte
}

type stateChunk struct{ Data []byte }

// sendStateFile streams a vote history with bounded memory and a digest.
func (s *Stream) sendStateFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("state file must be a nonempty regular file: %s", path)
	}
	digest := sha256.New()
	if _, err = io.Copy(digest, f); err != nil {
		return err
	}
	var header stateHeader
	header.Size = info.Size()
	copy(header.Digest[:], digest.Sum(nil))
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = s.encoder.Encode(header); err != nil {
		return err
	}
	buf := make([]byte, stateChunkSize)
	remaining := header.Size
	for remaining > 0 {
		n, readErr := io.ReadFull(f, buf[:min(int64(len(buf)), remaining)])
		if readErr != nil {
			return readErr
		}
		if err = s.encoder.Encode(stateChunk{Data: buf[:n]}); err != nil {
			return err
		}
		remaining -= int64(n)
	}
	return nil
}

// receiveStateFile installs a complete, verified vote history atomically.
func (s *Stream) receiveStateFile(path string) error {
	var header stateHeader
	if err := s.decoder.Decode(&header); err != nil {
		return err
	}
	if header.Size <= 0 {
		return fmt.Errorf("invalid state file size %d", header.Size)
	}
	if err := preflightStateDestination(path, header.Size); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".vote-history-transfer-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	remaining := header.Size
	for remaining > 0 {
		var chunk stateChunk
		if err := s.decoder.Decode(&chunk); err != nil {
			return err
		}
		if len(chunk.Data) == 0 || len(chunk.Data) > stateChunkSize || int64(len(chunk.Data)) > remaining {
			return fmt.Errorf("invalid state chunk size %d", len(chunk.Data))
		}
		if _, err := f.Write(chunk.Data); err != nil {
			return err
		}
		if _, err := hash.Write(chunk.Data); err != nil {
			return err
		}
		remaining -= int64(len(chunk.Data))
	}
	var actual [32]byte
	copy(actual[:], hash.Sum(nil))
	if actual != header.Digest {
		return fmt.Errorf("state file SHA-256 mismatch")
	}
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return nil
}
