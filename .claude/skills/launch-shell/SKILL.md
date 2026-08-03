---
name: launch-shell
description: Build and run the box-dispatch interactive TUI "launch shell" (Bubble Tea), or understand why it renders a plain report instead. Use when asked to run, launch, start, or screenshot the app, or to confirm a UI change works in the real shell.
---

# launch-shell

`box-dispatch` is a full-screen Bubble Tea / lipgloss / huh "solution launch shell". Running
the binary with **no subcommand in a real TTY** starts the wizard; anything scripted falls
back to the plain connectivity report.

## Build and run

```bash
go build -o box-dispatch ./cmd/box-dispatch   # binary is gitignored (/box-dispatch)
./box-dispatch                                # launches the full-screen shell
# or, without building:
go run ./cmd/box-dispatch
```

## The TTY gate (why you might get a plain report instead of the UI)

`cmd/box-dispatch/main.go` only launches the shell when **both** are true:

- `isTTY()` — `stdout` is a terminal (`term.IsTerminal`), and
- not `--json`.

Otherwise it runs `runConnectivityCheck` and prints a report. Consequences:

- **You (an agent) cannot drive the alt-screen TUI through piped Bash** — stdout isn't a
  TTY, so it won't even enter the shell, and Bubble Tea's alt-screen can't be scraped from a
  pipe anyway. To exercise it non-interactively, use the tests or the plain report, or ask
  the user to run `./box-dispatch` themselves (they can type `! ./box-dispatch` in the
  session prompt).
- For a machine-readable check without the UI: `go run ./cmd/box-dispatch check --json`
  (add `--offline` to skip live provider calls).

## Verifying shell/UI changes without a TTY

- Unit tests: `go test ./cmd/box-dispatch/...` (`shell_test.go` covers shell behavior).
- Full gate: use the **verify-build** skill.
- For a visual of the landing screen, use the **landing-screenshot** skill (renders the
  colored welcome screen, punk-rock 🤘 and all, to `docs/landing.png`).

## Where things live

- `cmd/box-dispatch/main.go` — cobra root; `runLaunchShell()` wraps `tea.NewProgram(..., tea.WithAltScreen())`.
- `cmd/box-dispatch/shell.go` — the model: screen enum, `Update`, `View`, welcome menu,
  deploy/package/history flows. Template repos ~L518; `startPackage()` ~L733.
- `cmd/box-dispatch/shell_test.go` — shell tests.

## Runtime config it reads (for a real deploy)

Providers are configured via env vars (see `.env.sample`): Box (`BOX_ACCESS_TOKEN`,
`BOX_FOLDER_ID`, …), Salesforce (`SF_ALIAS` / `SALESFORCE_ACCESS_TOKEN`), Databricks
(`DATABRICKS_HOST`/`DATABRICKS_TOKEN`), AWS (`AWS_PROFILE`, `AWS_REGION`). `.env` is
gitignored — copy `.env.sample` to `.env` to run a real deployment.
