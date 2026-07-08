package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"pr-review-server/pkg/reviewer/types"
)

// Mechanical gates: deterministic, zero-LLM signals computed from the PR's
// diff and the checked-out worktree. Offline evaluation against real
// release-blocker PRs showed each fires precisely (settings-ref: exactly the
// undefined-settings production bug; registry-split: exactly the missing
// frontend topic counterpart; shared-file: the incidental shared-module edits
// reviews anchor away from) with near-zero noise on known-good PRs.
//
// Gate findings enter the review through MergeFindings with provenance
// "mechanical", so an agent that waves them off cannot delete them.

var (
	gateSharedPath  = regexp.MustCompile(`(^|/)(common|shared|base|lib|utils?)/|(^|/)Base[A-Z]\w*\.(tsx?|jsx?|py)$`)
	gateSettingsRef = regexp.MustCompile(`settings\.([A-Z][A-Z0-9_]{2,})`)
	gateTopicClass  = regexp.MustCompile(`(?m)^\+class ([A-Z]\w*Topic)\b`)

	// The registry-split gate is repo-convention-specific, so its paths come
	// from the environment rather than being hardcoded: backend topic classes
	// live under GATE_TOPIC_BE_DIR and each must have a same-named frontend
	// class under GATE_TOPIC_FE_DIR. Both unset => the gate is disabled.
	gateTopicBEDir = os.Getenv("GATE_TOPIC_BE_DIR")
	gateTopicFEDir = os.Getenv("GATE_TOPIC_FE_DIR")

	// Overlay components that portal to a stacking layer; using them without
	// an explicit layer inherits the top-of-everything default (measured: the
	// one layerless overlay addition across 38 real PRs was the production
	// stacking bug; every clean addition passed layer= explicitly).
	// Matches the class signature — any JSX component named *Portal and direct
	// createPortal( calls — not one component's spelling: an overlay built on
	// raw createPortal shipped the same invisible/mis-stacked bug class the
	// narrow pattern missed.
	gateOverlayOpen  = regexp.MustCompile(`(?m)^\+.*(<(RichTooltip|[A-Za-z]\w*Portal|Portal)\b|\bcreatePortal\s*\()`)
	gateOverlayLayer = regexp.MustCompile(`(?m)^\+.*\blayer\s*[=:]`)

	// New properties on Django models change behavior/exception contracts for
	// every consumer and subclass (measured: 2 of 2 additions across 38 real
	// PRs were culprit PRs; zero on known-good PRs).
	gateModelProperty = regexp.MustCompile(`(?m)^\+\s*@(cached_)?property\b`)
)

// diffFile is one changed file: its path and the diff's added lines.
type diffFile struct {
	Path   string
	Added  []string // lines added by the PR (no leading '+')
	Status string   // "modified", "added", "removed"
}

// RunMechanicalGates evaluates all gates and returns provenance-ready findings
// (Importance MEDIUM at most — gates flag risk, they do not gate merges).
// grep runs against the worktree at dir via git grep.
func RunMechanicalGates(ctx context.Context, dir string, files []diffFile) []types.LineComment {
	var out []types.LineComment

	// shared-file: one aggregated advisory per PR.
	var shared []string
	for _, f := range files {
		if f.Status != "added" && gateSharedPath.MatchString(f.Path) {
			shared = append(shared, f.Path)
		}
	}
	if len(shared) > 0 {
		out = append(out, types.LineComment{
			FilePath: shared[0], LineNumber: 0, Importance: "MEDIUM",
			CommentBody: fmt.Sprintf("**Mechanical alert — shared module edited.** This PR modifies %d shared/base module(s): %s. Incidental edits to shared code are a leading cause of regressions outside the PR's headline feature; the review must explicitly address the blast radius of each.",
				len(shared), strings.Join(shared, ", ")),
		})
	}

	for _, f := range files {
		lower := strings.ToLower(f.Path)

		// settings-ref: python, non-test — referenced settings.X must be
		// assigned somewhere in the tree.
		if strings.HasSuffix(f.Path, ".py") && !strings.Contains(lower, "test") {
			seen := map[string]bool{}
			for _, l := range f.Added {
				for _, m := range gateSettingsRef.FindAllStringSubmatch(l, -1) {
					key := m[1]
					if seen[key] {
						continue
					}
					seen[key] = true
					pattern := "^[[:space:]]*" + key + "[[:space:]]*="
					if n, err := gitGrepCount(ctx, dir, pattern, false); err == nil && n == 0 {
						out = append(out, types.LineComment{
							FilePath: f.Path, LineNumber: 0, Importance: "MEDIUM",
							CommentBody: fmt.Sprintf("**Mechanical alert — undefined settings reference.** `settings.%s` is referenced by this PR but `%s` is not assigned anywhere in the tree. At runtime this raises AttributeError on first use.", key, key),
						})
					}
				}
			}
		}

		// portal-layer: overlay component added without an explicit layer.
		if strings.HasSuffix(f.Path, ".tsx") || strings.HasSuffix(f.Path, ".jsx") || strings.HasSuffix(f.Path, ".ts") {
			joined := "+" + strings.Join(f.Added, "\n+")
			if gateOverlayOpen.MatchString(joined) && !gateOverlayLayer.MatchString(joined) {
				out = append(out, types.LineComment{
					FilePath: f.Path, LineNumber: 0, Importance: "MEDIUM",
					CommentBody: "**Mechanical alert — portal overlay without an explicit layer.** This change renders portal-based UI (a *Portal component or createPortal call) without a `layer` prop. Portaled content escapes its parent's stacking and visibility context: on the default layer it renders above modals, and under browser fullscreen only descendants of the fullscreen element are visible at all. State the intended stacking layer explicitly, and verify the portal target is correct for fullscreen contexts.",
				})
			}
		}

		// model-property: new property on a Django model.
		if strings.HasSuffix(f.Path, "models.py") && !strings.Contains(lower, "test") {
			joined := "+" + strings.Join(f.Added, "\n+")
			if gateModelProperty.MatchString(joined) {
				out = append(out, types.LineComment{
					FilePath: f.Path, LineNumber: 0, Importance: "MEDIUM",
					CommentBody: "**Mechanical alert — new model property.** This change adds a property to a Django model. Properties on shared models define behavior and exception contracts for every consumer and subclass: verify what it returns or raises for each subclass, and whether existing hasattr/getattr call sites remain correct.",
				})
			}
		}

		// registry-split: new backend push topic must have an FE counterpart.
		if gateTopicBEDir != "" && gateTopicFEDir != "" &&
			strings.HasPrefix(f.Path, gateTopicBEDir) && !strings.Contains(lower, "test") {
			joined := "+" + strings.Join(f.Added, "\n+")
			for _, m := range gateTopicClass.FindAllStringSubmatch(joined, -1) {
				cls := m[1]
				if n, err := gitGrepCount(ctx, dir, `\b`+regexp.QuoteMeta(cls)+`\b`, true, gateTopicFEDir); err == nil && n == 0 {
					out = append(out, types.LineComment{
						FilePath: f.Path, LineNumber: 0, Importance: "MEDIUM",
						CommentBody: fmt.Sprintf("**Mechanical alert — backend topic without frontend counterpart.** `%s` is registered on the backend, but no class of that name exists under `%s`. Clients cannot handle unknown topics; historically a missing counterpart broke the entire grouped subscription.", cls, gateTopicFEDir),
					})
				}
			}
		}
	}
	return out
}

// gitGrepCount counts matching lines via `git grep -c -E` in dir, restricted to
// pathspec when given. POSIX ERE only (git grep -E has no \s/\b — callers use
// [[:space:]]; word matching uses -w via the word flag).
func gitGrepCount(ctx context.Context, dir, pattern string, word bool, pathspec ...string) (int, error) {
	args := []string{"grep", "-I", "-c", "-E"}
	if word {
		args = append(args, "-w")
		pattern = strings.TrimPrefix(strings.TrimSuffix(pattern, `\b`), `\b`)
	}
	args = append(args, pattern, "--")
	if len(pathspec) > 0 {
		args = append(args, pathspec...)
	} else {
		args = append(args, ".")
	}
	out, err := runGit(ctx, dir, args...)
	if err != nil {
		// git grep exits 1 on "no matches" — treat as zero, not error.
		if strings.TrimSpace(out) == "" {
			return 0, nil
		}
		return 0, err
	}
	total := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		i := strings.LastIndex(line, ":")
		if i < 0 {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(line[i+1:], "%d", &n); err == nil {
			total += n
		}
	}
	return total, nil
}

// parseNameStatusDiff builds diffFiles from `git diff` output: the name-status
// listing plus the unified diff, both against base...HEAD in the worktree.
func parseNameStatusDiff(ctx context.Context, dir, base string) ([]diffFile, error) {
	ns, err := runGit(ctx, dir, "diff", "--name-status", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("name-status: %w (%s)", err, ns)
	}
	status := map[string]string{}
	var order []string
	for _, line := range strings.Split(strings.TrimSpace(ns), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		st := "modified"
		switch parts[0][0] {
		case 'A':
			st = "added"
		case 'D':
			st = "removed"
		}
		p := parts[len(parts)-1]
		status[p] = st
		order = append(order, p)
	}

	full, err := runGit(ctx, dir, "diff", "--unified=0", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("unified diff: %w", err)
	}
	added := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(full, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			cur = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if cur != "" && strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added[cur] = append(added[cur], line[1:])
		}
	}

	var files []diffFile
	for _, p := range order {
		files = append(files, diffFile{Path: p, Added: added[p], Status: status[p]})
	}
	return files, nil
}

// GatesForWorktree is the production entry point: diff the worktree against
// origin/<defaultBranch> and run all gates. Best-effort — on any error it
// returns nil findings (gates are advisory; they must never fail a review).
func GatesForWorktree(ctx context.Context, dir, defaultBranch string) []types.LineComment {
	if dir == "" {
		return nil
	}
	// The poller does not always know the base branch; origin/HEAD points at
	// the clone's default branch and works for the three-dot diff.
	base := "origin/HEAD"
	if defaultBranch != "" {
		base = "origin/" + defaultBranch
	}
	files, err := parseNameStatusDiff(ctx, dir, base)
	if err != nil {
		return nil
	}
	// Guard pathological diffs: gates are per-added-line regex scans.
	total := 0
	for _, f := range files {
		total += len(f.Added)
	}
	if total > 20000 {
		return nil
	}
	return RunMechanicalGates(ctx, dir, files)
}
