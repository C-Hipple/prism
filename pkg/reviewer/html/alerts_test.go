package html

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pr-review-server/pkg/reviewer/types"
)

// mergeNote mirrors service/merge.go provenanceNote so fixtures track the
// production phrasing the renderer classifies on.
func mergeNote(provenance string) string {
	return "_[" + provenance + " finding — retained by reconciliation, not independently confirmed by the review agent]_\n\n"
}

// alertFixtureComments is a merged finding set with one agent finding, one
// mechanical gate alert, one required-check VIOLATED synthesis, and a SUMMARY
// carrying the check ledger.
func alertFixtureComments() []types.LineComment {
	return []types.LineComment{
		{
			FilePath:    "app.go",
			LineNumber:  13,
			CommentBody: "Add error handling here",
			Importance:  "CRITICAL",
		},
		{
			FilePath:    "web/components/OverlayHost.tsx",
			LineNumber:  0,
			Importance:  "MEDIUM",
			CommentBody: mergeNote("mechanical") + "**Mechanical alert — portal overlay without an explicit layer.** This change renders portal-based UI without a `layer` prop.",
		},
		{
			FilePath:    "app/config/handlers.py",
			LineNumber:  0,
			Importance:  "MEDIUM",
			CommentBody: mergeNote("required-check") + "**Required check CHK-settings-ref-1 answered VIOLATED without an accompanying finding — escalated automatically.** CHK-settings-ref-1 | VIOLATED | EVIDENCE: handlers.py:42",
		},
		{
			FilePath:   "SUMMARY",
			LineNumber: 0,
			CommentBody: "Overall summary.\n\n---\n_Required checks (id | verdict | evidence)_\n" +
				"- CHK-portal-layer-1 | SAFE | evidence-ok\n" +
				"- CHK-settings-ref-1 | VIOLATED | evidence-ok\n",
		},
	}
}

const alertFixtureDiff = `diff --git a/app.go b/app.go
index 123..456 100644
--- a/app.go
+++ b/app.go
@@ -12,6 +12,7 @@ func main() {
 	fmt.Println("Hello")
+	// New line
 }`

func renderAlertFixture(t *testing.T) string {
	t.Helper()
	report, err := GenerateReport(alertFixtureComments(), alertFixtureDiff, 42,
		"https://github.com/acme/example/pull/42", "Test PR body", "", "abc1234",
		"gemini-pro", 0, 0, 0, time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC))
	assert.NoError(t, err)
	return report
}

func TestGenerateReport_DeterministicAlerts_DefaultOff(t *testing.T) {
	t.Setenv("SURFACE_ALERTS", "")

	report := renderAlertFixture(t)

	// No pinned section markup (the stylesheet always carries the class
	// names, so assert on the rendered elements).
	assert.NotContains(t, report, "<h2>Deterministic alerts</h2>")
	assert.NotContains(t, report, `class="deterministic-alert"`)
	// The rest of the report is untouched.
	assert.Contains(t, report, "Add error handling here")
}

func TestGenerateReport_DeterministicAlerts_On(t *testing.T) {
	t.Setenv("SURFACE_ALERTS", "true")

	report := renderAlertFixture(t)

	// Pinned section present, exactly one row per deterministic finding
	// (the outer container's class is "deterministic-alerts", so counting the
	// singular class matches rows only).
	assert.Contains(t, report, "Deterministic alerts")
	assert.Equal(t, 2, strings.Count(report, `class="deterministic-alert"`))

	// Pinned at the TOP: before the PR description and all agent prose.
	alertsIdx := strings.Index(report, "Deterministic alerts")
	descIdx := strings.Index(report, "PR Description")
	assert.Greater(t, alertsIdx, -1)
	assert.Greater(t, descIdx, alertsIdx)

	// Verdicts resolved: the gate alert positionally via the SUMMARY ledger,
	// the synthesis via its body-embedded check id.
	assert.Contains(t, report, "CHK-portal-layer-1")
	assert.Contains(t, report, "alert-verdict-safe")
	assert.Contains(t, report, "CHK-settings-ref-1")
	assert.Contains(t, report, "alert-verdict-violated")

	// Provenance badges rendered; the agent finding stays out of the section.
	assert.Contains(t, report, `class="alert-badge alert-provenance"`)
	sectionEnd := strings.Index(report, "PR Description")
	assert.NotContains(t, report[:sectionEnd], "Add error handling here")
}

func TestBuildDeterministicAlerts_VerdictResolution(t *testing.T) {
	alerts := buildDeterministicAlerts(alertFixtureComments())
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(alerts), alerts)
	}

	gate := alerts[0]
	assert.Equal(t, "mechanical", gate.Provenance)
	assert.Equal(t, "web/components/OverlayHost.tsx", gate.FilePath)
	assert.Equal(t, "CHK-portal-layer-1", gate.CheckID)
	assert.Equal(t, "SAFE", gate.Verdict)
	assert.Equal(t, "evidence-ok", gate.Evidence)

	synth := alerts[1]
	assert.Equal(t, "required-check", synth.Provenance)
	assert.Equal(t, "CHK-settings-ref-1", synth.CheckID)
	assert.Equal(t, "VIOLATED", synth.Verdict)
	assert.Equal(t, "evidence-ok", synth.Evidence)
}

func TestBuildDeterministicAlerts_UnansweredNoteAndPositionalNumbering(t *testing.T) {
	comments := []types.LineComment{
		// First portal alert: unresolved — carries its check id in the
		// enforcement note appended by EnforceRequiredChecks.
		{
			FilePath:   "web/components/PanelA.tsx",
			LineNumber: 0,
			Importance: "MEDIUM",
			CommentBody: mergeNote("mechanical") +
				"**Mechanical alert — portal overlay without an explicit layer.** Body A." +
				"\n\n_Required check CHK-portal-layer-1 was not answered with evidence — treat as unresolved risk, not a cleared concern._",
		},
		// Second portal alert: answered, no embedded id — must number past
		// the first alert's embedded id to CHK-portal-layer-2.
		{
			FilePath:    "web/components/PanelB.tsx",
			LineNumber:  0,
			Importance:  "MEDIUM",
			CommentBody: mergeNote("mechanical") + "**Mechanical alert — portal overlay without an explicit layer.** Body B.",
		},
		{
			FilePath:   "SUMMARY",
			LineNumber: 0,
			CommentBody: "_Required checks (id | verdict | evidence)_\n" +
				"- CHK-portal-layer-1 | UNANSWERED | -\n" +
				"- CHK-portal-layer-2 | SAFE | evidence-ok\n",
		},
	}

	alerts := buildDeterministicAlerts(comments)
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(alerts), alerts)
	}
	assert.Equal(t, "CHK-portal-layer-1", alerts[0].CheckID)
	assert.Equal(t, "UNANSWERED", alerts[0].Verdict)
	assert.Equal(t, "", alerts[0].Evidence)
	assert.Equal(t, "CHK-portal-layer-2", alerts[1].CheckID)
	assert.Equal(t, "SAFE", alerts[1].Verdict)
	assert.Equal(t, "evidence-ok", alerts[1].Evidence)
}

func TestBuildDeterministicAlerts_NoLedgerNoVerdict(t *testing.T) {
	// Gates fire without REQUIRED_CHECKS: alerts still surface, verdict-less.
	comments := []types.LineComment{
		{
			FilePath:    "app/models.py",
			LineNumber:  0,
			Importance:  "MEDIUM",
			CommentBody: mergeNote("mechanical") + "**Mechanical alert — new model property.** Body.",
		},
	}
	alerts := buildDeterministicAlerts(comments)
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(alerts))
	}
	assert.Equal(t, "", alerts[0].CheckID)
	assert.Equal(t, "", alerts[0].Verdict)
}
