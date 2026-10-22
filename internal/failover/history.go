package failover

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	solanago "github.com/solana-foundation/solana-go/v2"
)

// HistoryMaxSize includes Agave's 18 GiB signed-history limit and its envelope.
// Read it with bounded memory;
// do not inherit the 32688-byte Tower buffer limit from older Firedancer builds.
const HistoryMaxSize int64 = (18 << 30) + 76

// ReadHistoryWatermark follows SavedVoteHistoryVersions::Current and the
// Firedancer 26.10.3 decoder. Observed blocks do not count as our votes. Native
// imports and Agave's restore verify the signature; this parser checks structure
// and identity and extracts a conservative boundary for participation checks.
func ReadHistoryWatermark(path, identity string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return readHistoryWatermark(f, identity)
}

func readHistoryWatermark(f *os.File, identity string) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() < 76 || info.Size() > HistoryMaxSize {
		return 0, fmt.Errorf("invalid Alpenglow history size %d", info.Size())
	}
	key, err := solanago.PublicKeyFromBase58(identity)
	if err != nil {
		return 0, err
	}
	var header [76]byte
	if _, err := f.ReadAt(header[:], 0); err != nil {
		return 0, err
	}
	if binary.LittleEndian.Uint32(header[:4]) != 0 {
		return 0, fmt.Errorf("unsupported Alpenglow history version")
	}
	size := binary.LittleEndian.Uint64(header[68:])
	if size > uint64(info.Size()-76) || size > uint64(HistoryMaxSize-76) {
		return 0, fmt.Errorf("invalid signed history length %d", size)
	}
	d := historyDecoder{r: io.NewSectionReader(f, 76, int64(size)), remaining: size}
	var pubkey [32]byte
	d.read(pubkey[:])
	if d.err != nil {
		return 0, d.err
	}
	if !bytes.Equal(pubkey[:], key[:]) {
		return 0, fmt.Errorf("vote history belongs to a different identity")
	}
	for i := 0; i < 9; i++ {
		minimum := []uint64{8, 40, 16, 8, 8, 8, 16, 40, 16}[i]
		n := d.count(minimum)
		for j := uint64(0); j < n && d.err == nil; j++ {
			switch i {
			case 0, 3, 4, 5:
				d.vote()
			case 1:
				d.vote()
				d.skip(32)
			case 2:
				d.vote()
				d.skip(d.count(32) * 32)
			case 6:
				d.vote()
				votes := d.count(11)
				for k := uint64(0); k < votes && d.err == nil; k++ {
					var tag [1]byte
					d.read(tag[:])
					d.vote()
					switch tag[0] {
					case 1, 4, 6:
						d.skip(32)
					case 2, 3, 5:
					default:
						d.err = fmt.Errorf("invalid Alpenglow vote tag %d", tag[0])
					}
					d.skip(2)
				}
			case 7:
				d.skip(40) // observed notarized blocks
			case 8:
				d.skip(8)
				d.skip(d.count(40) * 40) // observed parent-ready blocks
			}
		}
	}
	root := d.u64()
	if d.err != nil {
		return 0, fmt.Errorf("invalid Alpenglow vote history: %w", d.err)
	}
	return max(d.highest, root), nil
}

type historyDecoder struct {
	r         *io.SectionReader
	remaining uint64
	highest   uint64
	err       error
}

func (d *historyDecoder) read(p []byte) {
	if d.err != nil {
		return
	}
	if uint64(len(p)) > d.remaining {
		d.err = io.ErrUnexpectedEOF
		return
	}
	_, d.err = io.ReadFull(d.r, p)
	d.remaining -= uint64(len(p))
}
func (d *historyDecoder) skip(n uint64) {
	if d.err != nil {
		return
	}
	if n > d.remaining {
		d.err = io.ErrUnexpectedEOF
		return
	}
	_, d.err = d.r.Seek(int64(n), io.SeekCurrent)
	d.remaining -= n
}
func (d *historyDecoder) u64() uint64 {
	var b [8]byte
	d.read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}
func (d *historyDecoder) count(minimum uint64) uint64 {
	n := d.u64()
	if n > d.remaining/minimum {
		d.err = fmt.Errorf("history collection exceeds remaining data")
		return 0
	}
	return n
}
func (d *historyDecoder) vote() { d.highest = max(d.highest, d.u64()) }

// OutgoingHistoryFile selects only the file written by this handoff's source.
func (n NodeInfo) OutgoingHistoryFile(dryRun bool) string {
	if n.IsNativeFiredancer && !dryRun {
		return n.VoteHistoryFile + ".bak"
	}
	return n.VoteHistoryFile
}

func (n NodeInfo) captureOutgoingHistory(dryRun bool, demotedAt time.Time) (*os.File, error) {
	if n.IsNativeFiredancer && !dryRun {
		return captureStateFileModifiedSince(n.OutgoingHistoryFile(false), demotedAt)
	}
	return captureStateFile(n.OutgoingHistoryFile(dryRun))
}
