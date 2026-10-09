package failover

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/log"
)

const stateChunkSize = 1024 * 1024

func stateLabel(_ string) string { return "vote history" }

// prepareHistoryDestination only creates the dedicated native import directory.
func prepareHistoryDestination(path string, native, dryRun bool, logger *log.Logger) error {
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) && native {
		if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
			return err
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			if !os.IsExist(err) {
				return err
			}
		} else {
			if err := os.Chmod(dir, 0700); err != nil {
				return err
			}
			if dryRun {
				logger.Warn("dry run created the vote-history import directory", "path", dir, "permissions", "0700")
			} else {
				logger.Warn("vote-history import directory was missing; created it", "path", dir, "permissions", "0700")
			}
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("history destination is not a directory: %s", dir)
	}
	return nil
}

// createDryRunHistoryDestination reserves a unique path beside the configured
// destination. The real receiver atomically installs the transferred file at
// this path; the caller removes it when the dry run ends. Keeping it in the
// same directory exercises the actual destination filesystem without
// replacing a history file that a later activation may use.
func createDryRunHistoryDestination(path string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".solana-validator-failover-dry-run-history-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// resolveHistoryDirectory resolves existing parents without creating a directory.
func resolveHistoryDirectory(dir string) (string, error) {
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Abs(resolved)
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		suffix = append(suffix, filepath.Base(dir))
		dir = parent
	}
}

func validateHistoryDirectories(importFile, exportFile string) error {
	imported, err := resolveHistoryDirectory(filepath.Dir(importFile))
	if err != nil {
		return err
	}
	exported, err := resolveHistoryDirectory(filepath.Dir(exportFile))
	if err != nil {
		return err
	}
	if imported == exported {
		return fmt.Errorf("Firedancer import directory must differ from its live history export directory")
	}
	return nil
}

// captureStateFile takes a private snapshot and checks that the source did not
// change during capture. Open descriptors survive Firedancer's path renames.
func captureStateFile(path string) (*os.File, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	before, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 {
		return nil, fmt.Errorf("vote history must be a nonempty regular file: %s", path)
	}
	snapshot, err := os.CreateTemp("", "solana-validator-failover-history-*")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			snapshot.Close()
			os.Remove(snapshot.Name())
		}
	}()
	first := sha256.New()
	n, err := io.Copy(io.MultiWriter(snapshot, first), io.LimitReader(source, before.Size()+1))
	if err != nil {
		return nil, err
	}
	after, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, fmt.Errorf("vote history changed during capture: %s", path)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	second := sha256.New()
	if _, err := io.Copy(second, io.LimitReader(source, before.Size()+1)); err != nil {
		return nil, err
	}
	if !bytes.Equal(first.Sum(nil), second.Sum(nil)) {
		return nil, fmt.Errorf("vote history changed during capture: %s", path)
	}
	if _, err := snapshot.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return snapshot, nil
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
	f, err := captureStateFile(path)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	return s.sendOpenStateFile(f)
}

func (s *Stream) sendOpenStateFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("state file must be a nonempty regular file: %s", f.Name())
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
func (s *Stream) receiveStateFile(path string) error { return s.receiveStateFileLimited(path, 0) }

func (s *Stream) receiveStateFileLimited(path string, maximum int64) error {
	var header stateHeader
	if err := s.decoder.Decode(&header); err != nil {
		return err
	}
	if header.Size <= 0 || maximum > 0 && header.Size > maximum {
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
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
