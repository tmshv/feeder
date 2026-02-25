# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run Commands

```bash
go build -o feeder .           # Build binary
go run . serve                 # Run the server (listens on :3000)
go test -v -covermode=count ./...  # Run all tests with coverage
go test ./utils/...            # Run tests in a specific package
```

The CLI uses Kong for subcommands: `serve`, `add`, `import`, `update` (only `serve` is implemented).

## Architecture

Feeder is a Go RSS/Atom feed aggregator that periodically fetches feeds, stores articles in SQLite, extracts readable content via readability, and serves JSON Feed output over HTTP.

### Core Flow (`main.go`)

`run()` is the entry point for `serve`: it opens the SQLite store, starts the Fiber HTTP server in a goroutine, then spawns goroutines per feed (`runFeed`) that poll on a `RefreshMs` interval. New article URLs are sent to a buffered `news` channel, consumed by 3 `handleRecords` workers that fetch the full page, extract readable content, and convert HTML to Markdown.

### Package Layout

- **`internal/`** — Domain types: `Feed`, `Record`, `Page`
- **`store/`** — `Store` interface + `SqliteStore` implementation. Migrations run automatically on startup from `migrations/` directory using golang-migrate.
- **`utils/`** — URL helpers (UTM parameter stripping)
- **`migrations/`** — SQL migration files (3 tables: `feeds`, `records`, `pages`)

### Key Dependencies

- **gofiber/fiber** — HTTP server
- **mmcdole/gofeed** — RSS/Atom parser
- **cixtor/readability** — Article content extraction
- **JohannesKaufmann/html-to-markdown** — HTML-to-Markdown conversion
- **mattn/go-sqlite3** — SQLite driver (CGO required)
- **golang-migrate/migrate** — Database migrations
- **alecthomas/kong** — CLI argument parsing

### Database

SQLite database file: `feed.db`. Migrations in `migrations/` are applied automatically on store initialization. The `records` table uses `INSERT OR IGNORE` with a `UNIQUE(link, published_at)` constraint for deduplication.

### HTTP API

- `GET /` — Health check
- `GET /feed/:slug` — Returns a JSON Feed (v1.1) for the given feed slug
