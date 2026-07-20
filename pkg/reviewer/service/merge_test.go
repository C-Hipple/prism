package service

import (
	"strings"
	"testing"

	"pr-review-server/pkg/reviewer/types"
)

func lc(file string, line int, imp, body string) types.LineComment {
	return types.LineComment{FilePath: file, LineNumber: line, Importance: imp, CommentBody: body}
}

// The core regression this exists to prevent: a first-pass bug finding the
// agent dropped must survive the merge, provenance-tagged.
func TestMergeFindings_DroppedFirstPassFindingSurvives(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("SUMMARY", 0, "LOW", "Verdict: approve"),
		lc("a/b.ts", 10, "LOW", "nit"),
	}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("src/routing.ts", 85, "CRITICAL", "route classification breaks"),
	}}
	got := MergeFindings(agent, gemini)
	if len(got) != 3 {
		t.Fatalf("want 3 findings, got %d: %+v", len(got), got)
	}
	readmitted := got[2]
	// Re-admissions cap at MEDIUM: unconfirmed first-pass severity must not
	// create blockers (the first pass emits CRITICALs on half of clean PRs).
	if readmitted.Importance != "MEDIUM" {
		t.Errorf("re-admitted severity should cap at MEDIUM, got %q", readmitted.Importance)
	}
	if !strings.Contains(readmitted.CommentBody, "gemini finding") ||
		!strings.Contains(readmitted.CommentBody, "route classification breaks") {
		t.Errorf("provenance tag or body missing: %q", readmitted.CommentBody)
	}
}

func TestMergeFindings_DuplicateKeepsAgentPhrasingUpgradesSeverity(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("x.py", 100, "LOW", "agent phrasing"),
	}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("deep/path/x.py", 105, "CRITICAL", "gemini phrasing"), // same file (suffix), within tolerance
	}}
	got := MergeFindings(agent, gemini)
	if len(got) != 1 {
		t.Fatalf("want dedupe to 1, got %d", len(got))
	}
	if got[0].CommentBody != "agent phrasing" {
		t.Errorf("higher-priority phrasing should win: %q", got[0].CommentBody)
	}
	// Upgrades sourced from a lower-priority set also cap at MEDIUM.
	if got[0].Importance != "MEDIUM" {
		t.Errorf("severity should upgrade but cap at MEDIUM, got %q", got[0].Importance)
	}
}

func TestMergeFindings_SeverityNeverDowngrades(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("x.py", 100, "CRITICAL", "agent"),
	}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("x.py", 100, "LOW", "gemini"),
	}}
	got := MergeFindings(agent, gemini)
	if len(got) != 1 || got[0].Importance != "CRITICAL" {
		t.Fatalf("severity downgraded: %+v", got)
	}
}

func TestMergeFindings_BeyondToleranceIsDistinct(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("x.py", 100, "LOW", "a")}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("x.py", 100+mergeLineTolerance+1, "MEDIUM", "b")}}
	if got := MergeFindings(agent, gemini); len(got) != 2 {
		t.Fatalf("want 2 distinct findings, got %d", len(got))
	}
}

func TestMergeFindings_OnlyPrimarySummaryKept(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("SUMMARY", 0, "MEDIUM", "agent verdict")}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("SUMMARY", 0, "LOW", "gemini verdict")}}
	got := MergeFindings(agent, gemini)
	if len(got) != 1 || got[0].CommentBody != "agent verdict" {
		t.Fatalf("want only the primary SUMMARY, got %+v", got)
	}
}

func TestMergeFindings_WholeFileMatchesOnlyWholeFile(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("x.py", 0, "LOW", "whole-file note")}}
	gemini := FindingSet{Provenance: "gemini", Comments: []types.LineComment{
		lc("x.py", 5, "MEDIUM", "line 5 bug")}}
	if got := MergeFindings(agent, gemini); len(got) != 2 {
		t.Fatalf("line-0 should not swallow a line-anchored finding: %+v", got)
	}
}

// Whole-file findings carry no line signal to corroborate a basename match,
// so they dedup only on a strict path match: two directories' models.py are
// distinct findings, not duplicates.
func TestMergeFindings_WholeFileRequiresStrictPathMatch(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("payments/models.py", 0, "LOW", "reviewed, looks fine")}}
	checks := FindingSet{Provenance: "required-check", Comments: []types.LineComment{
		lc("tipping/models.py", 0, "MEDIUM", "unresolved memory check")}}
	got := MergeFindings(agent, checks)
	if len(got) != 2 {
		t.Fatalf("basename-only match must not dedup whole-file findings: %+v", got)
	}

	// Exact (and whole-path-suffix) matches still dedup, and the earlier
	// set's phrasing wins: a gate alert collapses into the required-check
	// synthesis that precedes it, not the other way around.
	synth := FindingSet{Provenance: "required-check", Comments: []types.LineComment{
		lc("app/Tooltip.tsx", 0, "MEDIUM", "escalated VIOLATED answer")}}
	mech := FindingSet{Provenance: "mechanical", Comments: []types.LineComment{
		lc("app/Tooltip.tsx", 0, "MEDIUM", "generic advisory")}}
	got = MergeFindings(synth, mech)
	if len(got) != 1 || !strings.Contains(got[0].CommentBody, "escalated VIOLATED answer") {
		t.Fatalf("gate alert should collapse into the earlier synthesis: %+v", got)
	}
}

func TestMergeFindings_EmptyAndSingleSet(t *testing.T) {
	if got := MergeFindings(); got != nil {
		t.Errorf("no sets -> nil, got %+v", got)
	}
	one := FindingSet{Provenance: "agent", Comments: []types.LineComment{lc("a.go", 1, "LOW", "x")}}
	got := MergeFindings(one)
	if len(got) != 1 || strings.Contains(got[0].CommentBody, "reconciliation") {
		t.Errorf("single set should pass through untagged: %+v", got)
	}
}

// TestMergeFindings_SummaryReconciliationNote — when findings are re-admitted,
// the primary SUMMARY must disclose the potential contradiction; when nothing
// is re-admitted the SUMMARY stays untouched.
func TestMergeFindings_SummaryReconciliationNote(t *testing.T) {
	agent := FindingSet{Provenance: "agent", Comments: []types.LineComment{
		lc("SUMMARY", 0, "LOW", "Verdict: approve"),
		lc("a.ts", 5, "LOW", "nit"),
	}}
	gemini := FindingSet{Provenance: "first-pass", Comments: []types.LineComment{
		lc("b.ts", 40, "CRITICAL", "crash"),
	}}
	got := MergeFindings(agent, gemini)
	if !strings.Contains(got[0].CommentBody, "Reconciliation: 1 earlier-pass finding") {
		t.Errorf("SUMMARY missing reconciliation note: %q", got[0].CommentBody)
	}
	// No re-admissions -> untouched SUMMARY.
	got = MergeFindings(agent, FindingSet{Provenance: "first-pass", Comments: nil})
	if strings.Contains(got[0].CommentBody, "Reconciliation") {
		t.Errorf("SUMMARY should be untouched with no re-admissions: %q", got[0].CommentBody)
	}
}
