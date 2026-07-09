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
			"class WiredTopic(RoomTopic):",  // FE counterpart exists -> no alert
			"class OrphanTopic(RoomTopic):", // no FE counterpart -> alert
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

func TestGates_PortalLayer(t *testing.T) {
	dir := gateFixtureRepo(t)
	// Layerless overlay -> alert.
	files := []diffFile{{Path: "app/Tooltip.tsx", Status: "modified",
		Added: []string{"        <RichTooltip", "            isOpen={true}"}}}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 || !strings.Contains(got[0].CommentBody, "overlay without an explicit layer") {
		t.Fatalf("want 1 portal-layer alert, got %+v", got)
	}
	// Explicit layer -> silent.
	files[0].Added = []string{"    return <Portal layer={portalLayer}>{content}</Portal>"}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("explicit layer should not alert: %+v", got)
	}
	// Non-jsx file -> silent.
	files[0].Path = "app/notes.md"
	files[0].Added = []string{"<RichTooltip"}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("non-jsx should not alert: %+v", got)
	}
}

func TestGates_ModelProperty(t *testing.T) {
	dir := gateFixtureRepo(t)
	files := []diffFile{{Path: "tipping/models.py", Status: "modified",
		Added: []string{"    @cached_property", "    def wanted(self):"}}}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 || !strings.Contains(got[0].CommentBody, "new model property") {
		t.Fatalf("want 1 model-property alert, got %+v", got)
	}
	// Plain @property also fires; non-models.py and tests do not.
	files[0].Added = []string{"    @property", "    def x(self):"}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 1 {
		t.Errorf("@property should alert: %+v", got)
	}
	files[0].Path = "tipping/views.py"
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("non-models.py should not alert: %+v", got)
	}
	files[0].Path = "tipping/tests/models.py"
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("test files should not alert: %+v", got)
	}
}

func TestGates_PortalLayer_ClassWide(t *testing.T) {
	dir := gateFixtureRepo(t)
	// createPortal without layer -> alert (including plain .ts).
	files := []diffFile{{Path: "app/preview.ts", Status: "modified",
		Added: []string{"    return createPortal(content, document.body)"}}}
	got := RunMechanicalGates(context.Background(), dir, files)
	if len(got) != 1 || !strings.Contains(got[0].CommentBody, "portal overlay") {
		t.Fatalf("want createPortal alert, got %+v", got)
	}
	// Any *Portal component -> alert.
	files[0] = diffFile{Path: "app/Media.tsx", Status: "modified",
		Added: []string{"        <FullscreenMediaPortal media={m} />"}}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 1 {
		t.Fatalf("want *Portal component alert, got %+v", got)
	}
	// createPortal alongside an explicit layer -> silent.
	files[0] = diffFile{Path: "app/preview.ts", Status: "modified",
		Added: []string{"    return createPortal(content, target)", "    layer={overlayLayer}"}}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("explicit layer should suppress: %+v", got)
	}
	// Identifier merely containing the substring -> silent.
	files[0] = diffFile{Path: "app/util.ts", Status: "modified",
		Added: []string{"    const recreatePortalCount = 3"}}
	if got := RunMechanicalGates(context.Background(), dir, files); len(got) != 0 {
		t.Errorf("substring identifier should not alert: %+v", got)
	}
}

func TestParseNameStatusDiff_DeletedFileRemovals(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := runGit(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(dir+"/doomed.ts", []byte("const menuId = getMenuId()\nexport { menuId }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "base")
	run("branch", "base")
	run("rm", "-q", "doomed.ts")
	run("commit", "-q", "-m", "delete file")

	files, err := parseNameStatusDiff(context.Background(), dir, "base")
	if err != nil {
		t.Fatal(err)
	}
	var doomed *diffFile
	for i := range files {
		if files[i].Path == "doomed.ts" {
			doomed = &files[i]
		}
	}
	if doomed == nil || doomed.Status != "removed" {
		t.Fatalf("deleted file missing from diff: %+v", files)
	}
	// The whole point: a deleted file's removed lines must feed the matchers.
	joined := strings.Join(doomed.Removed, "\n")
	if !strings.Contains(joined, "getMenuId") {
		t.Errorf("removed lines of a deleted file were dropped: %+v", doomed.Removed)
	}
}
