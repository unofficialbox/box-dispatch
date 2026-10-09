# Handoff: Box Dispatch web workspace and Box Open Elements adoption

## Current state

- **Date:** 2026-10-09
- **Repo:** `/Users/massnerder/Developer/unofficialbox/box-dispatch`
- **Branch:** `main`
- **Remote main:** `9f546a5c0ddfe4b6f446858f31b4d73613046cd1`
- **Module:** `github.com/unofficialbox/box-dispatch`, Go 1.26
- **Web packages:** `@unofficialbox/box-open-elements` and
  `@unofficialbox/box-open-elements-react` 0.28.4

The default no-subcommand experience is the React browser workspace served by the Go
application. The older August handoff describes a Bubble Tea launch shell and should not
be used as the current UI architecture reference. `AGENTS.md` and the current source are
authoritative.

## Recently merged work

### PR #26 — web workspace audit and Box Open Elements adoption

[PR #26](https://github.com/unofficialbox/box-dispatch/pull/26) was squash-merged to
`main` as `dd5785d501dfd75bb8c9f296359cfd7d0d1c5f52`.

It delivered and verified:

- live connection-status refresh whenever a page presents connection readiness;
- the published `box-path` lifecycle component in the deployment header;
- `box-resource-row` for selectable provider and saved-connection rows;
- `box-tile-group` for connection modes and rich solution choices;
- the published Box Open Elements app shell, navigation sidebar, breadcrumb, tables,
  timelines, metrics, sections, fact lists, dialogs, drawers, and form adapters where
  their semantic contracts fit;
- expanded sidebar rows with left-aligned icons and labels, without the artificial active
  stripe treatment;
- Box subdomain and Salesforce My Domain connection identity;
- provider-specific Open actions in provider summary headers;
- historical deployment links, component details, and preserved validation/deployment
  change previews;
- completed deployments excluded from the resumable Overview area;
- an Overview connection-row layout that keeps Ready and Not ready badges inside the
  provider card at desktop and narrow widths; and
- a full four-sided selected border for the active deployment template, including state
  transfer when another template is selected.

### PR #27 — Box Open Elements 0.28.4

[PR #27](https://github.com/unofficialbox/box-dispatch/pull/27) was squash-merged to
`main` as `9f546a5c0ddfe4b6f446858f31b4d73613046cd1`.

- Core and React adapter packages were upgraded together from 0.28.1 to 0.28.4.
- Releases 0.28.2 through 0.28.4 focus on Verdict Banner, Process Modeler, and Code
  Editor behavior. No Dispatch component adaptation was required.
- The package lock and Box Open Elements migration/gap documentation are current.

## Box Open Elements boundary

Use [`docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md`](docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md)
as the migration authority and
[`docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md`](docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md)
as the adopted/retained/gap inventory.

All planned migration phases are complete within published 0.28.4 contracts. Keep the
following behavior Dispatch-owned:

- routing, API calls, provider launch URLs, and deployment resume state;
- `ProviderLogo` and provider-specific identity copy;
- run and live-activity API-to-view-model translation;
- validation/deployment diff data loading and file selection state;
- page composition and domain terminology; and
- read-only connection summary composition where selection semantics do not apply.

Do not replace semantic page structure merely to increase the component count. Adopt a
library component only when its interaction and accessibility contract matches the
Dispatch surface.

## Remaining upstream dependencies

Two Box Open Elements enhancements remain open. They are not Dispatch release blockers:

1. [box-open-elements #379](https://github.com/unofficialbox/box-open-elements/issues/379)
   — generated JSX intrinsic-element declarations and typed custom-event maps. Until a
   containing release ships, retain the narrow local `web/src/boe.d.ts` boundary and
   native event listener casts.
2. [box-open-elements #380](https://github.com/unofficialbox/box-open-elements/issues/380)
   — density-safe Run Trace marker and connector geometry tokens. Until released, keep
   the coordinated `step`, `marker`, and connector CSS overrides plus the existing E2E
   geometry assertion.

Do not file duplicate issues for these needs. Recheck the published package, not only the
issue state, before removing a local compatibility boundary.

## Verification evidence

The final 0.28.4 baseline passed:

- 21 Vitest files / 62 unit tests;
- all 4 Playwright mock workflows, including the 390px mobile path;
- frontend lint and production build;
- `gofmt -l .`, `go build ./...`, `go vet ./...`, and `go test ./...`;
- live browser smoke on Overview and the Choose-a-solution deployment step; and
- desktop and narrow visual checks for the status-badge containment and selected-template
  border changes.

The production build still reports the pre-existing JavaScript chunk-size warning. Treat
that as a separate performance task unless a future change measurably increases it.

Human VoiceOver validation was explicitly deferred for this internal demo. Do not report
it as complete. Reintroduce it if production, compliance, or formal accessibility sign-off
requires it.

## Build and run

From the repository root:

```bash
go build -o box-dispatch ./cmd/box-dispatch
./box-dispatch
```

Run without opening a browser:

```bash
go run ./cmd/box-dispatch --no-open
```

Run the clean mock backend used by Playwright:

```bash
go run ./cmd/box-dispatch mock --no-open --port 8788
```

Frontend checks run from `web/`:

```bash
bun install
bun run test
bun run lint
bun run build
bun run test:e2e
```

Before every commit, push, or merge, run from the repository root:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```

Any path printed by `gofmt -l .` is a failure.

## Test and mock notes

- Restart the mock backend cleanly if a manual package seed leaks completed-deployment
  state into the E2E suite.
- A 404 from `/api/salesforce/scratch-orgs/latest` is expected in the mock profile and is
  tolerated through `Promise.allSettled`.
- Keep core and React adapter package versions in lockstep for future upgrades.
- Regenerate and commit `internal/webui/dist` through `bun run build` whenever frontend
  source changes affect the embedded application.

## Continuation point

- **Current Status:** The Box Open Elements migration, connection refresh behavior,
  deployment-path adoption, sidebar redesign, Overview status containment, selected
  template treatment, and 0.28.4 upgrade are merged to `main` and verified.
- **Recommended Next Step:** Monitor #379 and #380 and adopt their first containing
  release; otherwise begin the next product feature from a new branch based on `main`.
- **Why This Next:** No planned Dispatch migration work remains that can be completed
  locally without those upstream contracts.
- **Expected Outcome:** Removal of the remaining local JSX/event declarations and coupled
  Run Trace geometry override when the corresponding public APIs ship.
- **Blockers:** Upstream releases are required for those two cleanup items. There is no
  blocker for unrelated Dispatch product work.
