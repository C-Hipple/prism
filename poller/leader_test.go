package poller

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Leadership gates the automatic poll loop: isLeader() drives whether a
// scheduled/initial poll runs. These tests cover the decision logic directly
// (the Start() loop is a thin `if p.isLeader()` over it).
func TestPoller_Leadership(t *testing.T) {
	mockGH := NewMockGitHubClient()
	mockDB := NewMockDatabase()
	p := newTestPoller(mockGH, mockDB)
	ctx := context.Background()

	// Election disabled (no holderID, as in direct unit-test construction):
	// always leader so a lone poller behaves normally.
	if !p.isLeader() {
		t.Fatal("expected isLeader()=true when election is disabled (empty holderID)")
	}

	// Enable election for this instance.
	p.holderID = "inst-1"

	// DB denies leadership -> not leader, scheduled poll would be skipped.
	mockDB.TryAcquireOrRenewLeadershipFunc = func(string, time.Duration) (bool, error) { return false, nil }
	if p.updateLeadership(ctx) {
		t.Error("expected updateLeadership=false when DB denies the lease")
	}
	if p.isLeader() {
		t.Error("expected isLeader()=false after losing the lease")
	}

	// DB grants leadership -> leader.
	mockDB.TryAcquireOrRenewLeadershipFunc = func(string, time.Duration) (bool, error) { return true, nil }
	if !p.updateLeadership(ctx) {
		t.Error("expected updateLeadership=true when DB grants the lease")
	}
	if !p.isLeader() {
		t.Error("expected isLeader()=true after acquiring the lease")
	}

	// DB error -> fail OPEN (assume leadership) so a transient DB hiccup doesn't
	// halt polling across the whole fleet.
	mockDB.TryAcquireOrRenewLeadershipFunc = func(string, time.Duration) (bool, error) {
		return false, errors.New("transient db error")
	}
	if !p.updateLeadership(ctx) {
		t.Error("expected updateLeadership to fail open (true) on a lease query error")
	}
}

// The holder ID must be unique per instance so two instances of the same Cloud
// Run revision still contend for the lease correctly.
func TestNewHolderID_Unique(t *testing.T) {
	a, b := newHolderID(), newHolderID()
	if a == b {
		t.Errorf("expected distinct holder IDs, got %q twice", a)
	}
	if a == "" {
		t.Error("holder ID must not be empty")
	}
}
