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

The current Dispatch web package uses `@unofficialbox/box-open-elements` 0.5.0.
For each evaluated element, intake must inspect the published element's
`observedAttributes`, public properties, methods, and emitted events rather than
inferring capability from a screenshot or tag name. Upstream and maintainer-reported
work must remain distinct from functionality available in the installed package.

The maintainer response records several completed and accepted items. Some completed
items are not yet available in the published 0.5.0 package, so Dispatch must not depend
on them until a release containing those APIs is installed and verified.

## Adopted in Dispatch

| Category | Box Open Elements capability | Dispatch use |
| --- | --- | --- |
| Foundations | Design tokens and `boxIconography` | Color, typography, spacing, focus treatment, and navigation icons. |
| Components | `box-button` | Primary, secondary, and destructive workflow actions. |
| Components | `box-card` | Summary, selectable, configuration, and detail surfaces. |
| Components | `box-switch` | Provider and component enablement. |
| Components | `box-badge` | Live and verified state labels. |
| Components | `box-progress-bar` | Connection and run progress. |
| Components | `box-drawer` | Connection editing and run diagnostics, including focus management, Escape/backdrop dismissal, and controlled open state. |
| Components | `box-text-field` and `box-select` | Box credentials and authenticated Salesforce-org selection. |
| Components | `box-split-view` | Connect and Configure master-detail page structure. |

React 19 sets custom-element properties directly. Dispatch currently keeps a small
local JSX declaration file and listens to typed custom events at the application
boundary; an official React wrapper is not required.

## Available components that are not gaps

| Category | Component | Verified contract | Dispatch decision |
| --- | --- | --- | --- |
| Components | `box-progress-steps` | Published 0.5.0 provides controlled `value`, `value-changed`, arrow/Home/End navigation, roving tab index, and `aria-current`. | Do not use it as the horizontal wizard header. Its vertical setup-rail semantics do not match the clickable deployment workflow. Re-evaluate after per-step statuses are published. |
| Components | `box-table` | Controlled sorting and selection are already supported. | Use for ordinary history and selection tables. Rich cells and responsive behavior remain enhancements, not reasons to replace its current API. |
| Components | `box-drawer` | Controlled `open`, `open-changed`, `dismiss`, `show()`, `close()`, focus containment/restoration, backdrop and Escape behavior, and body portaling are present. | Adopted. Sticky actions and richer sizing remain enhancements. |
| Components | `box-text-field` | Shared field contract plus `value-changed`, loading, and valid states are present. | Adopted for connection forms. Autocomplete and password reveal remain enhancements. |
| Components | `box-select` | Shared field contract plus options, single/multiple values, and `value-changed` are present. | Adopted for authenticated-org and subject selection. Async loading, empty, and error states remain enhancements. |
| Components | `box-split-view` | Controlled ratio, optional resizing, and `ratio-changed` are present. | Adopted as the Connect and Configure layout primitive. Selection and detail behavior remain application composition. |

`box-stage-path` exists on upstream `main`, but it is not in the published 0.5.0
package and currently represents a read-only horizontal record lifecycle. It is not
a replacement for Dispatch's clickable, eligibility-gated wizard navigation.

## Composition recipes needed

These are documentation and composition needs, not requests to add product-specific
state to a low-level element.

| Category | Composition | Existing primitives | Recommended recipe |
| --- | --- | --- | --- |
| Patterns | Selectable master-detail workspace | `box-split-view`, `box-table`, `box-empty-state`, `box-skeleton`, `box-drawer` | Document controlled row/card selection, keyboard behavior, empty/loading detail states, and narrow-screen detail presentation. Dispatch owns selected provider/component state. |
| Patterns | Compact application navigation | Light-DOM `<a aria-current>`, `box-badge`, icons, optional drawer | Document expanded and collapsed navigation with accessible labels, tooltips, badges, link semantics, and responsive drawer behavior. Dispatch owns routes and active state. |

## Accepted enhancements not yet available in published 0.5.0

| Priority | Category | Component or area | Accepted need | Dispatch action until release |
| --- | --- | --- | --- | --- |
| P0 | Components | `box-progress-steps` | Per-step statuses for complete, current, pending, blocked, and failed steps. The maintainer reports this as completed, but the API is not in the installed release. | Keep the local clickable workflow header and re-evaluate when the containing package release is published. |
| P1 | Components | `box-select` | Loading, empty, and error states. | Render adjacent application feedback without changing the select contract. |
| P1 | Components | `box-text-field` | Autocomplete and password-visibility support. | Keep security guidance adjacent to the field; do not recreate a competing generic field. |
| P1 | Foundations | TypeScript custom-element declarations | JSX tag maps and typed event maps for React and TypeScript consumers. | Maintain the narrow local `boe.d.ts` boundary and native event listeners. Remove redundant declarations after adoption. |
| P1 | Components | `box-table` | Rich cell content, row expansion, loading/empty/error states, and responsive behavior. | Use the table where plain controlled rows fit; retain application layout for rich run-stage summaries. |
| P1 | Components | `box-drawer` | Sticky footer/action slots, size presets, busy state, mobile full-screen behavior, and unsaved-change guard. | Keep actions in slotted body content and application-owned save state. |
| P2 | Patterns | Master-detail and compact-navigation recipes | Official examples covering the compositions above. | Keep Dispatch compositions small and documented. |

Accessibility corrections for `box-dropdown`, `box-selectable-card`, and
`box-action-bar` are reported complete by the maintainer. Dispatch should consume
them through the next containing package release rather than applying local forks.

## Missing component gap

### Live run timeline

**Category:** Patterns  
**Roadmap status:** Accepted as `box-run-timeline`

Validation and deployment need aligned event nodes, connectors, timestamps,
expandable details, nested progress rows, and live success/warning/failure state.
The generic timeline primitives do not provide that complete operational model.

The accepted pattern should support:

- event status, timestamp, title, description, and expandable detail content;
- nested task/progress content with live updates;
- stable node and connector alignment at all viewport widths;
- accessible live-state announcements and failure recovery context.

Dispatch will keep its temporary page-level run composition until
`box-run-timeline` is published. It should then migrate rather than fork the pattern.

## Declined proposal

An official React wrapper is not planned. The supported direction is React 19's
native custom-element property handling plus generated TypeScript tag and event maps.
Dispatch may keep a tiny local helper for event subscription, but must not build a
parallel wrapper library.

## Dispatch adoption sequence

1. Use the published drawer, field, select, split-view, table, and progress APIs where their verified contracts fit.
2. Retain the custom clickable wizard header until a published Box Open Elements pattern supports horizontal navigation and eligibility/status semantics.
3. Retain the local live-run composition until `box-run-timeline` is published.
4. Replace local JSX declarations with the official TypeScript maps after release.
5. Re-run this source-level intake on each Box Open Elements upgrade and update this document with published-version evidence.

## Adoption rule

Before creating a Dispatch control, inspect the published Box Open Elements source
and catalog. Prefer an existing foundation or component. Record a **composition
recipe needed** when existing primitives cover the behavior but guidance is missing.
Record an **enhancement** when the correct primitive exists but a broadly useful API
is absent. Record a **gap** only when no existing primitive provides the right
foundation and the missing behavior is reusable beyond Dispatch.
