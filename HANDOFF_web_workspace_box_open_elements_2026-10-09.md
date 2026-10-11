# Handoff: Box Dispatch web workspace and Box Open Elements adoption

## Current state

- **Date:** 2026-10-11
- **Repo:** `unofficialbox/box-dispatch`; cloud checkout `/workspace/box-dispatch-cloud-verified`
- **Branch:** `codex/box-open-elements-0.37.0`
- **Base main:** `c1663ac0b992749245b5b6472a6736d9c581a772` (PR #40)
- **Module:** `github.com/unofficialbox/box-dispatch`, Go 1.26
- **Web packages:** `@unofficialbox/box-open-elements` and
  `@unofficialbox/box-open-elements-react` 0.37.0

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

### PR #37 — exact-cascade CSS consolidation

[PR #37](https://github.com/unofficialbox/box-dispatch/pull/37) was squash-merged to
`main` as `0c2be65c223c9e616565ae78261c87d3def44801`.

- Removed 390 earlier declarations superseded by the same selector, property,
  importance, and at-rule context.
- Preserved all 2,377 final cascade winners and reduced `App.css` by about 10.4 KB.
- Regenerated the embedded web assets and documented the safe cleanup boundary.

### PR #38 — Box Open Elements 0.28.6

- Core and React adapter packages were upgraded together from 0.28.4 to 0.28.6.
- Releases 0.28.5 and 0.28.6 change Process Modeler, Code Editor, and Form Wizard;
  Dispatch imports none of those surfaces, so no local component adaptation is needed.
- The change was squash-merged to `main` as `604878e77a1e9f39880c9d9918eabb0f29269fea`.

### PR #39 — Box Open Elements 0.33.0 (merged)

- Core and React adapter packages are upgraded together from 0.28.6 to 0.33.0.
- Version 0.29.0 ships the declarations from issue #379 and Run Trace geometry tokens
  from issue #380. Dispatch adopts both published contracts and removes the matching
  local compatibility code.
- Releases 0.30.0 through 0.33.0 add Process Modeler capabilities that Dispatch does
  not consume, so they require no additional application adaptation.

### PR #40 — charcoal and mint theme (merged)

Main includes the charcoal/dark-mint and white/light-green themes. The 0.37 update
preserves `web/src/theme.ts`, all application CSS, and existing theme assertions.

### Current branch — Box Open Elements 0.37.0

Both packages upgrade from 0.33.0 to 0.37.0 with no other lockfile resolution
changes. Published APIs and generated declarations remain compatible. The optional
new component features do not require Dispatch application changes. Embedded assets
and the adoption notes are refreshed. This branch is prepared for review; merge and
deployment require separate approval.

## Box Open Elements boundary

Use [`docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md`](docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md)
as the migration authority and
[`docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md`](docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md)
as the adopted/retained/gap inventory.

All planned migration phases are complete within published 0.37.0 contracts. Keep the
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

## Resolved upstream dependencies

Two former Box Open Elements dependencies shipped in 0.29.0 and are adopted here:

1. [box-open-elements #379](https://github.com/unofficialbox/box-open-elements/issues/379)
   — generated JSX intrinsic-element declarations and typed custom-event maps. Dispatch
   imports the official declarations and removes `web/src/boe.d.ts` and custom-event
   casts.
2. [box-open-elements #380](https://github.com/unofficialbox/box-open-elements/issues/380)
   — density-safe Run Trace marker and connector geometry tokens. Dispatch now uses the
   public tokens and retains its E2E geometry assertion.

Do not restore either compatibility boundary. Recheck the published package, not only
issue state, before changing a future integration contract.

## Verification evidence

The cloud continuation tested exact main `c1663ac0b992749245b5b6472a6736d9c581a772`
on 0.33.0 first, then the 0.37.0 upgrade:

- 23 Vitest files / 77 unit tests, frontend lint, TypeScript and production build;
- all 5 isolated Playwright mock workflows before and after the upgrade, plus a
  second instrumented run at each version;
- desktop and 390px Chromium, both system themes, keyboard activation, modal focus
  containment, Escape/focus restoration, validation file selection, history links,
  replay/reload, failure recovery, and Run Trace geometry;
- a further four-combination check (1440px/390px, light/dark) of metrics, native icon
  activation, connection drawer focus restoration, and keyboard table sorting;
- `gofmt -l .` (empty), `go build ./...`, `go vet ./...`, and `go test ./...`;
- axe-core 4.14.0 WCAG A/AA checks and screenshots at 28 matched workflow/page states
  before and after the upgrade, with no page overflow or new violation targets after
  normalizing dynamic event timestamps.

Automated accessibility is **not clean** at the baseline. Existing findings include
badge/body-copy contrast, sidebar toggle `aria-controls` across a shadow boundary,
visible-label/accessibility-name mismatch for brand/provider actions, mobile Path
scroll focus, and prohibited ARIA on a historical-detail surface. These remain
separate follow-up work; this dependency update does not alter the theme or claim
formal accessibility conformance. Table sorting also replaces the focused header
button; repeated keyboard sorts currently require refocusing it. Both published
0.33.0 and 0.37.0 table implementations rebuild the header in `update()`. The
four-combination sort check explicitly refocuses between activations. This should
be addressed through the upstream table contract rather than a shadow-DOM patch.
Axe also reports incomplete checks requiring
interpretation. Chromium keyboard, focus, semantics and automated checks are the
acceptance scope; Safari and human screen-reader checks are not required.

No browser runtime exceptions or console warnings occurred. Expected mock HTTP
failures include scratch-org lookup 404s and deliberately exercised authentication
recovery failures. The pre-existing bundle warning remains: main JavaScript changes
from 1,363.02 to 1,369.57 kB (gzip 388.28 to 389.87 kB).

Cloud tools: Go 1.26.0, Bun 1.4.2, system Chromium 151.0.7922.173. Source
`/workspace/box-dispatch-cloud-env.sh` before commands. The cloud Playwright override
uses system Chromium because the managed browser download was unavailable. Port
8788 is reserved for isolated mock tests. Screenshots and structured audit evidence
are outside the repo in `/workspace/box-dispatch-cloud-evidence/boe037/`.

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

- **Current Status:** The 0.37.0 core/React upgrade is implemented and verified on
  `codex/box-open-elements-0.37.0`, based on merged PR #40. Official native types and
  Run Trace tokens remain adopted; application source and themes are unchanged.
- **Recommended Next Step:** Review the upgrade diff and its draft pull request.
- **Expected Outcome:** Dispatch consumes both 0.37.0 packages in lockstep.
- **Blockers:** No upgrade blocker found. Existing automated accessibility findings
  and bundle-size warning remain follow-up work. Merge/deploy await approval.
