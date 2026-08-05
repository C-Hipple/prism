# PR Review Server

A self-hostable code review dashboard for GitHub pull requests, with an optional two-stage AI review pipeline.

**Dashboard** — polls GitHub for PRs assigned to you (or your org), shows CI status, draft/ready state, and merged/closed indicators, with filtering and search mirrored into the URL for shareable views. Rows can be hidden into a collapsed section, and reviews you request by pasting any PR URL land in a "Requested by Me" section.

**AI reviews** — a Gemini pass generates review comments; optionally a Claude Code agent stage clones the PR, verifies and refines those comments against the real code, and produces the final report. Reports are rendered as HTML with per-comment deep links (`#comment-N`), J/K keyboard navigation, and a Markdown export optimized for coding agents. A deterministic layer (mechanical gates + a "bug memory" of past bug patterns) can inject forced checks into the agent's review.

**Review API** — reviews are also exposed as structured JSON (findings with file/line, diff hunks, and source context) at `/api/review/{owner}/{repo}/{pr}`, and can be generated on demand for any PR — including merged and closed ones — via `POST /api/prs/generate-review`. A bundled Claude Code skill (`skills/claude/prism-review/`) consumes this API.

## Prerequisites

- **Go 1.25+** with CGO enabled (the SQLite driver requires it)
- **Node 20+** and npm (the dashboard is built with Vite and embedded into the Go binary)
- **git** on `PATH`
- Optional: **Docker** (recommended run path), the **`claude` CLI** (installed and authenticated) for agentic reviews

## Quick Start (Docker)

```bash
cp .env.example .env   # set GITHUB_TOKEN and GITHUB_USERNAME at minimum
make start             # docker-compose up -d
```

Dashboard: <http://localhost:7769>. Use `make logs`, `make status`, `make restart`, `make rebuild`, and `make stop` to manage it.

## Building from source

The frontend must be built first — `server/dist` is embedded into the binary via `go:embed` and is not checked in, so a bare `go build` on a fresh clone fails without this step:

```bash
cd frontend && npm ci && npm run build   # outputs to ../server/dist
cd .. && go build -o pr-review-server .

export GITHUB_TOKEN=your_token       # PAT with repo + read:org scopes
export GITHUB_USERNAME=your_username
export GEMINI_API_KEY=your_key       # optional, enables AI reviews

./pr-review-server                   # dashboard at http://localhost:8080
```

## Deployment options

| Resource | Options | Notes |
|----------|---------|-------|
| **Hosting** | Any server, VM, or container platform | Dockerfile and docker-compose included |
| **Database** | SQLite (default) or PostgreSQL | Set `DATABASE_URL` for PostgreSQL; SQLite works for small teams |
| **GitHub auth** | PAT (single-user dev mode) or GitHub App | GitHub App provides OAuth login and org-wide PR access for multi-user deployments |
| **Gemini API** | Optional | Required for AI-generated reviews |
| **Anthropic / `claude` CLI** | Optional | Required for the agentic review stage (`AGENTIC_REVIEWS=true`) |
| **GCS bucket** | Optional | Stores review artifacts (HTML/Markdown + JSON findings) in cloud deployments; defaults to local disk |

Minimal deployment: a single VM with Docker and a GitHub App.

### Auth modes

- **Dev mode** (default when no GitHub App is configured): authenticates to GitHub with `GITHUB_TOKEN` and auto-logs-in `GITHUB_USERNAME`. Good for single-user/local use.
- **Multi-user mode**: set the `GITHUB_APP_*` variables, `OAUTH_CALLBACK_URL`, and `SESSION_SECRET`. Users log in via GitHub OAuth; org membership (`GITHUB_ORG_NAME`) gates access. The OAuth callback path is `/auth/github/callback`.

## Environment variables

The most common ones:

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Dev mode | GitHub PAT with `repo` and `read:org` scopes |
| `GITHUB_USERNAME` | Dev mode | Auto-login user and poller identity |
| `GITHUB_APP_*`, `OAUTH_CALLBACK_URL`, `SESSION_SECRET` | Multi-user mode | GitHub App auth (see `.env.example`) |
| `GEMINI_API_KEY` | No | Enables AI-generated reviews |
| `AGENTIC_REVIEWS` | No | Pipe reviews through a Claude Code agent stage (needs the `claude` CLI) |
| `DATABASE_URL` | No | PostgreSQL connection string; unset = SQLite at `DB_PATH` |
| `SERVER_PORT` | No | Default `8080` (docker-compose publishes it on `7769`) |
| `POLLING_INTERVAL` | No | GitHub poll cadence, default `1m` |
| `DISABLE_POLLING` | No | Run purely as an on-demand review API |

See `.env.example` for the full reference, including agent tuning (`AGENT_*`), deterministic gates (`GATE_*`), bug memory, and feature flags.

## API

- `GET /api/review/{owner}/{repo}/{pr}` — structured review JSON (`?format=html` / `?format=md` for rendered output, `?sha=` to pin a commit)
- `POST /api/prs/generate-review` — trigger a review for any PR by reference, including merged/closed PRs
- `GET /api/status` — health check

## Auxiliary tools

- `go run ./cmd/gatecheck <worktree-dir> <base-branch>` — offline report of what the deterministic layer (gates + bug memory) would contribute for a diff; no LLM calls or API keys needed
- `go run ./cmd/seed-users` — seed user rows from GitHub (PostgreSQL only)
- `skills/claude/prism-review/` — Claude Code skill for fetching and acting on reviews; install by copying it to `~/.claude/skills/prism-review/`

## Development

```bash
go test -race ./...                                        # backend tests
golangci-lint run --timeout=5m                             # lint
cd frontend && npm run lint && npm run type-check && npm test
```

For frontend work, `npm run dev` in `frontend/` starts a Vite dev server on port 3000 that proxies API calls to the Go server (`BACKEND_URL`, default `http://localhost:7769`).
