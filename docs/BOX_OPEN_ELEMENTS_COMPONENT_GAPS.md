# Box Open Elements component adoption, gaps, and enhancements

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

The current Dispatch web package uses `@unofficialbox/box-open-elements` 0.12.0.
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
| Components | `box-spinner` | The currently active item in the live activity feed. |
| Components | `box-metric-card` | Overview summary metrics and readiness state. |
| Components | `box-drawer` | Connection editing and run diagnostics, including large sizing, busy state, sticky footer actions, focus management, Escape/backdrop dismissal, and controlled open state. |
| Components | `box-text-field` and `box-select` | Box credentials and authenticated Salesforce-org selection, including autocomplete, password reveal, loading, and empty-state support. |
| Components | `box-split-view` | Connect and Configure master-detail page structure. |
| Patterns | `box-run-trace` | Live provider and component validation/deployment activity. |

The published React adapter currently provides wrappers for `box-button`,
`box-text-field`, `box-select`, and `box-dialog`, where typed property and event
bridging is useful. Components without an adapter wrapper remain supported native
custom elements in React 19; Dispatch keeps a narrow JSX declaration boundary for
those elements.

## Available components that are not gaps

| Category | Component | Verified contract | Dispatch decision |
| --- | --- | --- | --- |
| Components | `box-progress-steps` | Published 0.6.0 provides controlled navigation and per-step complete, current, pending, blocked, failed, and disabled states. | Do not use it as the horizontal wizard header. Its vertical setup-rail semantics do not match the approved clickable deployment workflow. |
| Components | `box-table` | Escaped text, badge, and link cells plus expansion, loading, empty, error, sorting, and selection are supported. | Keep for data sets whose cells fit the published descriptors. Deployment history needs official provider-logo media, so it remains a small semantic application table until a safe image cell is published. |
| Components | `box-drawer` | Controlled state, focus behavior, sticky footer, size presets, busy state, mobile presentation, and cancelable dismissal are present. | Adopted for connection and diagnostics workflows. |
| Components | `box-text-field` | Shared field contract, autocomplete, password reveal, loading, and valid states are present. | Adopted for connection forms. |
| Components | `box-select` | Shared field contract, loading and empty states, options, single/multiple values, and `value-changed` are present. | Adopted for authenticated-org and subject selection. |
| Components | `box-split-view` | Controlled ratio, optional resizing, and `ratio-changed` are present. | Adopted as the Connect and Configure layout primitive. Selection and detail behavior remain application composition. |
| Components | `box-nav-sidebar` | Structured navigation items, collapsible behavior, slots, and badges are present. | Keep the compact Dispatch rail for now because the approved interaction uses route links, icon-only geometry, and application-owned active state. Track a compact-link recipe rather than forking the component. |
| Components | `box-stage-path` | Read-only horizontal lifecycle steps are published. | Do not use for editable wizard navigation because it does not expose eligibility-gated step activation. |
| Components | `box-progress-ring` | A minimum 48px ring that includes a percentage and label. | Use only when showing aggregate progress. It is intentionally not used as a per-row activity marker because repeated 0%-100% rings make the live log harder to scan. |

## Composition recipes needed

These are documentation and composition needs, not requests to add product-specific
state to a low-level element.

| Category | Composition | Existing primitives | Recommended recipe |
| --- | --- | --- | --- |
| Patterns | Selectable master-detail workspace | `box-split-view`, `box-table`, `box-empty-state`, `box-skeleton`, `box-drawer` | Document controlled row/card selection, keyboard behavior, empty/loading detail states, and narrow-screen detail presentation. Dispatch owns selected provider/component state. |
| Patterns | Compact application navigation | Light-DOM `<a aria-current>`, `box-badge`, icons, optional drawer | Document expanded and collapsed navigation with accessible labels, tooltips, badges, link semantics, and responsive drawer behavior. Dispatch owns routes and active state. |

## Remaining enhancements after the 0.6.0 upgrade

| Priority | Category | Component or area | Accepted need | Dispatch action until release |
| --- | --- | --- | --- | --- |
| P1 | Foundations | TypeScript custom-element declarations | JSX tag maps and typed event maps for React and TypeScript consumers. | Maintain the narrow local `boe.d.ts` boundary and native event listeners. Remove redundant declarations after adoption. |
| P1 | Components | `box-drawer` action integration | Preserve directly actionable footer and body controls when the drawer portals its content outside a framework's delegated event root. A typed `action` event or documented imperative action bridge would make React integrations reliable. | Keep the drawer, fields, and selects from Box Open Elements, with a narrow native-button listener only for drawer actions. Remove it when the published drawer/button contract supports portaled framework actions. |
| P2 | Components | `box-drawer` close affordance control | A `closable` property or equivalent slot/configuration to opt out of the built-in header Close control when a workflow presents one explicit footer Close action. | Use the published drawer and its footer slot; hide only the duplicate header part on connection drawers until a public configuration is available. |
| P2 | Components | `box-table` image cell descriptor | A safe declarative image/media cell with source, alt text, dimensions, and optional accompanying label. It must preserve the component's escaped-data boundary rather than accepting arbitrary HTML or render callbacks. | Use a semantic application-owned table for the five-row deployment history so official provider marks remain accessible and unmodified. Return to `box-table` when a safe image descriptor is published. |
| P1 | Patterns | Horizontal workflow navigation | A clickable horizontal wizard that combines eligibility gating with complete, current, pending, blocked, and failed states. | Keep the small local workflow header; neither the vertical `box-progress-steps` nor read-only `box-stage-path` matches the approved workflow. |
| P2 | Patterns | `box-run-trace` live-tail behavior | An opt-in auto-follow mode for continuously appended run events, with a bounded scroll region and an accessible way to inspect older activity. | Keep the small Dispatch scroll container around the BOE run trace/activity composition; it always follows the newest event while retaining the most recent 100 rows. |
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

1. Use the published drawer, field, select, split-view, metric-card, table, run-trace, and progress APIs where their verified contracts fit.
2. Retain the custom clickable wizard header until a published Box Open Elements pattern supports horizontal navigation and eligibility/status semantics.
3. Replace local JSX declarations with official React JSX tag maps if they are published.
4. Re-evaluate `box-nav-sidebar` after a compact link-navigation recipe is available.
5. Re-run this source-level intake on each Box Open Elements upgrade and update this document with published-version evidence.

## Adoption rule

Before creating a Dispatch control, inspect the published Box Open Elements source
and catalog. Prefer an existing foundation or component. Record a **composition
recipe needed** when existing primitives cover the behavior but guidance is missing.
Record an **enhancement** when the correct primitive exists but a broadly useful API
is absent. Record a **gap** only when no existing primitive provides the right
foundation and the missing behavior is reusable beyond Dispatch.
