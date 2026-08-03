# Handoff: box-dispatch launch shell, BCL, and reset

## Current state

- **Date:** 2026-08-03
- **Repo:** `/Users/massnerder/Developer/unofficialbox/box-dispatch`
- **Branch:** `restore-launch-shell`; use `git log --oneline --decorate -5` for the current
  tip rather than relying on a handoff-embedded commit hash.
- **Module:** `github.com/unofficialbox/box-dispatch`, Go 1.26.
- **Working tree at handoff review:** clean before this documentation refresh.

`box-dispatch` is a full-screen Bubble Tea/lipgloss/huh solution launch shell. With no
subcommand, a real TTY starts the shell; redirected output or `--json` runs the plain
connectivity report. Use `box-dispatch check` for an explicit connectivity check.

## Completed work

### Launch shell and packaging

- The windlass-era launch shell is restored as the default interactive experience and has
  the current Box Dispatch styling, progress views, history, deployment results, and reset
  flow.
- Template cloning is non-interactive (`GIT_TERMINAL_PROMPT=0`,
  `GCM_INTERACTIVE=Never`, and no stdin), so an authentication or proxy problem fails with
  the real Git error instead of hanging inside the alt-screen UI.
- `.env` is gitignored and `.env.sample` documents the provider configuration.
- `docs/landing.png` is the current README hero image.

### Reset demo environment

Reset/teardown is implemented rather than pending:

- The welcome screen exposes **Reset demo environment** and deployment history rows can open
  the reset preview.
- Deployment audit records are persisted automatically and include the created resource IDs
  teardown needs.
- Reset previews the exact recorded inventory and requires the operator to type the package
  name before deletion starts.
- Box resources are deleted strictly by recorded ID, in dependency-safe order, with the
  workspace folder last. Folder ID `0` is always refused.
- Box files, folders, metadata templates, AI agents, hubs, and Doc Gen templates use public
  provider APIs. Capabilities without a public API are excluded from the product surface.
- Box Forms, Box Apps, and Box HTTPS Connectors are absent from manifests, packaging,
  validation, deployment, and reset. Their missing public lifecycle APIs are recorded in
  [`docs/PUBLIC_API_GAPS.md`](docs/PUBLIC_API_GAPS.md).
- Salesforce metadata teardown uses a generated destructive-changes deployment.
- Unsupported providers and resource types are reported as remaining/manual instead of
  being silently treated as deleted.
- Per-resource failures do not abort the rest of the reset; the result screen distinguishes
  deleted, manual, and failed resources.

Key implementation files:

- [`internal/lifecycle/teardown.go`](internal/lifecycle/teardown.go)
- [`internal/lifecycle/teardown_test.go`](internal/lifecycle/teardown_test.go)
- [`internal/audit/deployment.go`](internal/audit/deployment.go)
- [`cmd/box-dispatch/shell.go`](cmd/box-dispatch/shell.go)
- [`cmd/box-dispatch/shell_test.go`](cmd/box-dispatch/shell_test.go)

### BCL artifact flow

- BCL is the single admin-facing import/export contract.
- `resolve` and `bootstrap` emit `<provider>-artifacts.bcl`; `import` accepts either one
  `.bcl` file or a directory of `*artifacts.bcl` files.
- The HCL-like form emitted by Box Dispatch is accepted by the loader and extracts resource
  IDs correctly. Worked HCL and JSON examples are covered by import tests.
- The shell reads migrated BCL configuration artifacts.

See [`BCL_ARTIFACT_CONTRACT.md`](BCL_ARTIFACT_CONTRACT.md) and
[`examples/bcl/README.md`](examples/bcl/README.md).

### Live deploy/reset validation

On 2026-08-03, a minimal Box-only CLM package was built with `create_new` and only the
folder-tree capability enabled. The run:

- authenticated to Box through the saved CCG connection;
- created one uniquely named workspace and nine child folders;
- automatically wrote an audit record containing all 10 resource IDs; and
- reset the deployment by recorded ID, deleting all 10 folders with zero remaining.

The preflight also exposed a package-contract gap: the upstream CLM template does not include
the old `config/box/folder-template.md` marker. The workspace is already fully declared in
`dispatch.json`, so Box Dispatch now includes the enabled workspace directly from the
manifest and no longer requires that redundant marker. A regression test covers this path.

### Agent tooling

The development tooling is committed:

- `AGENTS.md` is the cross-agent source of truth.
- `.cursor/rules/box-dispatch.mdc` mirrors the always-on Cursor guidance.
- `.claude/skills/verify-build/`, `launch-shell/`, and `landing-screenshot/` document the
  repo-specific verification and visual workflows.

Keep these surfaces aligned when the workflow changes; `AGENTS.md` is canonical.

## Operational risks and follow-ups

1. **Validate Salesforce reset.** The Box folder-tree deploy/audit/reset path is proven.
   Salesforce destructive teardown still needs validation against a disposable org.
2. **Reproduce the other machine's template-clone failure.** The public template repos do
   not require authentication. Re-run there and capture the now-visible Git error before
   changing credential helpers, URL rewrites, or proxy configuration.
3. **Partial public APIs remain explicit.** Automate workflows can be inspected through the
   public API but cannot be created or deleted; the gap is tracked in
   [`docs/PUBLIC_API_GAPS.md`](docs/PUBLIC_API_GAPS.md).
4. **Old `.env` values remain in Git history.** The file is no longer tracked and no live
   tokens were identified, but history rewriting would require explicit approval and a
   coordinated force-push.
5. **The branch name is historical.** `restore-launch-shell` is the active work branch; do
   not follow older fast-forward or merge instructions from previous handoff text.

## Verification gate

Run from the repository root before committing or pushing:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```

Any output from `gofmt -l .` is a failure.

## Continuation point

- **Current Status:** The launch shell, cleanup-safe reset flow, BCL import/export path, and
  cross-agent tooling are implemented. The Box folder deploy/audit/reset path is also proven
  live with complete cleanup.
- **Recommended Next Step:** Test Salesforce destructive teardown in a disposable org.
- **Why This Next:** It is the remaining live reset boundary not proven by the hermetic Go
  tests or the completed Box folder-tree smoke.
- **Expected Outcome:** Confirmed Salesforce cleanup behavior with exact errors for any path
  that still needs hardening.
- **Blockers:** The Salesforce test needs a selected authenticated disposable org.
