# Handoff: box-dispatch launch-shell polish + teardown groundwork

## Date / context
- **Date:** 2026-08-03
- **Repo:** `/Users/massnerder/Developer/unofficialbox/box-dispatch`
- **Branch:** `restore-launch-shell` (tip `1871d72`). `main` is at `4a55ac0`.
- **Module:** `github.com/unofficialbox/box-dispatch`, `go 1.26`.
- **What this is:** `box-dispatch` is a full-screen Bubble Tea / lipgloss / huh "solution
  launch shell" that packages and deploys Box + partner (Salesforce / Databricks / AWS)
  solution stacks. Run with no subcommand in a TTY → the wizard; scripted/`--json` → a plain
  connectivity report.

## Branch / merge state
- `main` is **2 commits behind** `restore-launch-shell`, and `main` **is an ancestor** of the
  branch — a clean fast-forward is possible (no merge commit needed):
  - `c4d027b chore: gitignore .env and add .env.sample template`
  - `1871d72 fix: run template clone non-interactively so packaging can't hang`
- To land them: `git checkout main && git merge --ff-only restore-launch-shell && git push`.
  (Confirm with the user first — they drive merges to main.)
- Working tree is clean except untracked `.claude/` (the new skills below + a personal
  `settings.local.json`). **Commit the skills** (`.claude/skills/`); leave
  `.claude/settings.local.json` untracked/personal.

## What landed this cycle (already committed/pushed to the branch)
1. **Cleanup + parallel-history merge to reconcile with origin/main.** The branch was proven
   to be the superset; add/add conflicts were resolved to "ours". Build + tests green.
2. **README hero image.** `docs/landing.png` — a colored screenshot of the welcome screen
   with a real punk-rock 🤘. Referenced under the `# box-dispatch` heading.
3. **`.env` hygiene.** `.env` is now gitignored; `.env.sample` documents every provider env
   var (Box / Salesforce / Databricks / AWS), marking secrets and setup-managed keys.
   ⚠️ **Untracking does not scrub history** — old `.env` values still live in past commits.
   Low sensitivity (no live tokens were in it), but if the user wants them gone it needs
   `git filter-repo`/BFG + a force-push to `main`. Not done; needs explicit consent.
4. **Packaging hang fix** (`internal/workspace/package.go:49`). The template clone inherited
   the environment and could fall back to git's interactive
   `Username for 'https://github.com':` prompt, which is invisible/unanswerable inside the
   alt-screen shell → it hung forever (the blank `github.com/` line the user saw). Fixed to
   run **non-interactively** (`GIT_TERMINAL_PROMPT=0`, `GCM_INTERACTIVE=Never`,
   `cmd.Stdin=nil`) so it **fails fast with git's real output** instead of hanging.

### Open diagnostic thread (packaging)
All four template repos (`box-bedrock-for-clm`, `-lifesciences`, `-citizen-services`,
`box-bedrock-template`) are **public** — a clean machine needs no auth. So the prompt on the
user's other machine was **machine-specific**: a git credential helper intercepting
github.com, a corporate proxy returning 401, or an `insteadOf` URL rewrite. **Next action for
that thread:** have the user re-run the deploy on that machine and send the new error message
(it now surfaces git's real output), then clear the specific trigger (e.g.
`gh auth setup-git`, unset a bad credential helper, or fix the proxy).

## New agent skills (in `.claude/skills/`, commit these)
Invoke with the Skill tool; each has a `SKILL.md`:
- **`verify-build`** — the Go quality gate to run before every commit/push/merge:
  `gofmt -l .` → `go build ./...` → `go vet ./...` → `go test ./...`. All must be clean.
- **`launch-shell`** — how to build/run the TUI, and the **TTY gate** (why piped Bash gets a
  plain report, not the alt-screen UI) so an agent doesn't chase a dead end. Points to the
  tests and the screenshot skill for non-interactive verification.
- **`landing-screenshot`** — the non-obvious pipeline to regenerate `docs/landing.png` with a
  real color emoji: force `termenv.TrueColor` (lipgloss strips color off-TTY) → capture ANSI
  → `ansi2html.py` (bundled) → headless-Chrome screenshot at 2×. Explains why `freeze` can't
  be used (resvg can't render Apple Color Emoji).

## The big pending feature — "Reset demo environment" (teardown)
A full implementation plan exists at
`~/.claude/plans/generic-scribbling-fiddle.md` (**read it before starting**). Summary:

- **Goal:** a **Reset** action in the shell that deletes exactly what a past deployment
  created, driven entirely by the deployment audit trail, **by ID only** (never name-matching,
  so it can't touch resources dispatch didn't create).
- **Scope:** Box + Salesforce (Databricks/AWS record no resources). Box Forms/Apps are deleted
  through the authenticated browser adapter (same mechanism that creates them).
- **Prerequisite gaps the plan closes first:**
  - `createAIAgent`/`createHub` currently discard the created IDs
    (`internal/lifecycle/box_sdk.go` ~L362/L384) → hubs and AI agents are invisible to
    teardown. Must record their IDs.
  - `deployBoxPublicAdapters` only appends a `ResourceReference` for `docgen_template`
    (`box_public_adapters.go` ~L104) → add `hub` and `ai_agent`.
  - The audit record is only written on an explicit keypress after deploy
    (`shell.go` ~L1171). Auto-persist it when the deploy queue drains (~L766) or reset has no
    trail to act on.
  - `boxAPI` has **no delete methods** (`box_sdk.go:21-35`) — add `deleteFolder/File/
    MetadataTemplate/AIAgent/Hub/DocgenTemplate` to both backends (`*boxSDK` via typed
    managers; `boxCLI` via the existing `boxRequest(ctx,"DELETE",...)` escape hatch, box_sdk.go:182;
    hubs/docgen need header `box-version: 2025.0`). **Automate workflows have no delete API** →
    report as unmanaged.
- **New engine:** `internal/lifecycle/teardown.go` with
  `DestroyProvider(root, provider, resources)` mirroring `DeployProvider`; delete Box
  resources in reverse dependency order, workspace folder last (recursive); refuse folder ID
  `"0"` (mirror the `loadBoxTarget` guard, lifecycle.go:496). Salesforce via a generated
  `destructiveChanges.xml` + `sf project deploy start`. Continue-on-error per item.
- **Shell UI:** new `screenTeardown`, a third welcome option "Reset demo environment"
  (update the hardcoded `options` slice ~shell.go:1477 **and** `m.moveCursor(key, 2)` ~L943 in
  lockstep), plus an Enter action on a history row. Preview the exact resources, then a
  destructive confirm (`huh.NewInput().Validate` requiring the workspace name typed) modeled
  on `requestDeployConfirmation` (shell.go:689).
- **Caveat flagged in the plan:** the browser Form/App delete calls are **unverified against a
  live tenant** — they must fail soft and report the surface as remaining, not abort the reset.

## Other known follow-ups (from persistent memory)
- **BCL emitter round-trip gap:** `WriteBCL` output parses but extracts zero artifacts — flat
  summaries vs. nested extraction. Investigation pending. (See memory `bcl-emitter-roundtrip-gap`.)
- **UI restore vs BCL split:** keep the windlass-era shell styling; the BCL migration is the
  keeper work. (See memory `ui-restore-vs-bcl-split`.)

## Verification (always, before reporting done)
Run the **`verify-build`** skill, or by hand from the repo root:
```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```
All four must be clean. As of this handoff, they are.

## Where things live (quick map)
- `cmd/box-dispatch/main.go` — cobra root; `runLaunchShell()` (alt-screen), the TTY gate.
- `cmd/box-dispatch/shell.go` — the shell model: screen enum, `Update`/`View`, welcome menu,
  package/deploy/history flows. `newDispatchShell()` L426; `View()` L2053.
- `internal/workspace/package.go` — template clone + component pruning (packaging fix here).
- `internal/lifecycle/` — deploy engine + Box/Salesforce backends + browser adapters
  (teardown work lands here).
- `internal/audit/deployment.go` — deployment records (`ListDeployments`; needs
  `FindDeployment`).
- `internal/bcl/`, `internal/engine/` — BCL artifact contract + import/export.

---

## Continuation point
- **Current Status:** Launch-shell polish is complete and green on `restore-launch-shell`
  (docs image, `.env` hygiene, packaging-hang fix). Three dev skills and this handoff are
  written but not yet committed. `main` is a clean fast-forward behind the branch.
- **Recommended Next Step:** With the user's OK, fast-forward `main` to `1871d72` and commit
  the new `.claude/skills/` + this handoff.
- **Why This Next:** It's zero-risk (ff-only, tree already green) and gets the packaging fix
  and dev tooling onto `main` where the other machine and future agents can use them.
- **Expected Outcome:** `main` carries the fix and tooling; the next agent can pick up the
  Reset/teardown feature from the plan file with the skills in place.
- **Blockers:** (1) The user drives merges to `main` — confirm before pushing. (2) The
  packaging root-cause on the other machine still needs that machine's new error output to
  close out. (3) History still contains old `.env` values — scrubbing needs explicit consent.
