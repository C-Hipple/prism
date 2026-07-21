package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Finding Outcome Tests
// =============================================================================

func testOutcome(overrides func(*FindingOutcome)) *FindingOutcome {
	o := &FindingOutcome{
		RepoOwner:   "owner",
		RepoName:    "repo",
		PRNumber:    7,
		ReviewedSHA: "abc1234",
		Fingerprint: "pkg/api/handler.go:4:deadbeef0123",
		Provenance:  "agent",
		Severity:    "critical",
		Outcome:     "dismissed",
		Reason:      "guarded by the feature flag two lines up",
		DecidedBy:   "alice",
		DecidedAt:   time.Now().UTC().Truncate(time.Second),
	}
	if overrides != nil {
		overrides(o)
	}
	return o
}

func TestGormDB_FindingOutcome_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	want := testOutcome(nil)
	require.NoError(t, db.UpsertFindingOutcome(want))

	got, err := db.GetFindingOutcomesForPR("owner", "repo", 7)
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, want.RepoOwner, got[0].RepoOwner)
	assert.Equal(t, want.RepoName, got[0].RepoName)
	assert.Equal(t, want.PRNumber, got[0].PRNumber)
	assert.Equal(t, want.ReviewedSHA, got[0].ReviewedSHA)
	assert.Equal(t, want.Fingerprint, got[0].Fingerprint)
	assert.Equal(t, want.Provenance, got[0].Provenance)
	assert.Equal(t, want.Severity, got[0].Severity)
	assert.Equal(t, want.Outcome, got[0].Outcome)
	assert.Equal(t, want.Reason, got[0].Reason)
	assert.Equal(t, want.DecidedBy, got[0].DecidedBy)
	assert.WithinDuration(t, want.DecidedAt, got[0].DecidedAt, time.Second)
	assert.NotZero(t, got[0].ID)
}

// Re-deciding the same finding (same owner/repo/pr/sha/fingerprint) must
// overwrite the prior row, not append a second one.
func TestGormDB_FindingOutcome_UpsertOverwrites(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	require.NoError(t, db.UpsertFindingOutcome(testOutcome(nil)))
	require.NoError(t, db.UpsertFindingOutcome(testOutcome(func(o *FindingOutcome) {
		o.Outcome = "acknowledged"
		o.Reason = ""
		o.DecidedBy = "bob"
	})))

	got, err := db.GetFindingOutcomesForPR("owner", "repo", 7)
	require.NoError(t, err)
	require.Len(t, got, 1, "same finding key must upsert, not duplicate")
	assert.Equal(t, "acknowledged", got[0].Outcome)
	assert.Equal(t, "", got[0].Reason)
	assert.Equal(t, "bob", got[0].DecidedBy)
}

// Outcomes are per-SHA: the same fingerprint decided at two reviewed SHAs is
// two distinct rows (the harvest joins them against per-SHA sidecars).
func TestGormDB_FindingOutcome_PerSHARows(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	require.NoError(t, db.UpsertFindingOutcome(testOutcome(nil)))
	require.NoError(t, db.UpsertFindingOutcome(testOutcome(func(o *FindingOutcome) {
		o.ReviewedSHA = "def5678"
		o.Outcome = "acknowledged"
	})))

	got, err := db.GetFindingOutcomesForPR("owner", "repo", 7)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestGormDB_FindingOutcome_ScopedToPR(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	require.NoError(t, db.UpsertFindingOutcome(testOutcome(nil)))
	require.NoError(t, db.UpsertFindingOutcome(testOutcome(func(o *FindingOutcome) {
		o.PRNumber = 8
	})))
	require.NoError(t, db.UpsertFindingOutcome(testOutcome(func(o *FindingOutcome) {
		o.RepoName = "other-repo"
	})))

	got, err := db.GetFindingOutcomesForPR("owner", "repo", 7)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 7, got[0].PRNumber)
	assert.Equal(t, "repo", got[0].RepoName)

	empty, err := db.GetFindingOutcomesForPR("owner", "repo", 999)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
