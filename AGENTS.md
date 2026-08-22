# AGENTS.md — box-dispatch

Guidance for AI coding agents (Codex, Cursor, Claude Code, and any tool that reads
`AGENTS.md`) working in this repo. Claude Code additionally has invokable skills under
`.claude/skills/` that expand on the screenshot pipeline and the TTY gate below.

## What this project is

`box-dispatch` is a Go CLI whose default mode is a full-screen **Bubble Tea / lipgloss / huh
"solution launch shell"** that packages and deploys Box + partner (Salesforce, Databricks,
AWS Bedrock AgentCore) solution stacks. Module `github.com/unofficialbox/box-dispatch`,
`go 1.26`.

Run with **no subcommand in a real TTY** → the interactive wizard. Scripted or `--json` →
a plain connectivity report (`runConnectivityCheck`).

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

## Building and running the shell

```bash
go build -o box-dispatch ./cmd/box-dispatch   # binary is gitignored (/box-dispatch)
./box-dispatch                                # full-screen shell (needs a real terminal)
go run ./cmd/box-dispatch                     # same, without building
```

### The TTY gate (important for agents)

`cmd/box-dispatch/main.go` launches the shell only when **stdout is a TTY** (`isTTY()`,
`term.IsTerminal`) **and** not `--json`. Consequences:

- **You cannot drive the alt-screen TUI through piped/non-interactive shells** — it won't even
  enter the shell, and Bubble Tea's alt-screen can't be scraped from a pipe. Verify shell
  changes with the tests instead, or ask the human to run `./box-dispatch`.
- Machine-readable check without the UI: `go run ./cmd/box-dispatch check --json`
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

- `cmd/box-dispatch/main.go` — cobra root; `runLaunchShell()` (alt-screen); the TTY gate.
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
