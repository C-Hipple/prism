package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Bug memory: a library of failure patterns distilled from this deployment's
// real production bugs, matched against a PR's changed files and injected
// into the agent prompt as priors to check. The library itself is
// deployment-specific data loaded at runtime (local file or object storage);
// nothing repo-specific lives in this codebase.
//
// Leakage control is structural: an entry sourced from the very PR under
// review is always excluded by the matcher itself (see MatchBugMemory). That
// rule is correct in production (a PR should not be judged by a pattern
// distilled from its own postmortem) and doubles as leave-one-out for
// benchmark evaluation without any harness flags.

// BugMemoryEntry is one distilled failure pattern.
type BugMemoryEntry struct {
	ID              string   `json:"id"`
	SourceCases     []string `json:"source_cases"`     // incident/ticket ids (provenance)
	SourceRepo      string   `json:"source_repo"`      // "owner/repo" the pattern came from
	SourcePRs       []int    `json:"source_prs"`       // culprit PR number(s) of the incident
	Class           string   `json:"class"`            // failure-mode class for dedup (e.g. "wiring-omission")
	Pattern         string   `json:"pattern"`          // <=400 chars, one-sentence failure mode + what to verify
	TriggerPaths    []string `json:"trigger_paths"`    // doublestar globs against changed paths
	TriggerKeywords []string `json:"trigger_keywords"` // case-insensitive substrings against added+removed lines
}

// BugMemoryMatch is the observable result of matching, kept for telemetry.
type BugMemoryMatch struct {
	Version  string   `json:"version"`
	Matched  []string `json:"matched"`          // entry ids injected, post-cap
	Excluded []string `json:"excluded_leakage"` // entry ids excluded by the source-PR rule
}

// BugMemoryLibrary is the parsed library plus its version marker.
type BugMemoryLibrary struct {
	Version string           `json:"version"`
	Entries []BugMemoryEntry `json:"entries"`
}

const (
	bugMemoryMaxEntries    = 6    // injected entries per review
	bugMemoryMaxPerClass   = 2    // per failure-mode class
	bugMemoryPatternMax    = 400  // chars per pattern
	bugMemorySectionBudget = 6000 // chars for the whole prompt section
)

// LoadBugMemory parses and validates library JSON. Invalid entries are
// dropped with a reason (never a hard failure — the library is advisory).
func LoadBugMemory(data []byte) (*BugMemoryLibrary, []string, error) {
	var lib BugMemoryLibrary
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, nil, fmt.Errorf("bug memory: parse: %w", err)
	}
	var kept []BugMemoryEntry
	var dropped []string
	for _, e := range lib.Entries {
		switch {
		case e.ID == "":
			dropped = append(dropped, "(missing id)")
		case e.Pattern == "":
			dropped = append(dropped, e.ID+": empty pattern")
		case len(e.TriggerPaths) == 0 && len(e.TriggerKeywords) == 0:
			dropped = append(dropped, e.ID+": no triggers")
		default:
			e.Pattern = sanitizePattern(e.Pattern)
			kept = append(kept, e)
		}
	}
	lib.Entries = kept
	return &lib, dropped, nil
}

// sanitizePattern applies structural sanitization only: collapse newline runs
// (entries are prompt lines, not documents), neutralize code fences, and cap
// length. No phrase filtering — that silently eats legitimate entries.
func sanitizePattern(p string) string {
	p = strings.ReplaceAll(p, "```", "'''")
	p = strings.Join(strings.Fields(p), " ")
	if len(p) > bugMemoryPatternMax {
		p = p[:bugMemoryPatternMax]
	}
	return p
}

// MatchBugMemory selects the entries relevant to this PR's diff.
//
// An entry matches when any trigger path glob matches any changed path, or
// any trigger keyword appears (case-insensitively) in the diff's added or
// removed lines. Entries sourced from this exact PR are excluded
// unconditionally (structural leave-one-out). Results are capped at
// bugMemoryMaxEntries with at most bugMemoryMaxPerClass per class; ties break
// by distinct-trigger hit count, then stable id order.
func MatchBugMemory(lib *BugMemoryLibrary, files []diffFile, owner, repo string, prNumber int) ([]BugMemoryEntry, BugMemoryMatch) {
	res := BugMemoryMatch{}
	if lib == nil || len(lib.Entries) == 0 || len(files) == 0 {
		return nil, res
	}
	res.Version = lib.Version
	repoKey := strings.ToLower(owner + "/" + repo)

	var lowerLines []string
	for _, f := range files {
		for _, l := range f.Added {
			lowerLines = append(lowerLines, strings.ToLower(l))
		}
		for _, l := range f.Removed {
			lowerLines = append(lowerLines, strings.ToLower(l))
		}
	}

	type scored struct {
		e    BugMemoryEntry
		hits int
	}
	var candidates []scored
	for _, e := range lib.Entries {
		if excludedForPR(e, repoKey, prNumber) {
			res.Excluded = append(res.Excluded, e.ID)
			continue
		}
		hits := 0
		for _, g := range e.TriggerPaths {
			for _, f := range files {
				if ok, err := doublestar.Match(g, f.Path); err == nil && ok {
					hits++
					break
				}
			}
		}
		for _, kw := range e.TriggerKeywords {
			k := strings.ToLower(kw)
			for _, l := range lowerLines {
				if strings.Contains(l, k) {
					hits++
					break
				}
			}
		}
		if hits > 0 {
			candidates = append(candidates, scored{e, hits})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].hits != candidates[j].hits {
			return candidates[i].hits > candidates[j].hits
		}
		return candidates[i].e.ID < candidates[j].e.ID
	})

	perClass := map[string]int{}
	var out []BugMemoryEntry
	for _, c := range candidates {
		if len(out) >= bugMemoryMaxEntries {
			break
		}
		if c.e.Class != "" && perClass[c.e.Class] >= bugMemoryMaxPerClass {
			continue
		}
		perClass[c.e.Class]++
		out = append(out, c.e)
		res.Matched = append(res.Matched, c.e.ID)
	}
	return out, res
}

// excludedForPR is the structural leave-one-out rule: an entry whose source
// incident is this exact PR never informs its own review.
func excludedForPR(e BugMemoryEntry, repoKey string, prNumber int) bool {
	if strings.ToLower(e.SourceRepo) != repoKey {
		return false
	}
	for _, pr := range e.SourcePRs {
		if pr == prNumber {
			return true
		}
	}
	return false
}

// bugMemorySection renders the prompt section for the matched entries.
// Empty matches render nothing, keeping the prompt byte-identical to a
// memoryless run.
func bugMemorySection(entries []BugMemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n--- THIS REPO'S BUG HISTORY (past production bugs in areas this PR touches) ---\n")
	b.WriteString("These are priors to check, not findings to report: for each, check this diff for the same mechanism and flag it ONLY with concrete evidence (file/line) from THIS diff; if it does not apply, move on.\n")
	for _, e := range entries {
		line := "- " + e.Pattern + "\n"
		if b.Len()+len(line) > bugMemorySectionBudget {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}
