package db

import (
	"gorm.io/gorm/clause"
)

// Finding-outcome persistence (outcome-triage foundation).
//
// These methods are deliberately NOT part of the Database interface — the
// feature is optional (env-gated at the HTTP layer) and extending the shared
// interface would force every mock/alternate implementation to grow with it.
// Callers that need them type-assert the narrow capability (see
// server.findingOutcomeStore).

func findingOutcomeModelToOutcome(m *FindingOutcomeModel) FindingOutcome {
	return FindingOutcome{
		ID:          int(m.ID),
		RepoOwner:   m.RepoOwner,
		RepoName:    m.RepoName,
		PRNumber:    m.PRNumber,
		ReviewedSHA: m.ReviewedSHA,
		Fingerprint: m.Fingerprint,
		Provenance:  m.Provenance,
		Severity:    m.Severity,
		Outcome:     m.Outcome,
		Reason:      m.Reason,
		DecidedBy:   m.DecidedBy,
		DecidedAt:   m.DecidedAt,
	}
}

// UpsertFindingOutcome inserts a triage decision, or overwrites the existing
// one for the same (owner, repo, pr, reviewed sha, fingerprint). Overwrite —
// not append — is intentional: the table records the CURRENT human verdict
// per finding; history isn't needed for label harvesting and would bloat the
// unique key.
func (g *GormDB) UpsertFindingOutcome(o *FindingOutcome) error {
	model := FindingOutcomeModel{
		RepoOwner:   o.RepoOwner,
		RepoName:    o.RepoName,
		PRNumber:    o.PRNumber,
		ReviewedSHA: o.ReviewedSHA,
		Fingerprint: o.Fingerprint,
		Provenance:  o.Provenance,
		Severity:    o.Severity,
		Outcome:     o.Outcome,
		Reason:      o.Reason,
		DecidedBy:   o.DecidedBy,
		DecidedAt:   o.DecidedAt,
	}
	return g.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "repo_owner"},
			{Name: "repo_name"},
			{Name: "pr_number"},
			{Name: "reviewed_sha"},
			{Name: "fingerprint"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"provenance", "severity", "outcome", "reason", "decided_by", "decided_at",
		}),
	}).Create(&model).Error
}

// GetFindingOutcomesForPR returns every recorded triage decision for a PR,
// across all reviewed SHAs, newest decision first.
func (g *GormDB) GetFindingOutcomesForPR(owner, repo string, prNumber int) ([]FindingOutcome, error) {
	var models []FindingOutcomeModel
	if err := g.db.
		Where("repo_owner = ? AND repo_name = ? AND pr_number = ?", owner, repo, prNumber).
		Order("decided_at DESC, id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	outcomes := make([]FindingOutcome, len(models))
	for i := range models {
		outcomes[i] = findingOutcomeModelToOutcome(&models[i])
	}
	return outcomes, nil
}
