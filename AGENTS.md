# AGENTS.md — box-dispatch

Guidance for AI coding agents (Codex, Cursor, Claude Code, and any tool that reads
`AGENTS.md`) working in this repo. Claude Code additionally has invokable skills under
`.claude/skills/` that expand on the screenshot pipeline and the TTY gate below.

## What this project is

`box-dispatch` is a Go application whose default interactive mode serves and opens the React
browser workspace while the same process owns the loopback Go API. The browser experience
packages and deploys Box + partner solution stacks. Module
`github.com/unofficialbox/box-dispatch`, `go 1.26`.

Run with **no subcommand** → the complete browser application. The executable serves the
embedded interface and local API together, then opens the browser. Use `box-dispatch check`
for connectivity reports. The previous full-screen Bubble Tea interface remains available
through `box-dispatch terminal`.

## Verification gate — run before every commit/push/merge

All four must be clean. Do not report work done, and do not commit, until they pass.

```bash
gofmt -l .          # any path printed = a FAILURE, not a warning; fix with `gofmt -w <file>`
go build ./...      # must exit 0
go vet ./...        # must exit 0, no diagnostics
go test ./...       # all packages ok; tests are hermetic (no live provider calls)
```

One-liner:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```

Rules:
- `gofmt -l .` is the authoritative formatter check — not `go fmt`. Two gofmt slips
  (struct-field alignment, trailing newline) have shipped here before; never skip it.
- If `go test` fails, report the failing package and its real output. Never call the suite
  green when it isn't.
- A test that wants network or credentials is a bug in the test, not a reason to skip the gate.

## Building and running Dispatch

```bash
go build -o box-dispatch ./cmd/box-dispatch   # binary is gitignored (/box-dispatch)
./box-dispatch                                # web UI + local API; opens the browser
go run ./cmd/box-dispatch                     # same, without building
./box-dispatch terminal                       # legacy full-screen terminal interface
```

### Launch behavior (important for agents)

`cmd/box-dispatch/main.go` always launches the complete browser application when no subcommand
is supplied. Consequences:

- `go run ./cmd/box-dispatch --no-open` starts the embedded UI and local API without opening a browser.
- Run `./box-dispatch terminal` to exercise the legacy alt-screen TUI.
- Machine-readable connectivity check: `go run ./cmd/box-dispatch check --json`
  (add `--offline` to skip live provider calls).
- Shell tests: `go test ./cmd/box-dispatch/...` (`shell_test.go`).

## Runtime configuration

Providers are configured by env vars — see **`.env.sample`** for the full, commented list.
Copy it to `.env` (which is **gitignored**; never commit real tokens) to run a real deploy.
The checker looks for: a Box CCG app saved by Dispatch, or Box `BOX_CLIENT_ID`,
`BOX_CLIENT_SECRET`, and `BOX_REFRESH_TOKEN`; Salesforce `SF_ALIAS` (or
`SALESFORCE_ACCESS_TOKEN`); Databricks `DATABRICKS_HOST` + `DATABRICKS_TOKEN`; AWS
`AWS_PROFILE` + `AWS_REGION`/`AWS_DEFAULT_REGION`.

## Repository map

- `cmd/box-dispatch/main.go` — cobra root; default browser launch; `runLaunchShell()` (alt-screen).
- `cmd/box-dispatch/shell.go` — the shell model: screen enum, `Update`/`View`, welcome menu,
  package/deploy/history flows. `newDispatchShell()` ~L426; `View()` ~L2053.
- `internal/workspace/package.go` — template clone + component pruning. The clone runs
  **non-interactively** (`GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=Never`, `Stdin=nil`) so a
  credential challenge fails fast instead of hanging invisibly inside the shell. Keep it so.
- `internal/lifecycle/` — deploy engine + public Box/Salesforce backends.
- `internal/audit/deployment.go` — deployment audit records (`ListDeployments`).
- `internal/bcl/`, `internal/engine/` — BCL is the single import/export artifact contract
  (`BCL_ARTIFACT_CONTRACT.md`).
- `internal/solution/` — typed BCL solution/deployment configuration, bundled manifests,
  and BCL-only package configuration validation.
- `internal/{checker,config,boxconn,providers,shellstate,model}` — supporting packages.

## Conventions

- Match the surrounding code's idiom, naming, and comment density. This codebase favors
  small per-method inline helpers over broad abstractions.
- Keep the clone non-interactive (see `package.go` above) — it is a deliberate anti-hang fix.
- Delete/teardown work, when added, must operate **by recorded resource ID only** (never
  name-matching) and must refuse Box folder ID `"0"`. See the plan referenced in the current
  `HANDOFF_*.md`.
- Regenerating the README hero image (`docs/landing.png`) has a specific pipeline (force
  truecolor → ANSI → HTML → headless-Chrome screenshot, because `freeze`/resvg can't render
  color emoji). Claude Code users: see the `landing-screenshot` skill. Others: see
  `.claude/skills/landing-screenshot/SKILL.md` and its bundled `ansi2html.py`.

## Handoffs

The latest `HANDOFF_*.md` at the repo root carries current branch state, recent fixes, and
pending features (notably the "Reset demo environment" teardown work). Read the newest one
before starting substantial work.
