package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gateFixtureRepo builds a tiny git worktree with the structures the gates
// grep against: a settings file, a wired FE topic, and shared modules.
func gateFixtureRepo(t *testing.T) string {
	t.Helper()
	skipIfNoGit(t)
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e.c",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e.c")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main", ".")
	write("settings_base.py", "DEFINED_BUCKET = \"x\"\n  INDENTED_SETTING = 1\n")
	write("fe_registry/topics/room.ts", "export class WiredTopic extends RoomTopic {}\n")
	write("common/util.py", "def helper():\n    pass\n")
	run("add", ".")
	run("commit", "-q", "-m", "fixture")
	return dir
}

func TestGates_SettingsRef(t *testing.T) {
	dir := gateFixtureRepo(t)
	files := []diffFile{{
		Path:   "photo/tasks.py",
		Status: "modified",
		Added: []string{
			"    b = settings.DEFINED_BUCKET",     // defined -> no alert
			"    i = settings.INDENTED_SETTING",   // defined (indented) -> no alert
			"    m = settings.MISSING_BUCKET_KEY", // undefined -> alert
		},
	}}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 alert (MISSING_BUCKET_KEY), got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].CommentBody, "MISSING_BUCKET_KEY") ||
		got[0].Importance != "MEDIUM" {
		t.Errorf("unexpected alert: %+v", got[0])
	}
	// Test files are exempt.
	files[0].Path = "photo/tests/test_tasks.py"
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("test files should not alert: %+v", got)
	}
}

func TestGates_RegistrySplit(t *testing.T) {
	dir := gateFixtureRepo(t)
	// The registry gate is disabled unless configured; point it at the
	// fixture's generic layout for the duration of the test.
	oldBE, oldFE := gateTopicBEDir, gateTopicFEDir
	gateTopicBEDir, gateTopicFEDir = "be_topics/", "fe_registry"
	defer func() { gateTopicBEDir, gateTopicFEDir = oldBE, oldFE }()
	files := []diffFile{{
		Path:   "be_topics/room.py",
		Status: "modified",
		Added: []string{
			"class WiredTopic(RoomTopic):",   // FE counterpart exists -> no alert
			"class OrphanTopic(RoomTopic):",  // no FE counterpart -> alert
		},
	}}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 {
		t.Fatalf("want exactly 1 alert (OrphanTopic), got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].CommentBody, "OrphanTopic") {
		t.Errorf("unexpected alert: %+v", got[0])
	}

	// Unconfigured (default) => gate disabled entirely.
	gateTopicBEDir, gateTopicFEDir = "", ""
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("registry gate should be disabled when unconfigured: %+v", got)
	}
}

func TestGates_SharedFileAggregates(t *testing.T) {
	dir := gateFixtureRepo(t)
	files := []diffFile{
		{Path: "common/util.py", Status: "modified"},
		{Path: "app/components/common/molecules/BaseWidget/BaseWidget.tsx", Status: "modified"},
		{Path: "feature/page.tsx", Status: "modified"},
		{Path: "common/newfile.py", Status: "added"}, // added files exempt
	}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 {
		t.Fatalf("want 1 aggregated shared-file alert, got %d: %+v", len(got), got)
	}
	b := got[0].CommentBody
	if !strings.Contains(b, "2 shared/base module(s)") ||
		!strings.Contains(b, "common/util.py") || !strings.Contains(b, "BaseWidget.tsx") {
		t.Errorf("aggregation wrong: %q", b)
	}
}

func TestGates_BestEffortOnBadWorktree(t *testing.T) {
	// GatesForWorktree must never error a review: bad dir -> nil findings.
	if got := GatesForWorktree(context.Background(), "/nonexistent/dir", "main"); got != nil {
		t.Errorf("want nil on bad worktree, got %+v", got)
	}
	if got := GatesForWorktree(context.Background(), "", ""); got != nil {
		t.Errorf("want nil on empty args, got %+v", got)
	}
}
