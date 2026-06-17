---
name: prism-review
description: Fetch a PRism AI review for a GitHub PR, investigate the cited code, and report your own assessment with suggested next steps. Default behavior is read-only review and recommendation, NOT applying fixes. Use when the user says things like "read the PRism review for PR X", "fetch the prism review for owner/repo#N", or any variation referring to a "PRism" or "prism" AI review. The skill calls the prism server's `/api/review/{owner}/{repo}/{pr}` endpoint, prints the review findings, and then you read the cited files and write up what you found. It can also trigger a fresh review on demand (including for merged/closed PRs) via the `generate-review` endpoint. Only apply changes if the user explicitly asks (e.g. "apply the fixes", "handle the suggestions").
---

# prism-review

PRism is a privately-hosted AI code review service. When the user asks you
to handle the PRism review for a PR, this skill fetches it from the prism
server and hands the body back for you to act on.

## How to invoke

Run the fetch helper:

```bash
~/.claude/skills/prism-review/fetch.sh <pr-ref>
```

Where `<pr-ref>` can be any of:

- A bare PR number (e.g. `42`) — resolved against `git remote get-url origin`
  in the current directory.
- `owner/repo#N` shorthand (e.g. `acme/example#6`).
- A full GitHub PR URL (e.g. `https://github.com/acme/example/pull/6`).

The script prints a metadata header followed by `=== FINDINGS (N) ===`
and one block per finding. Each block has:

- `--- [SEVERITY] file:line ---` header
- `COMMENT:` the reviewer's markdown text
- `DIFF HUNK:` the unified-diff hunk containing the cited line
- `SOURCE CONTEXT:` a window of post-image source around the line
  (the cited line is the LAST line of `source_before`)

The `DIFF HUNK` and `SOURCE CONTEXT` together tell you exactly what the
comment refers to without having to re-read the file from scratch — though
you should still open the file to see callers / tests / wider context
before forming a judgment.

## Triggering a review on demand (including merged / closed PRs)

`fetch.sh` only *reads* a review that already exists. The prism poller
auto-reviews **open** PRs, so a PR the poller never saw while it was open —
most importantly a **merged or closed** PR — isn't in the database, and
`fetch.sh` returns 404 for it.

To force a fresh review for any PR regardless of state, POST to the
`generate-review` endpoint:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $(gh auth token)" \
  -H "Content-Type: application/json" \
  --data '{"owner":"acme","repo":"example","number":42}' \
  "${PRISM_BASE_URL}/api/prs/generate-review"
```

It fetches the PR straight from GitHub (open/merged/closed all work),
ingests it, and starts a review against the PR's HEAD commit — independent
of the poll cycle. The JSON response carries deterministic, DB-independent
result URLs:

```jsonc
{
  "status": "success",
  "commit": "<head-sha>",
  "review_url":   "/reviews/acme_example_42_<sha7>.html",  // rendered HTML review
  "findings_url": "/reviews/acme_example_42_<sha7>.json",  // structured findings (same shape fetch.sh parses)
  "state": "closed", "merged": true
}
```

Poll `findings_url` (prefixed with `${PRISM_BASE_URL}`) until it returns
200 — it's 404 while the review generates, 200 once it lands (agent reviews
take a few minutes):

```bash
curl -sS -H "Authorization: Bearer $(gh auth token)" \
  "${PRISM_BASE_URL}/reviews/acme_example_42_<sha7>.json"
```

These `/reviews/...` objects are served straight from storage with no
database lookup, so they survive the closed-PR cleanup that removes merged
PRs from the DB. For merged/closed PRs, **retrieve results via
`findings_url`, not `fetch.sh`** — `fetch.sh` is keyed on the PR still being
in the database and will 404 once cleanup runs. Evaluate the sidecar's
`findings[]` exactly as you would `fetch.sh` output.

> If your prism deployment predates the `review_url` / `findings_url`
> response fields, construct the path yourself from `commit`:
> `${PRISM_BASE_URL}/reviews/{owner}_{repo}_{number}_{first7ofCommit}.json`.

## What to do with the review

**Default mode is read-only assessment, not application.** Investigate the
code, form your own opinion, and report back. Do NOT edit files unless the
user explicitly asked you to apply / fix / handle the suggestions in their
original message. If unsure, ask before editing.

0. **Check the status header first.** Four states can come back:
   - `Status: review is in flight` — no findings yet. Tell the user the
     review is still generating and that they can retry in ~30 seconds (or
     you can poll for them by re-running the skill).
   - `Status: review generation FAILED` — surface the error message to the
     user and suggest re-triggering from the prism dashboard. Do NOT try to
     act on a missing review.
   - `=== NO STRUCTURED FINDINGS ===` — the review predates the JSON
     sidecar (older runs). Point the user at the `Review URL` for the HTML
     view, or ask them to regenerate to get structured output.
   - Normal output (`=== FINDINGS (N) ===`) — continue below.
1. **Check `Stale: true` in the header.** If true, the review was generated
   against an older commit than the PR's current HEAD. Surface this to the
   user as a one-line warning before going further; offer to regenerate.
   Also note `In flight (regenerating): true` — that means a fresh review
   is being computed right now, so the findings you're reading will be
   superseded shortly. Mention this to the user before acting.
2. **Walk the findings in the order printed**: they're already sorted
   critical → medium → low → unknown, then by file/line.
3. **For each finding, investigate before judging.** Use the included
   `DIFF HUNK` + `SOURCE CONTEXT` as your starting point, then open the
   cited file for wider context — callers, tests, related modules. PRism
   can be confidently wrong about intent; don't take its word.
4. **Form an independent assessment.** For each finding, decide:
   - **Agree** — the issue is real and the suggested direction is sound.
   - **Agree on problem, different fix** — issue is real but PRism's proposed
     change is wrong/incomplete; describe what you'd do instead.
   - **Disagree** — explain why (intentional behavior, missing context,
     covered by tests, etc.).
   Also flag anything *you* noticed while reading the code that PRism missed.
5. **Report back, don't edit.** Output a structured assessment:
   - One line per finding: severity · file:line · your verdict · recommended
     next step (one short clause).
   - A "Things PRism missed" section if applicable.
   - A "Suggested next steps" section ordered by priority.
   End with: "Want me to apply any of these?" so the user can opt in to
   edits. Only edit files if they say yes (or if their original message
   already said "fix" / "apply" / "handle").

## Auth

The script uses `Authorization: Bearer $(gh auth token)` by default. If the
user has the `gh` CLI authenticated, no setup is needed. Override with
`PRISM_TOKEN` env var if necessary.

## Server URL

`PRISM_BASE_URL` is **required** — set it to your prism deployment's base
URL (e.g. `https://prism.example.com` in prod, `http://localhost:8080`
when running the server locally). The skill exits with an error if it's
unset.

## Pinning to a specific commit

Set `PRISM_SHA=<short_or_full_sha>` to fetch a review for a specific commit
rather than the latest review on the PR. Useful when comparing what changed
between review generations.

## Failure modes

- Exit 2: bad PR ref or could not resolve owner/repo from git remote.
- Exit 3: no auth token available (run `gh auth login` or set `PRISM_TOKEN`).
- Exit 4: server returned non-200. Body is printed to stderr; common cases:
  - 401 — token rejected by prism server (your GitHub login isn't registered).
  - 404 — PR not in prism's database, or no review has been generated yet.
    Trigger one yourself with the `generate-review` endpoint (see
    "Triggering a review on demand") — it works even for merged/closed PRs.
