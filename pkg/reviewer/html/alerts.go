package html

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"pr-review-server/pkg/reviewer/payload"
	"pr-review-server/pkg/reviewer/types"
)

// Deterministic-alerts section: a pinned block at the very top of the report
// listing every mechanical-gate / required-check finding, so a fired
// deterministic signal cannot be buried below agent prose. Feature-gated by
// SURFACE_ALERTS (default off): with the flag unset the rendered report is
// byte-identical to a build without this section.

// surfaceAlertsEnabled reports whether the pinned section renders.
func surfaceAlertsEnabled() bool {
	return os.Getenv("SURFACE_ALERTS") == "true"
}

// AlertView is one row of the pinned "Deterministic alerts" section.
type AlertView struct {
	Provenance string // "mechanical" or "required-check"
	FilePath   string
	LineNumber int
	Importance string
	Body       string // raw Markdown; the template runs it through markdownify
	CheckID    string // required-check id (CHK-...) when resolvable, else ""
	Verdict    string // VIOLATED | SAFE | NOT-APPLICABLE | UNANSWERED; "" when unknown
	Evidence   string // ledger evidence label ("evidence-ok" / "evidence-missing"); "" when unknown
}

// VerdictClass returns the CSS-class suffix for the verdict badge.
func (a AlertView) VerdictClass() string {
	return strings.ToLower(a.Verdict)
}

// Body-embedded check-id markers, written by service/checks.go:
// EnforceRequiredChecks appends "_Required check CHK-x was not answered with
// evidence ..." to unresolved gate alerts and synthesizes "**Required check
// CHK-x answered VIOLATED ...**" findings. The ledger it appends to the
// SUMMARY records every issued check as "- CHK-id | VERDICT | evidence-label".
var (
	alertViolatedRe   = regexp.MustCompile(`Required check (CHK-[a-z0-9-]+) answered VIOLATED`)
	alertUnansweredRe = regexp.MustCompile(`Required check (CHK-[a-z0-9-]+) was not answered with evidence`)
	ledgerRowRe       = regexp.MustCompile(`(?m)^-\s+(CHK-[a-z0-9-]+)\s+\|\s+([A-Z-]+)\s+\|\s+(\S+)`)
	checkIDNumRe      = regexp.MustCompile(`^CHK-([a-z0-9-]+?)-([0-9]+)$`)
)

type ledgerEntry struct{ verdict, evidence string }

// parseCheckLedger extracts id → (verdict, evidence label) from the SUMMARY
// bodies. First entry per id wins, mirroring the enforcement layer's
// first-answer-wins rule.
func parseCheckLedger(comments []types.LineComment) map[string]ledgerEntry {
	ledger := map[string]ledgerEntry{}
	for _, c := range comments {
		if c.FilePath != "SUMMARY" {
			continue
		}
		for _, m := range ledgerRowRe.FindAllStringSubmatch(c.CommentBody, -1) {
			ev := m[3]
			if ev == "-" {
				ev = ""
			}
			if _, ok := ledger[m[1]]; !ok {
				ledger[m[1]] = ledgerEntry{verdict: m[2], evidence: ev}
			}
		}
	}
	return ledger
}

// syncKindCounter advances the per-kind positional counter past an id that
// arrived embedded in an alert body, so later same-kind alerts without an
// embedded id keep the CHK-<kind>-N numbering BuildRequiredChecks assigned.
func syncKindCounter(perKind map[string]int, id string) {
	m := checkIDNumRe.FindStringSubmatch(id)
	if m == nil {
		return
	}
	if n, err := strconv.Atoi(m[2]); err == nil && n > perKind[m[1]] {
		perKind[m[1]] = n
	}
}

// buildDeterministicAlerts collects the mechanical / required-check findings
// in merge order and resolves each one's check verdict when present. An id
// embedded in the body (enforcement notes, VIOLATED syntheses) is
// authoritative; otherwise the alert's gate kind is matched positionally to
// the CHK-<kind>-N ids BuildRequiredChecks numbers in firing order. The
// verdict is best-effort display — the alert row itself renders regardless.
func buildDeterministicAlerts(comments []types.LineComment) []AlertView {
	ledger := parseCheckLedger(comments)
	perKind := map[string]int{}
	var out []AlertView
	for _, c := range comments {
		if c.FilePath == "SUMMARY" || c.FilePath == "GENERAL" {
			continue
		}
		prov := payload.DeriveProvenance(c)
		if !payload.IsDeterministicProvenance(prov) {
			continue
		}
		v := AlertView{
			Provenance: prov,
			FilePath:   c.FilePath,
			LineNumber: c.LineNumber,
			Importance: c.Importance,
			Body:       c.CommentBody,
		}
		if m := alertViolatedRe.FindStringSubmatch(c.CommentBody); m != nil {
			v.CheckID, v.Verdict = m[1], "VIOLATED"
			syncKindCounter(perKind, v.CheckID)
		} else if m := alertUnansweredRe.FindStringSubmatch(c.CommentBody); m != nil {
			v.CheckID, v.Verdict = m[1], "UNANSWERED"
			syncKindCounter(perKind, v.CheckID)
		} else if kind := payload.AlertKind(c.CommentBody); kind != "" {
			perKind[kind]++
			candidate := fmt.Sprintf("CHK-%s-%d", kind, perKind[kind])
			if _, ok := ledger[candidate]; ok {
				v.CheckID = candidate
			}
		}
		if v.CheckID != "" {
			if e, ok := ledger[v.CheckID]; ok {
				v.Verdict, v.Evidence = e.verdict, e.evidence
			}
		}
		out = append(out, v)
	}
	return out
}
