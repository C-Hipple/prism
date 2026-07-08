// gatecheck reports what the deterministic review signals — mechanical gates
// and the bug-memory matcher — would contribute to a review of a checked-out
// worktree, without running any LLM. It imports the production functions, so
// its output is predictive of production behavior by construction.
//
// Usage:
//
//	gatecheck <worktree-dir> <base-ref> [--bug-memory file.json --repo owner/repo --pr N]
//
// Output: one JSON OfflineWorktreeReport on stdout. A diff that failed to
// parse reports {"diff_parsed": false, ...} — distinct from a quiet diff.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pr-review-server/pkg/reviewer/service"
)

func main() {
	fs := flag.NewFlagSet("gatecheck", flag.ExitOnError)
	memPath := fs.String("bug-memory", "", "path to a bug-memory library JSON")
	repo := fs.String("repo", "", "owner/repo of the PR under check (for leave-one-out)")
	pr := fs.Int("pr", 0, "PR number under check (for leave-one-out)")

	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gatecheck <worktree-dir> <base-ref> [--bug-memory file.json --repo owner/repo --pr N]")
		os.Exit(2)
	}
	dir, base := args[0], args[1]
	_ = fs.Parse(args[2:])

	var lib *service.BugMemoryLibrary
	if *memPath != "" {
		data, err := os.ReadFile(*memPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gatecheck: read bug memory: %v\n", err)
			os.Exit(1)
		}
		var dropped []string
		lib, dropped, err = service.LoadBugMemory(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gatecheck: %v\n", err)
			os.Exit(1)
		}
		for _, d := range dropped {
			fmt.Fprintf(os.Stderr, "gatecheck: dropped entry: %s\n", d)
		}
	}

	owner, repoName := "", ""
	if parts := strings.SplitN(*repo, "/", 2); len(parts) == 2 {
		owner, repoName = parts[0], parts[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	rep := service.OfflineCheckWorktree(ctx, dir, base, lib, owner, repoName, *pr)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintf(os.Stderr, "gatecheck: encode: %v\n", err)
		os.Exit(1)
	}
	if !rep.DiffParsed {
		os.Exit(3) // signal "no diff signal" distinctly for sweep scripts
	}
}
