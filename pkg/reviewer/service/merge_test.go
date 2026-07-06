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
		lc("src/urlStateBase.ts", 85, "CRITICAL", "welcome route breaks"),
	}}
	got := MergeFindings(agent, gemini)
	if len(got) != 3 {
		t.Fatalf("want 3 findings, got %d: %+v", len(got), got)
	}
	readmitted := got[2]
	if readmitted.Importance != "CRITICAL" {
		t.Errorf("severity lost: %q", readmitted.Importance)
	}
	if !strings.Contains(readmitted.CommentBody, "gemini finding") ||
		!strings.Contains(readmitted.CommentBody, "welcome route breaks") {
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
	if got[0].Importance != "CRITICAL" {
		t.Errorf("severity must upgrade to max, got %q", got[0].Importance)
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
