# Box Open Elements migration plan

## Goal

Replace Dispatch-owned UI primitives with current Box Open Elements components
without moving Dispatch-specific workflow, provider, routing, or data logic into the
design system.

This plan is based on:

- the Dispatch React implementation on `codex/history-deployment-summaries`;
- the original `@unofficialbox/box-open-elements` and React adapter baseline at
  `0.12.0`; and
- the Box Open Elements packages at version `0.28.4`, reviewed on October 9, 2026.

Phase 0 upgraded both packages to 0.27.0. The 0.28.1 follow-up upgrades core and
React packages in lockstep and adopts the component contracts released in 0.28.0.
The 0.28.4 maintenance update keeps both packages aligned; releases 0.28.2 through
0.28.4 focus on Verdict Banner, Process Modeler, and Code Editor behavior and do
not change Dispatch's adopted contracts.

## Implementation status

- **Phase 0 complete:** core and React adapter upgraded together to 0.27.0; lockfile,
  embedded browser assets, frontend tests, end-to-end tests, and the full Go gate are
  current.
- **0.28.1 follow-up complete:** confirmation dialogs, rich solution/default tiles,
  drawer close-affordance control, the collapsed sidebar toggle, and shell navigation
  landmarks now use the published APIs instead of Dispatch workarounds.
- **0.28.4 maintenance update complete:** core and React packages remain in lockstep;
  the intervening release contracts require no Dispatch code changes.
- **October 9 audit complete:** the deployment header uses `box-path`; Connect,
  Configure, and saved-connection selectors use `box-resource-row`; connection-mode
  choices use `box-tile-group`; and the sidebar uses the published header/body/footer
  composition with expanded route identity by default.
- **Phase 1 complete within the published component contracts:** fact lists,
  strategy tiles, history filters, React field adapters, alerts, spinners, status
  badges, and toast integration are migrated and verified.
- **Phase 2 complete:** the application shell, sidebar, breadcrumb, and workflow
  navigation use the published components. Version 0.28 resolves the former
  collapsed-toggle and nested-navigation constraints.
- **Phases 3 and 4 complete:** deployment data, validation-file selection, live
  activity, metrics, provider summaries, and completed-run composition now use
  the published table, timeline, metric, card, section, and fact-list contracts.
- The read-only connection summary rows remain Dispatch composition. Selectable
  provider and saved-connection rows now use Resource Row.

## Decision rules

1. Prefer a Box Open Elements component when it owns the same semantics and
   interaction contract, not merely a similar appearance.
2. Keep semantic HTML for page structure when no reusable interaction is involved.
   A `<section>`, heading, or application-specific list is not automatically a
   custom component that needs replacing.
3. Keep Dispatch responsible for routing, API calls, provider URLs, workflow state,
   and domain-specific copy.
4. Use the React adapter when it provides typed property and event bridging. Use a
   native custom element for the remaining published components instead of creating
   another local wrapper layer.
5. Do not combine nested interactive controls inside a button or listbox option.
6. Remove local CSS only after the replacement passes interaction, responsive, and
   accessibility checks.

## Current inventory and target mapping

### Application shell and workflow navigation

| Current Dispatch surface | Box Open Elements target | Decision |
| --- | --- | --- |
| `Sidebar` and the outer `.app-shell` layout | `box-app-shell`, `box-nav-sidebar`, and `box-sidebar-toggle-button` | Replaced. Keep hash routing and active-route state in Dispatch; render route links in the sidebar slots. |
| `RailIcon` | Nav-sidebar icon slots plus Box Open Elements iconography | Reduce, then retain only the small icon lookup if the sidebar still needs it. Do not duplicate icon SVGs. |
| `DeploymentHeader` breadcrumbs | `box-breadcrumb` | Replace the custom separator and link styling. Dispatch continues to provide the route targets. |
| Deployment lifecycle indicator | `box-path` | Replaced. The header is a read-only record lifecycle; Back/Continue buttons own task navigation and eligibility. |
| History back control | `box-link-button` or the page breadcrumb | Replace the custom text-button styling while preserving navigation semantics. |

### Forms and selection

| Current Dispatch surface | Box Open Elements target | Decision |
| --- | --- | --- |
| Solution cards in `ChoosePage` | `box-tile-group` | Replaced in 0.28.1 using published per-option description and metadata. Dispatch retains the solution data and selected value. |
| Strategy buttons in `ConfigurePage` and `SettingsPage` | `box-tile-group` | Replaced in Phase 1 with a shared event-integration component and native radio semantics. |
| Default-solution rows in `SettingsPage` | `box-tile-group` | Replaced in 0.28.1 using published status and disabled-reason fields, preserving availability context. |
| Native history search input | `box-search-field` | Replaced in Phase 1 and bound to `value-changed`; the component owns its clear behavior. |
| Native history selects | React `Select` adapter / `box-select` | Replaced in Phase 1. The adapter owns property synchronization and native custom-event binding. |
| History filter layout and clear behavior | `box-filter-bar` only if its query, sort, view, and chip model fits | Prototype rather than force. The current filters are four independent fields; composing `box-search-field`, `box-select`, and `box-button` is acceptable if `box-filter-bar` would distort the model. |
| Existing switches, text fields, and selects in connection drawers | Current `box-switch`, React `TextField`, and React `Select` adapters | Retain the published controls. The deployment-name and drawer value fields moved to React adapters in Phase 1; switches remain native until an adapter is published. |

### Feedback, summaries, and disclosure

| Current Dispatch surface | Box Open Elements target | Decision |
| --- | --- | --- |
| `DeploymentConfirmationDialog` | React `Dialog` adapter / `box-dialog` | Replaced in 0.28.1. The published disabled/busy confirm contract keeps deployment unavailable while Salesforce package preparation is active. |
| Inline connection and defaults errors | React `Alert` adapter / `box-alert` | Replaced in Phase 1 while preserving the app-owned error messages. |
| Custom status pills in `DeploymentHeader` and `ReviewPage` | `box-badge` | Replaced in Phase 1 with explicit success, error, and in-progress tones. |
| Loading and unavailable history detail states | `box-spinner` and React `Alert` adapter | Replaced in Phase 1 with distinct status and alert semantics. Empty-state migration remains scoped to later data-surface work. |
| Overview summary metrics | `box-metric-card` | Replace the custom metric strip one metric at a time. Use the card action event only where the metric is genuinely actionable. |
| `DetailsRail` / `DetailList` | `box-fact-list` | The hand-built definition list was replaced in Phase 1 while retaining the app-specific aside and card placement. |
| Provider count lists in deployment history | `box-fact-list` | Replaced in Phase 4. The published fact list preserves the recorded labels and values while provider cards retain the comparison grouping. |
| Native diagnostic `<details>` | `box-accordion` | Replace for the single technical-detail disclosure. Issue #313 already tracks richer multi-open/live-summary behavior; do not create a duplicate. |
| `AppToast` integration wrapper | React `Toast` adapter | Migrated in Phase 1. Retain only the small app notice model and the top-layer placement integration. |
| Connection drawer close affordance | `box-drawer` `hide-close-button` | Adopted in 0.28.1; the temporary `::part(close)` override is removed. |

### Tables, lists, and run output

| Current Dispatch surface | Box Open Elements target | Decision |
| --- | --- | --- |
| `DeploymentHistoryTable` on Overview and History | `box-table` | Replace. Use escaped text cells, badge cells, and a link descriptor with `#history/{id}` for SPA navigation. Keep the hash-state synchronization in Dispatch. |
| Deployment component details table | `box-table` | Replace directly with System, Component, and Result columns and an explicit empty state. |
| File selector in `ValidationChangesDrawer` | Selectable `box-table` | Replaced. The table's controlled single-selection contract preserves current-row state and keyboard selection; the current `box-document-list` does not expose controlled selected/current state. |
| `LiveActivityFeed` | `box-timeline` | Replaced. Dispatch retains the 12-event live-tail policy and a keyboard-focusable bounded scroll host; use `box-audit-log` only for a full searchable/exportable audit view, not this compact feed. |
| `RunTimeline` | Existing `box-run-trace` | Retain. The mapping in `runTimelineModel` is application logic and remains. The 0.28.4 built-in running marker, visible status label, and summary are sufficient; the former shadow-root animation injection was removed. |
| Summary/provider result compositions | `box-card`, `box-fact-list`, `box-result-blocks`, and existing buttons/badges | Compose from primitives. Do not force `box-run-summary` unless its run/todo/step model matches the Dispatch record. |
| External destination links in Summary | `box-link-button` | Replace the locally styled anchor when it is a navigation action. Keep provider launch URL validation in Dispatch. |

### Connection and provider rows

The following rows combine selection, multiline identity, readiness state, and an
independent action:

- provider cards in `ConnectPage` and `ConfigurePage`;
- saved connections in `Drawers`;
- connection rows in `ProviderConnectionPanel`; and
- connection health panels on Overview.

`box-datalist-item` owns selectable-option behavior, but 0.27 has no separate
trailing status/action region. Placing **Open**, **Remove**, or **Configure** inside
the option activation target would create ambiguous nested interaction.

Issue [box-open-elements #354](https://github.com/unofficialbox/box-open-elements/issues/354)
delivered Resource Row. Its selectable resource model now owns the Connect,
Configure, and saved-connection lists, while the current non-selectable summary
rows remain application composition.

For the current summary rows:

- keep one shared Dispatch connection-row composition;
- use `box-card`, `box-badge`, `box-button` / `box-icon-button`, and provider logos
  inside it;
- preserve separate focus targets for selection and secondary actions; and
- do not force selection semantics into a read-only status summary.

Issues #355, #357, #358, #359, and #360 are resolved and adopted in the 0.28.1
upgrade. No local CSS or behavior workaround remains for those contracts.

## Components that should remain Dispatch-owned

- `ProviderLogo`, because Box and Salesforce branding and fallbacks are
  product-specific assets.
- `runTimelineModel` and `liveActivityModel`, because they translate Dispatch API
  records into presentation models.
- Hash routing and deployment resume logic in `App.tsx`.
- Safe provider destination validation and the decision to open Box or Salesforce.
- Page-level composition, headings, domain copy, deployment names, and provider
  terminology.
- The state and data-loading controller for the validation/deployment diff drawer.
- Semantic page sections and lists that carry no reusable interaction contract.

## Phased implementation

### Phase 0 — upgrade and establish the integration boundary

Status: **Complete**

1. Upgrade both Box Open Elements packages together from 0.12.0 to 0.27.0.
2. Refresh the lockfile and import only the new entries used by the next phase.
3. Review 0.27 migration notes and source contracts for every currently used
   element, especially custom event names, boolean properties, drawer top-layer
   behavior, and token changes.
4. Replace local JSX declarations with package types where available. Keep
   `boe.d.ts` only for uncovered native elements.
5. Run the complete existing test suite and a browser smoke pass before changing
   markup. This isolates upgrade regressions from migration regressions.
6. Reconcile `BOX_OPEN_ELEMENTS_COMPONENT_GAPS.md` against the new installed
   version; it is historical intake, not the current migration authority.

Exit gate: the existing application behaves the same on 0.27.0.

### Phase 1 — low-risk primitives

Status: **Complete within the current component contracts**

1. Replace `DeploymentConfirmationDialog` with the React adapter. **Complete in 0.28.1.**
2. Replace inline errors and history loading/failure states. **Complete.**
3. Replace `DetailList` with `box-fact-list`; retain the intentionally horizontal
   provider statistics. **Complete.**
4. Replace strategy and rich solution choices with `box-tile-group`.
   **Complete in 0.28.1, including metadata, status, and disabled reasons.**
5. Replace history search/select controls. **Complete.**
6. Move the deployment-name and drawer value fields plus toast event/property
   synchronization to React adapters. **Complete.**

Connection drawers now use `hide-close-button`; no shadow-DOM query or part-hiding
bridge is required. The published dialog retains modal focus containment, Escape
cancellation, focus restoration, and a disabled/busy confirmation action. These
behaviors remain covered by the mock end-to-end suite.

Exit gate: repeated fact-list CSS, custom radio-card behavior, and the custom modal
implementation are removed.

### Phase 2 — shell and navigation

Status: **Complete within the current component contracts**

1. Introduce `box-app-shell` around the current application content. **Complete.**
2. Move the current rail into `box-nav-sidebar` without changing routes. **Complete.**
3. Add `box-sidebar-toggle-button` and verify collapsed accessible names.
   **Complete.** In 0.28.1 the toggle uses the header slot and stays reachable when
   collapsed; App Shell yields the navigation landmark to the slotted sidebar.
4. Replace breadcrumbs with `box-breadcrumb`. **Complete.**
5. Replace the workflow header indicator with `box-path`.
   **Complete.** The header reports read-only lifecycle state; explicit page actions
   retain task navigation.

Exit gate: mouse, keyboard, browser Back/Forward, reload-on-deep-link, and narrow
viewport navigation all preserve current behavior.

Verification completed with component tests, the mock end-to-end suite, a 390px
viewport pass, and live Chrome inspection of collapse/expand, visible state, the
accessibility tree, and Arrow-key workflow navigation.

### Phase 3 — data surfaces

Status: **Complete**

1. Replace Overview and History deployment tables with one `box-table` adapter.
   **Complete.** The shared adapter owns property/event synchronization; row links
   use safe hash destinations and remain browser-native links.
2. Replace the deployment component table with `box-table`. **Complete.**
3. Replace the diff file selector with a library collection primitive.
   **Complete with `box-table`.** Controlled single selection, repeat selection,
   replacement file sets, and empty/error copy are covered by tests.
4. Prototype `box-timeline` for the compact live activity feed. **Complete.** The
   host remains keyboard focusable because it owns the bounded scroll region.
5. Adopt Resource Row for selectable provider and saved-connection rows while
   retaining one read-only summary-row composition. **Complete.**

Exit gate: sorting/filtering, row links, result badges, empty states, and selected
file behavior pass unit and end-to-end tests.

### Phase 4 — page compositions and CSS retirement

Status: **Complete within the current component contracts**

1. Replace Overview metrics with `box-metric-card`. **Complete.** History summary
   metrics use the same primitive.
2. Simplify summary/provider cards with `box-card`, `box-section`, and
   `box-fact-list`. **Complete.** Summary destinations and history return
   navigation use `box-link-button`; technical diagnostics use `box-accordion`.
3. Remove selectors in `App.css` that no longer target migrated table, file-list,
   activity-feed, metric, summary, and provider-summary markup. **Complete for the
   migrated surfaces.** The former drawer-close bridge was removed in 0.28.1.
4. Consolidate the remaining CSS into page layout and Dispatch-specific composition
   rules; use only published parts for component-level treatment. **Complete for the
   Phase 3–4 surfaces.**
5. Update the component-gap document with the final adopted/retained inventory.
   **Complete.** No new Phase 3–4 upstream gap was found.

Exit gate: no local visual primitive duplicates a published Box Open Elements
component, and remaining custom UI is documented as application composition or an
upstream gap.

## Verification matrix

Run the narrowest relevant tests during each replacement, then the full web gate:

```bash
cd web
bun run test
bun run lint
bun run build
bun run test:e2e
```

For each migrated interaction, verify:

- keyboard-only operation, visible focus, Escape behavior, and focus restoration;
- accessible names, current/selected state, live announcements, and error text;
- pointer activation does not double-fire through React and custom-element events;
- Chromium at desktop width and at 390px, including dialog, drawer, and popover
  top-layer behavior;
- Overview, workflow, History list, historical detail, Settings, validation diff,
  failure recovery, and completed summary routes;
- browser Back/Forward and direct hash reloads; and
- light/dark token behavior if Dispatch exposes or inherits both themes.

Retain before/after screenshots for the shell, workflow header, connection drawer,
history table/detail, validation diff, and completed summary. Browser acceptance
uses Chromium at those two viewport sizes; keyboard, focus, semantic accessibility,
and automated checks remain in scope.

## Delivery order and pull-request boundaries

Keep each phase reviewable:

1. dependency upgrade and type/event compatibility;
2. dialog, feedback, facts, and selection controls;
3. shell, sidebar, breadcrumbs, and workflow steps;
4. tables, diff file list, and live activity;
5. page summaries, CSS cleanup, and documentation.

Do not mix data-model or backend changes into these pull requests. If a library
component cannot preserve the current product behavior, stop that individual
replacement, record the contract mismatch, and either compose existing primitives
or open a focused upstream enhancement before continuing.
