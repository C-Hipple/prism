package db

import (
	"time"
)

// PollerLeaseModel is a single-row table implementing a leader-election lease.
// Exactly one poller instance holds the lease at a time; only the holder runs
// the automatic poll cycle, so multiple Cloud Run instances (e.g. during a
// deploy overlap) never poll concurrently and race each other on rate limits
// or the via_teams prune.
type PollerLeaseModel struct {
	ID        string    `gorm:"primaryKey;column:id"`
	Holder    string    `gorm:"column:holder;not null"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
}

func (PollerLeaseModel) TableName() string { return "poller_leases" }

// pollerLeaseID is the fixed primary key of the singleton lease row.
const pollerLeaseID = "poller"

// TryAcquireOrRenewLeadership attempts to acquire the poller lease for holderID,
// or renew it if holderID already holds it. Returns true iff holderID holds a
// valid lease after the call.
//
// One atomic statement does it: insert the lease if absent, otherwise update it
// only when WE already hold it OR the existing lease has expired. The WHERE on
// the conflict path means a live lease held by someone else yields zero affected
// rows (we are not the leader); any successful insert/update yields one (we are).
// expires_at is always rewritten on a successful renew, so RowsAffected==0 can
// only mean "another holder's lease is still valid" — no follow-up read needed.
//
// Times are computed in Go and passed as parameters so the comparison is
// dialect-agnostic (no Postgres now() vs SQLite datetime() divergence).
func (g *GormDB) TryAcquireOrRenewLeadership(holderID string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	expiry := now.Add(ttl)
	res := g.db.Exec(`
		INSERT INTO poller_leases (id, holder, expires_at) VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET holder = ?, expires_at = ?
		WHERE poller_leases.holder = ? OR poller_leases.expires_at < ?`,
		pollerLeaseID, holderID, expiry,
		holderID, expiry,
		holderID, now,
	)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
