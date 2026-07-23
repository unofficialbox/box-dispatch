# Archived Go prototype

These files preserve the original engine, model, and provider implementation as
plain text. They were removed from the Go build on July 21, 2026 because they
duplicated provider policy, contained obsolete credential assumptions, and no
longer compiled against the active model.

The active implementation is:

- `internal/checker`: provider discovery and connectivity adapters
- `internal/config`: non-secret local connection selections
- `cmd/windlass`: Bubble Tea setup wizard and command entry points

Do not restore these packages as a second execution path. Recover any useful
behavior by implementing it through the canonical provider adapter registry.
