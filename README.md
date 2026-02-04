# PR Review Server

AI-powered code review dashboard for GitHub pull requests.

## Quick Start

```bash
export GITHUB_TOKEN=your_token
export GITHUB_USERNAME=your_username
export GEMINI_API_KEY=your_gemini_key

go run .
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Yes | GitHub PAT with `repo` and `read:org` scopes |
| `GITHUB_USERNAME` | Yes | Your GitHub username |
| `GEMINI_API_KEY` | No | Required for AI reviews |
| `SERVER_PORT` | No | Default: 8080 |
| `POLLING_INTERVAL` | No | Default: 1m |
