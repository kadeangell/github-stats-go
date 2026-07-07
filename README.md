# github-stats-go

Go rewrite of [jstrieb/github-stats](https://github.com/jstrieb/github-stats). Generates
`generated/overview.svg` and `generated/languages.svg` from the GitHub API on a daily
GitHub Actions cron and commits them back to this repo, for embedding in a profile README.

Zero dependencies — Go stdlib only.

## Why the rewrite

The Python original silently swallowed API errors and committed the resulting bad images
(zeros) over good ones. This version:

- Fails the run (exit non-zero) on any core API error, so the workflow **doesn't commit** and
  yesterday's good images stay published.
- Surfaces GraphQL `errors` payloads, retries 202/5xx with exponential backoff, honors
  `Retry-After` on rate limits.
- Paginates owned and contributed repo cursors independently (upstream advanced them in
  lockstep).
- Sanity-gates output: refuses to write images if repos or contributions come back zero.

## Setup

1. Create a repo from this code; add an `ACCESS_TOKEN` secret (classic PAT with `repo` scope,
   needed for traffic/views data).
2. Optional secrets: `EXCLUDED` (comma-separated `owner/repo` to skip), `EXCLUDED_LANGS`
   (comma-separated language names).
3. The workflow (`.github/workflows/generate.yml`) runs daily and on push; embed the images:

```markdown
![](https://raw.githubusercontent.com/<user>/<repo>/master/generated/overview.svg#gh-dark-mode-only)
![](https://raw.githubusercontent.com/<user>/<repo>/master/generated/languages.svg#gh-dark-mode-only)
```

(The `#gh-dark-mode-only` fragment triggers the dark theme baked into the SVGs.)

## Local run

```sh
ACCESS_TOKEN="$(gh auth token)" GITHUB_ACTOR=<user> EXCLUDE_FORKED_REPOS=true go run .
go test ./...
```
