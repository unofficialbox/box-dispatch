# Handoff: Box Dispatch web workspace and Box Open Elements adoption

## Current state

- **Date:** 2026-10-11
- **Repo:** `unofficialbox/box-dispatch`; cloud checkout `/workspace/box-dispatch-cloud-verified`
- **Branch:** `codex/boe-0-39-upgrade`
- **Base main:** `331273e727218be46bfb76f54ea9bd80e2dc27ca` (merged PR #42)
- **Module:** `github.com/unofficialbox/box-dispatch`, Go 1.26
- **Web packages:** `@unofficialbox/box-open-elements` and
  `@unofficialbox/box-open-elements-react` 0.39.0

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

### PR #41 — Box Open Elements 0.37.0 (merged)

Both packages upgrade from 0.33.0 to 0.37.0 with no other lockfile resolution
changes. Published APIs and generated declarations remain compatible. The optional
new component features do not require Dispatch application changes. Embedded assets
and the adoption notes are refreshed. PR #41 was squash-merged as
`27936b11a21b34685e1c49f097b75dbd551d140e` with specific user approval.

### PR #42 — accessibility cleanup and BOE 0.38.0 (merged)

Kyle approved PR #42, squash-merged as
`331273e727218be46bfb76f54ea9bd80e2dc27ca`. Its post-merge checks passed with
79 unit tests and five E2E workflows; local/remote main and the reviewed tree matched.
The historical cleanup allocation began October 11 at **02:34:00 UTC**.
The branch adopts verified published core/React 0.38.0 from upstream commit
`4e7da132911563e1cb3f80720134cb16b8aa1755`; both downloaded tarballs match
the registry SHA-512 integrity and the React core peer range is compatible.
It fixes Dispatch-owned accessibility through existing DOM and theme contracts:

- the brand name includes its visible B/ glyph, Dispatch and Solution delivery copy,
  with explicit text-node separators; the optional sidebar toggle control IDREF is
  omitted because its shadow button cannot resolve the light-DOM target;
- lifecycle overflow belongs to a named keyboard-focusable region with visible focus,
  preserving the read-only Path component and existing stages;
- historical deployment actions have a named group role;
- affected muted text and the live activity feed use existing theme tokens instead
  of hardcoded light colors; charcoal/mint fills and registered themes are unchanged;
- focused unit tests and scoped axe/keyboard checks are added to the existing E2E
  workflows. `axe-core` 4.14.0 is a test-only dependency. Scoped audits attach
  violations, incomplete checks and contrast passes; the visible owned text contexts
  require at least one resolved contrast pass and no incomplete contrast result.
  Other incomplete categories remain evidence for assessment, not blanket failures.

Fact-list styles remain unchanged: their earlier contrast findings disappeared once
entrance animations settled. No BOE source, native control names or status color-mix
internals are patched. Published 0.37.2 repairs the original bright Badge/status
foregrounds and Resource Row/Diff Viewer native names. Scoped real-app checks cover
those contracts in both themes, retaining evaluated contrast passes and incompletes.
Path checks cover all stage labels after theme fills settle; Run Trace checks its
status summary.
Full-page evidence retains other content and all stages rather than treating these
scoped checks as global conformance.

An early light Path scan sampled the theme-transition fill #e9e9e9 at 4.13:1.
Settled screenshot pixels verify #6f6f6f on #f4f4f4 at 4.57:1; the regression
waits for the native fill before auditing all stage labels. This is not an open gap.
Published 0.38.0 resolves the confirmed numeric-gutter contrast failure with
opaque secondary text. The real diff drawer now checks its native line-number cells
in both themes, independently of the pre-existing changed-code overlap incompletes.
It also resolves Switch public-description association: native AX descriptions are
Required/Optional, while Included/Not included names, checked state, required disabled
state and optional focusability are preserved before/after toggling. The earlier
2.92:1 gutter measurement and absent Switch descriptions remain historical evidence,
not current open gaps. No local override or rule suppression is introduced.
Non-text hidden
Path markers and Run Trace overlap incompletes are also retained by full-page scans;
they are not waived through the strict owned-text helper.

The final cleanup/adoption passes 79 unit tests, all five updated mock E2E
workflows, frontend lint/build, frozen installation, and the full Go gate. Final
attachments retain 18 scoped audits with zero violations/incompletes and nine
resolved contrast contexts, plus explicit numeric-gutter measurements. Gutter
computed colors and screenshot pixels measure 5.02:1 light and 9.68:1 dark.
Two focused read-only probes retain native Switch AX before/after toggling and
rendered gutter screenshots. The isolated six-check full-page replay retains 41
matched states with no page overflow. It reports six qualified native-Drawer scrim
contrast nodes, zero name-mismatch violations, 453 contrast and four name-rule
incomplete node occurrences (repeated states included). The four name incompletes
are Switch valueText heuristics; native AX now independently confirms the correct
primary names, Required/Optional descriptions and checked/disabled states.

The original sidebar IDREF, brand name, history ARIA, mobile scroll focus and owned
body-copy findings are cleared on audited normal surfaces. The Drawer scan derives
scrim gray for headings/copy and a tile legend despite the painted white inner
panel. Outer dialog is rgba(0,0,0,.75); panel and screenshot background are white.
Actual #222 headings and #6f6f6f copy measure 15.91:1 and 5.02:1. That specific
background-resolution qualification does not waive unrelated findings or claim
formal conformance. Other gradient/image, hidden-glyph and overlap incompletes
retain the bounded computed/pixel classifications from the earlier assessment.

The published 0.37.1 table fix is adopted in lockstep. The real Dispatch workflow
checks four successive Enter/Space sorts with focus set only once, in light/dark
at 1440px/390px, then opens historical details and returns to History. Static
tables omit selection ARIA; the controlled file selector has grid semantics and
one selected row. Embedded assets are regenerated. The 0.37.2 status/name adoption
was verified separately from the qualified residuals. The PR #42 merge approval
was specific; the current 0.39.0 draft needs new merge approval and no deployment
is authorized.

### Current branch — BOE 0.39.0 upgrade

Published core/React 0.39.0 are adopted together from upstream release
`5732d0098eccf594b924fdade409e58bda285ed5`. Registry metadata, downloaded
SHA-512 integrity and the React core peer range are independently verified. Only
those two dependency resolutions change; no application or theme compatibility
change is needed. Embedded assets are regenerated and remain byte-identical.

Runtime artifact changes from 0.38.0 are Box Plot and Trace Waterfall enhancements.
Dispatch consumes neither element and has no chart series or chart UI; Overview
and History use Metric Card value/message properties. Its metrics and existing
workflow are verified alongside desktop/mobile light/dark sorting, gutter contrast
and Switch descriptions. The original accessibility fixes and qualifications remain.

The fixed effort estimate for this requested continuation is 20 minutes of active
work beginning 13:16:08 UTC, excluding later external review/approval waits. Prepare
an independent-review draft; prior merge approvals do not apply to this upgrade.

## Box Open Elements boundary

Use [`docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md`](docs/BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md)
as the migration authority and
[`docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md`](docs/BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md)
as the adopted/retained/gap inventory.

All planned migration phases are complete within published 0.39.0 contracts. Keep the
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

- **Current Status:** PR #42 is merged on main. `codex/boe-0-39-upgrade` adopts
  verified published core/React 0.39.0; the rebuilt embedded assets are unchanged.
- **Recommended Next Step:** Independently review the 0.39.0 draft and obtain new
  exact-head merge approval. Deployment remains separately unapproved.
- **Remaining qualifications:** Native Drawer scrim reports retain the verified
  white-panel interpretation. Sidebar/select/visible-overlap samples pass; hidden
  Path glyphs and modal-covered content are separately classified. Pre-existing
  Switch invalid-error stress contrast remains upstream baseline evidence, outside
  this normal-workflow adoption; it is not a new regression or a global-clean claim.
  Bundle warning remains. Merge/deploy require approval.
