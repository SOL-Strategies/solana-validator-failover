package failover

import (
	"context"
	"errors"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/internal/solana"
)

// newTestClient returns a Client wired with the given mock, suitable for unit
// tests that only exercise methods which do not need a live QUIC connection.
func newTestClient(mock solana.ClientInterface) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		ctx:             ctx,
		cancel:          cancel,
		logger:          log.Default(),
		solanaRPCClient: mock,
	}
	return c
}

// slotSequenceMock builds a MockClient whose GetCurrentSlot returns successive
// values from the provided slice. Once the slice is exhausted it repeats the
// last value, so the slot appears stable until the test is done.
func slotSequenceMock(slots []uint64) *solana.MockClient {
	i := 0
	return solana.NewMockClient().WithGetCurrentSlot(func() (uint64, error) {
		v := slots[i]
		if i < len(slots)-1 {
			i++
		}
		return v, nil
	})
}

// TestWaitUntilStartOfNextSlot_SlotChangesImmediately checks that when the
// very first poll already sees a new slot, the function returns without extra
// delay.
func TestWaitUntilStartOfNextSlot_SlotChangesImmediately(t *testing.T) {
	// call 1 (initial): slot 10
	// call 2 (first poll): slot 11  ← already changed
	mock := slotSequenceMock([]uint64{10, 11})
	c := newTestClient(mock)

	got, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 11 {
		t.Errorf("expected slot 11, got %d", got)
	}
}

// TestWaitUntilStartOfNextSlot_SlotChangesAfterSeveralPolls checks the normal
// case where the slot is stable for a few polls before transitioning.
func TestWaitUntilStartOfNextSlot_SlotChangesAfterSeveralPolls(t *testing.T) {
	// call 1 (initial): slot 50
	// calls 2–4 (polls): slot 50 (no change yet)
	// call 5 (poll):     slot 51
	mock := slotSequenceMock([]uint64{50, 50, 50, 50, 51})
	c := newTestClient(mock)

	got, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 51 {
		t.Errorf("expected slot 51, got %d", got)
	}
}

// TestWaitUntilStartOfNextSlot_SkippedSlot verifies that a slot which jumps
// by more than one (e.g. a skipped slot) is still detected correctly since
// the check is slot > currentSlot, not slot == currentSlot+1.
func TestWaitUntilStartOfNextSlot_SkippedSlot(t *testing.T) {
	// slot 99 → 102 (slots 100 and 101 were skipped)
	mock := slotSequenceMock([]uint64{99, 99, 102})
	c := newTestClient(mock)

	got, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 102 {
		t.Errorf("expected slot 102, got %d", got)
	}
}

// TestWaitUntilStartOfNextSlot_RPCErrorOnInitialCall checks that an error on
// the very first GetCurrentSlot call is surfaced as a returned error (the
// function cannot proceed without knowing the starting slot).
func TestWaitUntilStartOfNextSlot_RPCErrorOnInitialCall(t *testing.T) {
	boom := errors.New("rpc unavailable")
	mock := solana.NewMockClient().WithGetCurrentSlot(func() (uint64, error) {
		return 0, boom
	})
	c := newTestClient(mock)

	_, err := c.waitUntilStartOfNextSlot()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("expected error to wrap %v, got: %v", boom, err)
	}
}

// TestWaitUntilStartOfNextSlot_RPCErrorsDuringPollingAreRetried verifies that
// transient errors mid-poll do not abort the wait; the function retries and
// succeeds once the slot eventually changes.
func TestWaitUntilStartOfNextSlot_RPCErrorsDuringPollingAreRetried(t *testing.T) {
	boom := errors.New("transient rpc error")
	call := 0
	mock := solana.NewMockClient().WithGetCurrentSlot(func() (uint64, error) {
		call++
		switch call {
		case 1: // initial fetch
			return 200, nil
		case 2: // first poll: transient error
			return 0, boom
		case 3: // second poll: another error
			return 0, boom
		case 4: // third poll: slot still the same
			return 200, nil
		default: // slot has now changed
			return 201, nil
		}
	})
	c := newTestClient(mock)

	got, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 201 {
		t.Errorf("expected slot 201, got %d", got)
	}
}
