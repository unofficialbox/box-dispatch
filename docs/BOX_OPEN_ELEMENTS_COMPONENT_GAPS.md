# Box Open Elements component adoption, gaps, and enhancements

> Historical intake note: this document records earlier component decisions made
> against older package versions. The current version-by-version replacement plan is
> [BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md](./BOX_OPEN_ELEMENTS_MIGRATION_PLAN.md).
> Reconcile this inventory during migration Phase 0 before treating an item below as
> a current library limitation.

## Purpose

Dispatch uses Box Open Elements for shared foundations and reusable controls.
This document separates four different kinds of roadmap input:

- capability available in the published package and ready to adopt;
- a composition recipe that applications should assemble from existing primitives;
- an accepted enhancement to an existing primitive;
- a genuinely missing reusable component or pattern.

This avoids treating every Dispatch screen composition as a missing component.

## Catalog categories

The classifications match the
[Box Open Elements catalog](https://unofficialbox.github.io/box-open-elements/):

- **Foundations** — tokens, theming, geometry, motion, icons, accessibility, and brand;
- **Components** — individual framework-agnostic custom elements;
- **Patterns** — composed views built from foundations and components.

## Verification baseline

The current Dispatch web package uses `@unofficialbox/box-open-elements` 0.28.6.
For each evaluated element, intake must inspect the published element's
`observedAttributes`, public properties, methods, and emitted events rather than
inferring capability from a screenshot or tag name. Upstream and maintainer-reported
work must remain distinct from functionality available in the installed package.

Versions 0.28.0 and 0.28.1 resolve the dialog, drawer, solution-tile, sidebar-toggle,
and application-shell issues previously tracked here. Dispatch adopted those APIs
and removed the matching local substitutes. The October 9 follow-up also adopted
Resource Row for selectable provider and saved-connection rows, Path for the
read-only deployment lifecycle, and Tile Group for connection-mode choices. The
read-only connection summaries remain application composition because they do not
offer a selection target.

## Adopted in Dispatch

| Category | Box Open Elements capability | Dispatch use |
| --- | --- | --- |
| Foundations | Design tokens and `boxIconography` | Color, typography, spacing, focus treatment, and navigation icons. |
| Components | `box-button` | Primary, secondary, and destructive workflow actions. |
| Components | `box-card` | Summary and detail surfaces. |
| Components | `box-switch` | Provider and component enablement. |
| Components | `box-badge` | Live and verified state labels. |
| Components | `box-status-icon` | Shared execution and outcome glyphs in review and completed-deployment summaries. |
| Components | `box-progress-bar` | Connection and run progress. |
| Components | `box-spinner` | Loading states for historical deployment data and other asynchronous surfaces. |
| Components | `box-metric-card` | Overview summary metrics and readiness state. |
| Components | `box-drawer` | Connection editing and run diagnostics, including large sizing, busy state, sticky footer actions, focus management, Escape/backdrop dismissal, and controlled open state. |
| Components | React `Dialog` adapter / `box-dialog` | Deployment confirmation, including disabled and busy confirmation while Salesforce package preparation is active. |
| Components | `box-text-field` and `box-select` | Box credentials and authenticated Salesforce-org selection, including autocomplete, password reveal, loading, and empty-state support. |
| Components | `box-tile-group` | Deployment strategy, solution selection, Settings defaults, and Box/Salesforce connection-mode choices with per-option metadata, availability, and status. |
| Components | `box-split-view` | Connect and Configure master-detail page structure. |
| Components | `box-app-shell`, `box-nav-sidebar`, and `box-sidebar-toggle-button` | Application landmarks, responsive shell structure, compact route navigation, and explicit expand/collapse state. |
| Components | `box-breadcrumb` | Deployment location and workspace return navigation. |
| Components | `box-path` | Read-only deployment lifecycle in the record header, including complete, current, upcoming, and failed states. |
| Components | `box-resource-row` | Selectable provider rows and saved Box/Salesforce connections with independent status and secondary-action slots. |
| Components | `box-table` | Sortable deployment history, deployed-component details, and controlled validation-file selection with loading, empty, error, badge, link, and selected-row states. |
| Components | `box-section` | Provider-summary, deployment-detail, and completed-run destination section structure. |
| Components | `box-link-button` | Historical-deployment return navigation and completed-run external destination actions. |
| Components | `box-accordion` | The single technical-detail disclosure in run diagnostics. |
| Patterns | `box-timeline` | The compact 12-event validation/deployment activity tail. |
| Patterns | `box-run-trace` | Live provider and component validation/deployment activity. Its public running state supplies the marker, status text, summary, token colors, and reduced-motion behavior; Dispatch does not patch the component shadow root. |

The published React adapter now provides wrappers for buttons, dialogs, selects,
comboboxes, text and number fields, checkboxes, tabs, cards, alerts, toasts, drawers,
and code blocks, where typed property and event bridging is useful. Components
without an adapter wrapper remain supported native custom elements in React 19;
Dispatch keeps a narrow JSX declaration boundary for those elements.

## Available components that are not gaps

| Category | Component | Verified contract | Dispatch decision |
| --- | --- | --- | --- |
| Components | `box-progress-steps` | Published 0.27.0 provides controlled navigation and per-step complete, current, pending, blocked, failed, and disabled states, keyboard navigation, public parts, and `value-changed`. | Available for editable setup tasks. Dispatch now uses the read-only `box-path` because workflow navigation remains in the Back/Continue controls and the header only reports lifecycle position. |
| Components | `box-table` | Escaped text, badge, and link cells plus expansion, loading, empty, error, sorting, and selection are supported. | Adopted for all current Dispatch data surfaces. Deployment history intentionally uses concise provider names rather than logos, so its cells fit the safe published descriptors. |
| Components | `box-drawer` | Controlled state, focus behavior, sticky footer, size presets, busy state, mobile presentation, cancelable dismissal, and `hide-close-button` are present. | Adopted for connection and diagnostics workflows. Connection drawers use the public close-affordance API instead of a CSS part override. |
| Components | `box-text-field` | Shared field contract, autocomplete, password reveal, loading, and valid states are present. | Adopted for connection forms. |
| Components | `box-select` | Shared field contract, loading and empty states, options, single/multiple values, and `value-changed` are present. | Adopted for authenticated-org and subject selection. |
| Components | `box-split-view` | Controlled ratio, optional resizing, and `ratio-changed` are present. | Adopted as the Connect and Configure layout primitive. Selection and detail behavior remain application composition. |
| Components | `box-nav-sidebar` | Structured navigation items, collapsible behavior, slots, responsive parts, accessible collapsed-row naming, and a reachable collapsed header toggle are present. | Adopted with application-owned hash routing and active state. The toggle now uses the documented header slot. |
| Components | `box-resource-row` | Selectable resource identity, metadata, status, and secondary-action regions are published. | Adopted for Connect, Configure, and saved-connection lists. Retained `ProviderConnectionRow` is intentionally read-only and therefore does not take Resource Row selection semantics. |
| Components | `box-path` | Read-only horizontal lifecycle stages with complete/current/upcoming/error state and responsive chevron/base variants are published. | Adopted in the deployment header. Back/Continue actions remain the explicit workflow controls. |
| Components | `box-progress-ring` | A minimum 48px ring that includes a percentage and label. | Use only when showing aggregate progress. It is intentionally not used as a per-row activity marker because repeated 0%-100% rings make the live log harder to scan. |

## Composition recipes needed

These are documentation and composition needs, not requests to add product-specific
state to a low-level element.

| Category | Composition | Existing primitives | Recommended recipe |
| --- | --- | --- | --- |
| Patterns | Selectable master-detail workspace | `box-split-view`, `box-table`, `box-empty-state`, `box-skeleton`, `box-drawer` | Document controlled row/card selection, keyboard behavior, empty/loading detail states, and narrow-screen detail presentation. Dispatch owns selected provider/component state. |
| Patterns | Compact application navigation | Light-DOM `<a aria-current>`, `box-badge`, icons, optional drawer | Document expanded and collapsed navigation with accessible labels, tooltips, badges, link semantics, and responsive drawer behavior. Dispatch owns routes and active state. |

## Remaining enhancements after the 0.28.6 review

| Priority | Category | Component or area | Accepted need | Dispatch action until release |
| --- | --- | --- | --- | --- |
| P1 | Foundations | TypeScript custom-element declarations | JSX tag maps and typed event maps for React and TypeScript consumers. Tracked in [box-open-elements #379](https://github.com/unofficialbox/box-open-elements/issues/379). | Maintain the narrow local `boe.d.ts` boundary and native event listeners. Remove redundant declarations after adoption. |
| P2 | Patterns | `box-timeline` live-tail composition | A bounded, keyboard-focusable scroll host remains application composition; the component owns semantic events while Dispatch owns the 12-event policy and auto-follow behavior. | Keep the small focusable host rule around `box-timeline`; no upstream enhancement is required by the current contract. |
| P2 | Foundations | `box-run-trace` marker geometry tokens | The pattern's connector position is coupled to its internal step padding. Public marker/connector inset tokens, or an invariant that part-level step padding preserves alignment, would let consuming products tune density without breaking the trace geometry. Tracked in [box-open-elements #380](https://github.com/unofficialbox/box-open-elements/issues/380). | Dispatch currently overrides the published `step` and `marker` parts together with the step connector pseudo-element in `App.css`. The mock end-to-end suite checks marker centres and connector edge alignment. Recheck these coupled overrides on every package upgrade until geometry tokens are published. |
| P2 | Patterns | Master-detail and compact-navigation recipes | Official examples covering the compositions above. | Keep Dispatch compositions small and documented. |

Accessibility corrections for `box-dropdown`, `box-selectable-card`, and
`box-action-bar` are reported complete by the maintainer. Dispatch should consume
them through the next containing package release rather than applying local forks.

## Resolved component gap

### Live run trace

**Category:** Patterns
**Roadmap status:** Published in 0.6.0 as `box-run-trace`

Validation and deployment need aligned event nodes, connectors, timestamps,
expandable details, nested progress rows, and live success/warning/failure state.
Version 0.6.0 now provides the operational model through `box-run-trace`.

The accepted pattern should support:

- event status, timestamp, title, description, and expandable detail content;
- nested task/progress content with live updates;
- stable node and connector alignment at all viewport widths;
- accessible live-state announcements and failure recovery context.

Dispatch migrated its provider/component progress adapter to `box-run-trace` and
removed the custom timeline DOM and styling. The application still owns the mapping
from Dispatch server events to the reusable run-step data model.

## React adapter boundary

**Category:** Components

Dispatch should use `@unofficialbox/box-open-elements-react` for the published
adapter surface when a component needs property synchronization or direct event
binding. It must not create its own wrappers for components that the adapter does
not yet expose. Native custom elements are the supported React integration for
those remaining components, including `box-spinner`, `box-badge`,
`box-progress-bar`, and `box-run-trace`.

## Dispatch adoption sequence

1. Use the published drawer, field, select, split-view, metric-card, table, timeline, run-trace, section, card, fact-list, and progress APIs where their verified contracts fit.
2. Use `box-path` for deployment lifecycle state; keep workflow navigation in explicit Back/Continue actions.
3. Replace local JSX declarations with official React JSX tag maps if they are published.
4. Use the published sidebar header-toggle and single-navigation-landmark composition; do not restore the former body-slot or nested-landmark workarounds.
5. Re-run this source-level intake on each Box Open Elements upgrade and update this document with published-version evidence.

## Adoption rule

Before creating a Dispatch control, inspect the published Box Open Elements source
and catalog. Prefer an existing foundation or component. Record a **composition
recipe needed** when existing primitives cover the behavior but guidance is missing.
Record an **enhancement** when the correct primitive exists but a broadly useful API
is absent. Record a **gap** only when no existing primitive provides the right
foundation and the missing behavior is reusable beyond Dispatch.
