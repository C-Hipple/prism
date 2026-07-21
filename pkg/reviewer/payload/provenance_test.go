package payload

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"

	"pr-review-server/pkg/reviewer/types"
)

// mergeNote renders the reconciliation marker exactly as service/merge.go
// provenanceNote does, so these fixtures track the production phrasing.
func mergeNote(provenance string) string {
	return "_[" + provenance + " finding — retained by reconciliation, not independently confirmed by the review agent]_\n\n"
}

func TestDeriveProvenance_DefaultsToAgent(t *testing.T) {
	c := types.LineComment{FilePath: "app.go", LineNumber: 10, CommentBody: "plain agent finding"}
	if got := DeriveProvenance(c); got != ProvenanceAgent {
		t.Errorf("DeriveProvenance() = %q, want %q", got, ProvenanceAgent)
	}
}

func TestDeriveProvenance_ParsesMergeNote(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"first-pass", ProvenanceFirstPass},
		{"mechanical", ProvenanceMechanical},
		{"required-check", ProvenanceRequiredCheck},
		{"earlier pass", ProvenanceFirstPass},
		{"carried from review of abc1234", ProvenanceCarried},
		{"prior-review", ProvenanceCarried},
		{"premortem", "premortem"}, // unknown labels pass through verbatim
	}
	for _, tc := range cases {
		c := types.LineComment{
			FilePath:    "app.go",
			LineNumber:  10,
			CommentBody: mergeNote(tc.label) + "the finding body",
		}
		if got := DeriveProvenance(c); got != tc.want {
			t.Errorf("DeriveProvenance(note %q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

func TestDeriveProvenance_ExplicitFieldWins(t *testing.T) {
	// A producer-set field beats the prose marker: carry-forward re-attributes
	// findings whose bodies still describe the prior run.
	c := types.LineComment{
		FilePath:    "app.go",
		LineNumber:  10,
		CommentBody: mergeNote("first-pass") + "body",
		Provenance:  "carried",
	}
	if got := DeriveProvenance(c); got != ProvenanceCarried {
		t.Errorf("DeriveProvenance() = %q, want %q", got, ProvenanceCarried)
	}
}

func TestBuild_ProvenanceRoundTrip(t *testing.T) {
	comments := []types.LineComment{
		{FilePath: "a.go", LineNumber: 1, Importance: "CRITICAL", CommentBody: "agent finding"},
		{FilePath: "b.go", LineNumber: 2, Importance: "MEDIUM",
			CommentBody: mergeNote("first-pass") + "readmitted critical"},
		{FilePath: "web/OverlayHost.tsx", LineNumber: 0, Importance: "MEDIUM",
			CommentBody: mergeNote("mechanical") + "**Mechanical alert — portal overlay without an explicit layer.** details"},
		{FilePath: "c.py", LineNumber: 0, Importance: "MEDIUM",
			CommentBody: mergeNote("required-check") + "**Required check CHK-settings-ref-1 answered VIOLATED without an accompanying finding — escalated automatically.** body"},
		{FilePath: "d.go", LineNumber: 3, Importance: "MEDIUM", Provenance: "carried",
			CommentBody: mergeNote("carried from review of abc1234") + "carried body"},
	}

	pl := Build("o", "r", 1, "sha", comments, "", nil)

	raw, err := json.Marshal(pl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Payload
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]string{
		"a.go":                ProvenanceAgent,
		"b.go":                ProvenanceFirstPass,
		"web/OverlayHost.tsx": ProvenanceMechanical,
		"c.py":                ProvenanceRequiredCheck,
		"d.go":                ProvenanceCarried,
	}
	if len(back.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d", len(back.Findings), len(want))
	}
	for _, f := range back.Findings {
		if f.Provenance != want[f.File] {
			t.Errorf("finding %s: provenance = %q, want %q", f.File, f.Provenance, want[f.File])
		}
	}
	if !strings.Contains(string(raw), `"provenance":"mechanical"`) {
		t.Errorf("marshaled payload missing structured provenance field:\n%s", raw)
	}
}

func TestBuild_ProvenanceSurvivesToLineComments(t *testing.T) {
	comments := []types.LineComment{
		{FilePath: "a.go", LineNumber: 1, Importance: "LOW",
			CommentBody: mergeNote("mechanical") + "gate body"},
	}
	pl := Build("o", "r", 1, "sha", comments, "", nil)
	round := pl.ToLineComments()
	if len(round) != 1 || round[0].Provenance != ProvenanceMechanical {
		t.Errorf("ToLineComments provenance = %+v, want mechanical", round)
	}
}

func TestAlertID(t *testing.T) {
	gate := types.LineComment{
		FilePath:    "web/OverlayHost.tsx",
		LineNumber:  0,
		CommentBody: "**Mechanical alert — portal overlay without an explicit layer.** details",
	}
	if got := AlertID(gate); got != "portal-layer:web/OverlayHost.tsx:0" {
		t.Errorf("AlertID() = %q", got)
	}
	unknown := types.LineComment{FilePath: "x.py", LineNumber: 4, CommentBody: "**Bug-memory alert — past failure pattern (BM-1).** body"}
	if got := AlertID(unknown); got != "alert:x.py:4" {
		t.Errorf("AlertID() = %q", got)
	}
}

// AlertKind must know every gate phrasing checks.go issues check ids for —
// including the round-3 gates — or the pinned-alerts section can never
// resolve their CHK-<kind>-N ids and AlertID degrades to "alert:".
func TestAlertKind_CoversRound3Gates(t *testing.T) {
	cases := map[string]string{
		"**Mechanical alert — new symbol with no references.** `orphanFn` ...":            "wiring",
		"**Mechanical alert — symbol de-wired.** ...":                                     "wiring",
		"**Mechanical alert — resource acquired without unmount cleanup.** ...":           "effect-cleanup",
		"**Mechanical alert — task payload contract changed.** ...":                       "task-skew",
		"**Mechanical alert — migration residue.** ...":                                   "migration-residue",
		"**Mechanical alert — portal overlay without an explicit layer.** ...":            "portal-layer",
		"**Mechanical alert — undefined settings reference.** ...":                        "settings-ref",
		"**Mechanical alert — shared module edited.** ...":                                "shared-file",
		"**Mechanical alert — new model property.** ...":                                  "model-property",
		"**Mechanical alert — backend topic without frontend counterpart.** `XTopic` ...": "registry-split",
	}
	for body, want := range cases {
		if got := AlertKind(body); got != want {
			t.Errorf("AlertKind(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestMissingAlerts(t *testing.T) {
	gateBody := "**Mechanical alert — undefined settings reference.** `settings.NEW_FLAG` is referenced but never assigned."
	alert := types.LineComment{FilePath: "app/config/handlers.py", LineNumber: 0, Importance: "MEDIUM", CommentBody: gateBody}

	// Represented verbatim (survived the merge with a provenance-note prefix).
	surviving := []Finding{{
		Severity: "medium", Provenance: ProvenanceMechanical,
		File: "app/config/handlers.py", Comment: mergeNote("mechanical") + gateBody,
	}}
	if missing := MissingAlerts(surviving, []types.LineComment{alert}); len(missing) != 0 {
		t.Errorf("verbatim-represented alert reported missing: %+v", missing)
	}

	// Represented by a required-check VIOLATED synthesis on the same file —
	// the merge intentionally lets the confirmed-defect body absorb the
	// generic advisory.
	synthesized := []Finding{{
		Severity: "medium", Provenance: ProvenanceRequiredCheck,
		File: "app/config/handlers.py", Comment: mergeNote("required-check") + "**Required check CHK-settings-ref-1 answered VIOLATED without an accompanying finding — escalated automatically.** ...",
	}}
	if missing := MissingAlerts(synthesized, []types.LineComment{alert}); len(missing) != 0 {
		t.Errorf("synthesis-represented alert reported missing: %+v", missing)
	}

	// Swallowed: a nearby agent finding's phrasing won the duplicate collapse
	// and the gate's text is gone.
	swallowed := []Finding{{
		Severity: "critical", Provenance: ProvenanceAgent,
		File: "app/config/handlers.py", Comment: "unrelated agent prose about this file",
	}}
	missing := MissingAlerts(swallowed, []types.LineComment{alert})
	if len(missing) != 1 || missing[0].FilePath != alert.FilePath {
		t.Fatalf("swallowed alert not detected: %+v", missing)
	}
}

func TestBuild_NoSwallow_LogsDroppedAlert(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	gateBody := "**Mechanical alert — portal overlay without an explicit layer.** details"
	alert := types.LineComment{FilePath: "web/OverlayHost.tsx", LineNumber: 0, Importance: "MEDIUM", CommentBody: gateBody}

	// Synthetic swallow: the merged output has only an unrelated agent
	// finding on the file — the alert's text is gone.
	comments := []types.LineComment{
		{FilePath: "web/OverlayHost.tsx", LineNumber: 0, Importance: "CRITICAL", CommentBody: "agent phrasing that won the dedup"},
	}
	Build("o", "r", 1, "sha", comments, "", nil, WithGateAlerts([]types.LineComment{alert}))

	out := buf.String()
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "portal-layer:web/OverlayHost.tsx:0") {
		t.Errorf("expected no-swallow ERROR naming the alert id, got log:\n%s", out)
	}

	// Represented alert: no ERROR.
	buf.Reset()
	kept := []types.LineComment{
		{FilePath: "web/OverlayHost.tsx", LineNumber: 0, Importance: "MEDIUM", CommentBody: mergeNote("mechanical") + gateBody},
	}
	Build("o", "r", 1, "sha", kept, "", nil, WithGateAlerts([]types.LineComment{alert}))
	if strings.Contains(buf.String(), "ERROR") {
		t.Errorf("represented alert produced a spurious ERROR:\n%s", buf.String())
	}
}
