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

The current Dispatch web package uses `@unofficialbox/box-open-elements` 0.27.0.
For each evaluated element, intake must inspect the published element's
`observedAttributes`, public properties, methods, and emitted events rather than
inferring capability from a screenshot or tag name. Upstream and maintainer-reported
work must remain distinct from functionality available in the installed package.

Version 0.6.0 contains several items that were previously tracked as accepted gaps or
enhancements. Dispatch adopted those APIs in the same upgrade and removed the local
substitutes where the published contract matched the approved workflow.

## Adopted in Dispatch

| Category | Box Open Elements capability | Dispatch use |
| --- | --- | --- |
| Foundations | Design tokens and `boxIconography` | Color, typography, spacing, focus treatment, and navigation icons. |
| Components | `box-button` | Primary, secondary, and destructive workflow actions. |
| Components | `box-card` | Summary, selectable, configuration, and detail surfaces. |
| Components | `box-switch` | Provider and component enablement. |
| Components | `box-badge` | Live and verified state labels. |
| Components | `box-progress-bar` | Connection and run progress. |
| Components | `box-spinner` | Loading states for historical deployment data and other asynchronous surfaces. |
| Components | `box-metric-card` | Overview summary metrics and readiness state. |
| Components | `box-drawer` | Connection editing and run diagnostics, including large sizing, busy state, sticky footer actions, focus management, Escape/backdrop dismissal, and controlled open state. |
| Components | `box-text-field` and `box-select` | Box credentials and authenticated Salesforce-org selection, including autocomplete, password reveal, loading, and empty-state support. |
| Components | `box-split-view` | Connect and Configure master-detail page structure. |
| Components | `box-app-shell`, `box-nav-sidebar`, and `box-sidebar-toggle-button` | Application landmarks, responsive shell structure, compact route navigation, and explicit expand/collapse state. |
| Components | `box-breadcrumb` | Deployment location and workspace return navigation. |
| Components | `box-progress-steps` | Eligibility-gated workflow navigation with complete, current, failed, and unavailable states plus keyboard interaction. |
| Components | `box-table` | Sortable deployment history, deployed-component details, and controlled validation-file selection with loading, empty, error, badge, link, and selected-row states. |
| Components | `box-section` | Provider-summary, deployment-detail, and completed-run destination section structure. |
| Components | `box-link-button` | Historical-deployment return navigation and completed-run external destination actions. |
| Components | `box-accordion` | The single technical-detail disclosure in run diagnostics. |
| Patterns | `box-timeline` | The compact 12-event validation/deployment activity tail. |
| Patterns | `box-run-trace` | Live provider and component validation/deployment activity. |

The published React adapter now provides wrappers for buttons, dialogs, selects,
comboboxes, text and number fields, checkboxes, tabs, cards, alerts, toasts, drawers,
and code blocks, where typed property and event bridging is useful. Components
without an adapter wrapper remain supported native custom elements in React 19;
Dispatch keeps a narrow JSX declaration boundary for those elements.

## Available components that are not gaps

| Category | Component | Verified contract | Dispatch decision |
| --- | --- | --- | --- |
| Components | `box-progress-steps` | Published 0.27.0 provides controlled navigation and per-step complete, current, pending, blocked, failed, and disabled states, keyboard navigation, public parts, and `value-changed`. | Adopted. Dispatch maps workflow eligibility into the published states and uses public parts to present the element as the approved horizontal workflow. |
| Components | `box-table` | Escaped text, badge, and link cells plus expansion, loading, empty, error, sorting, and selection are supported. | Adopted for all current Dispatch data surfaces. Deployment history intentionally uses concise provider names rather than logos, so its cells fit the safe published descriptors. |
| Components | `box-drawer` | Controlled state, focus behavior, sticky footer, size presets, busy state, mobile presentation, and cancelable dismissal are present. | Adopted for connection and diagnostics workflows. |
| Components | `box-text-field` | Shared field contract, autocomplete, password reveal, loading, and valid states are present. | Adopted for connection forms. |
| Components | `box-select` | Shared field contract, loading and empty states, options, single/multiple values, and `value-changed` are present. | Adopted for authenticated-org and subject selection. |
| Components | `box-split-view` | Controlled ratio, optional resizing, and `ratio-changed` are present. | Adopted as the Connect and Configure layout primitive. Selection and detail behavior remain application composition. |
| Components | `box-nav-sidebar` | Structured navigation items, collapsible behavior, slots, responsive parts, and accessible collapsed-row naming are present. | Adopted with application-owned hash routing and active state. The toggle stays in the always-visible body until issue #359 resolves the collapsed-header recipe. |
| Components | `box-stage-path` | Read-only horizontal lifecycle steps are published. | Do not use for editable wizard navigation because it does not expose eligibility-gated step activation. |
| Components | `box-progress-ring` | A minimum 48px ring that includes a percentage and label. | Use only when showing aggregate progress. It is intentionally not used as a per-row activity marker because repeated 0%-100% rings make the live log harder to scan. |

## Composition recipes needed

These are documentation and composition needs, not requests to add product-specific
state to a low-level element.

| Category | Composition | Existing primitives | Recommended recipe |
| --- | --- | --- | --- |
| Patterns | Selectable master-detail workspace | `box-split-view`, `box-table`, `box-empty-state`, `box-skeleton`, `box-drawer` | Document controlled row/card selection, keyboard behavior, empty/loading detail states, and narrow-screen detail presentation. Dispatch owns selected provider/component state. |
| Patterns | Compact application navigation | Light-DOM `<a aria-current>`, `box-badge`, icons, optional drawer | Document expanded and collapsed navigation with accessible labels, tooltips, badges, link semantics, and responsive drawer behavior. Dispatch owns routes and active state. |

## Remaining enhancements after the 0.27.0 review

| Priority | Category | Component or area | Accepted need | Dispatch action until release |
| --- | --- | --- | --- | --- |
| P1 | Foundations | TypeScript custom-element declarations | JSX tag maps and typed event maps for React and TypeScript consumers. | Maintain the narrow local `boe.d.ts` boundary and native event listeners. Remove redundant declarations after adoption. |
| P1 | Components | `box-drawer` action integration | Preserve directly actionable footer and body controls when the drawer portals its content outside a framework's delegated event root. A typed `action` event or documented imperative action bridge would make React integrations reliable. | Keep the drawer, fields, and selects from Box Open Elements, with a narrow native-button listener only for drawer actions. Remove it when the published drawer/button contract supports portaled framework actions. |
| P2 | Components | `box-drawer` close affordance control | A `closable` property or equivalent slot/configuration to opt out of the built-in header Close control when a workflow presents one explicit footer Close action. | Use the published drawer and its footer slot; hide only the duplicate header part on connection drawers until a public configuration is available. |
| P1 | Components | `box-nav-sidebar` collapsed toggle reachability ([#359](https://github.com/unofficialbox/box-open-elements/issues/359)) | Keep the companion toggle reachable after collapse; the published header-slot recipe currently hides the control with the header. | Keep `box-sidebar-toggle-button` in the always-visible body and push it toward the footer with light-DOM layout. |
| P1 | Components | `box-app-shell` + `box-nav-sidebar` landmark composition ([#360](https://github.com/unofficialbox/box-open-elements/issues/360)) | Avoid nested navigation landmarks when the sidebar is placed in the shell's documented nav slot. | Use distinct outer and inner labels so the landmarks remain distinguishable until a single-landmark composition is published. |
| P2 | Patterns | `box-timeline` live-tail composition | A bounded, keyboard-focusable scroll host remains application composition; the component owns semantic events while Dispatch owns the 12-event policy and auto-follow behavior. | Keep the small focusable host rule around `box-timeline`; no upstream enhancement is required by the current contract. |
| P2 | Foundations | `box-run-trace` marker geometry tokens | The pattern's connector position is coupled to its internal step padding. Public marker/connector inset tokens, or an invariant that part-level step padding preserves alignment, would let consuming products tune density without breaking the trace geometry. | Do not override the `step` part. Dispatch relies on the published internal geometry so marker centres and connectors remain aligned. |
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
2. Use `box-progress-steps` for workflow state and interaction; limit Dispatch styling to the published parts needed for the approved horizontal composition.
3. Replace local JSX declarations with official React JSX tag maps if they are published.
4. Remove the sidebar-toggle body workaround after issue #359 is released and adopted.
5. Re-run this source-level intake on each Box Open Elements upgrade and update this document with published-version evidence.

## Adoption rule

Before creating a Dispatch control, inspect the published Box Open Elements source
and catalog. Prefer an existing foundation or component. Record a **composition
recipe needed** when existing primitives cover the behavior but guidance is missing.
Record an **enhancement** when the correct primitive exists but a broadly useful API
is absent. Record a **gap** only when no existing primitive provides the right
foundation and the missing behavior is reusable beyond Dispatch.
