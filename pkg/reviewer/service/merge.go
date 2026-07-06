package service

import (
	"path"
	"strings"

	"pr-review-server/pkg/reviewer/types"
)

// FindingSet pairs a provenance label with one source's review comments.
// Provenance examples: "agent", "gemini", "premortem", "mechanical".
type FindingSet struct {
	Provenance string
	Comments   []types.LineComment
}

// mergeLineTolerance: two findings on the same file within this many lines are
// treated as the same underlying issue.
const mergeLineTolerance = 10

// importanceRank orders severities for the max-upgrade rule. Unknown/empty
// ranks lowest so a labeled duplicate always wins.
func importanceRank(imp string) int {
	switch strings.ToUpper(strings.TrimSpace(imp)) {
	case "CRITICAL":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

// MergeFindings unions review findings from multiple sources into one list.
//
// Rationale: the agent stage's output used to fully REPLACE the first-pass
// (Gemini) findings, and our release-blocker eval showed the agent deleting
// correct first-pass catches it had argued itself out of. Merging is
// deterministic — a dropped finding survives no matter how persuasive the
// agent's dismissal prose was.
//
// Rules:
//   - Sets are in priority order: the first set is the canonical voice (its
//     phrasing wins on duplicates, and only its SUMMARY entries are kept).
//   - Two non-SUMMARY findings are duplicates when they target the same file
//     (matched on full path or basename, since sources differ in how much of
//     the path they emit) within mergeLineTolerance lines. Line 0 (whole-file)
//     only matches line 0.
//   - Duplicates keep the higher-priority phrasing but upgrade Importance to
//     the max across the pair — a later source never *lowers* severity.
//   - Findings unique to a lower-priority set are appended with a provenance
//     note so readers know they were not independently confirmed.
//
// The caller decides what belongs in each set (e.g. filtering Gemini style
// nits before the merge); MergeFindings only unions what it is given.
func MergeFindings(sets ...FindingSet) []types.LineComment {
	var merged []types.LineComment
	for si, set := range sets {
		for _, c := range set.Comments {
			if c.FilePath == "SUMMARY" {
				if si == 0 {
					merged = append(merged, c)
				}
				continue
			}
			if di, ok := findDuplicate(merged, c); ok {
				if importanceRank(c.Importance) > importanceRank(merged[di].Importance) {
					merged[di].Importance = c.Importance
				}
				continue
			}
			if si > 0 {
				c.CommentBody = provenanceNote(set.Provenance) + c.CommentBody
			}
			merged = append(merged, c)
		}
	}
	return merged
}

// findDuplicate returns the index in merged of a finding duplicating c.
func findDuplicate(merged []types.LineComment, c types.LineComment) (int, bool) {
	for i, m := range merged {
		if m.FilePath == "SUMMARY" || !sameFile(m.FilePath, c.FilePath) {
			continue
		}
		if m.LineNumber == 0 || c.LineNumber == 0 {
			if m.LineNumber == c.LineNumber {
				return i, true
			}
			continue
		}
		d := m.LineNumber - c.LineNumber
		if d < 0 {
			d = -d
		}
		if d <= mergeLineTolerance {
			return i, true
		}
	}
	return -1, false
}

// sameFile matches paths exactly, or by suffix/basename — different sources
// emit different amounts of leading path for the same file.
func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	if strings.HasSuffix(a, "/"+b) || strings.HasSuffix(b, "/"+a) {
		return true
	}
	return path.Base(a) == path.Base(b)
}

// provenanceNote renders the marker prepended to re-admitted findings.
func provenanceNote(provenance string) string {
	if provenance == "" {
		provenance = "earlier pass"
	}
	return "_[" + provenance + " finding — retained by reconciliation, not independently confirmed by the review agent]_\n\n"
}
