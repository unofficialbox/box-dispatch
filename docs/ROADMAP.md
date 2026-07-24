# Roadmap

Larger directions, distinct from day-to-day bug fixes. Each entry records why it
matters with concrete evidence from the code, so the motivation survives.

## 1. Never fall back silently

**Principle:** when a chosen mechanism (an auth method, a transport, an endpoint,
a provider adapter) cannot be used, fail loudly and explain — never quietly
substitute a lesser one.

**Why:** silent fallbacks have repeatedly turned one problem into a confusing,
distant symptom in this codebase.
- `newBoxAPI` degraded a failed SDK/CCG auth to the lower-scoped OAuth CLI path.
  The only visible symptom was a later `403 insufficient_scope` on Doc Gen, with
  no hint that CCG had been attempted and failed. (Fixed: a configured CCG
  failure now surfaces.)
- The unauthenticated browser session reported a Box Form as "absent", which the
  teardown treated as "deleted" — a false success in a destructive operation.
- The Box CLI response envelope going unwrapped made a swallowed `403` read as
  "no results", so hubs were re-created on every run (four duplicates
  accumulated).

**Shape of the rule:** a fallback is only acceptable when the alternative is
genuinely equivalent and the switch is stated. Degrading capability, scope,
ownership, or correctness is not a fallback — it is a hidden failure. Prefer an
actionable error naming what to fix.

## 2. Evaluate replacing the Box CLI with the Box Go SDK

**Direction:** move Box operations off the `box` CLI and onto the SDK directly —
`github.com/unofficialbox/box-open-go-sdk`
(https://pkg.go.dev/github.com/unofficialbox/box-open-go-sdk). The CLI is
convenient for a user to authenticate once, but it is restrictive as a
programmatic backend.

**Why the CLI is restrictive (evidence from this codebase):**
- **No per-command environment selection.** `box tokens:get` / `box request` only
  ever use the *current default* environment; there is no `--environment` flag.
  Selecting a different auth (e.g. a CCG app) means mutating global CLI state with
  `configure:environments:set-current`, which affects every other use of the CLI
  on the machine.
- **Response envelope.** `box request` returns
  `{"statusCode","headers","body"}`, so every caller has to unwrap it, and a
  non-2xx status is easy to miss (see roadmap item 1).
- **Fragile environment storage.** A CCG environment on the test machine pointed
  at a deleted temp config file (`/var/folders/.../tmp.*`) and returned "Could
  not read environments config file" — an entire auth path silently unusable.
- **Token minting limits.** `box tokens:get --user-id` fails on an OAuth
  environment, which is why box-dispatch already falls through to a token bridge.
- **A split, hybrid backend.** `internal/lifecycle/box_sdk.go` maintains two
  implementations of `boxAPI` — a typed `*boxSDK` and a shell-out `boxCLI` — and
  even the SDK path shells to the CLI for uploads (`uploadFileWithCLI`). Auth can
  differ between the two, which is a bug surface.

**What the SDK buys us:**
- Deterministic, per-call auth via a token box-dispatch controls
  (`gantryruntime.DeveloperToken`), including CCG-as-user, without touching global
  CLI state.
- Typed requests/responses with real error/status handling — no envelope
  unwrapping, no swallowed 403s.
- A single backend instead of the `boxSDK` / `boxCLI` split, removing the auth
  mismatch between them.

**Cost / open questions:**
- The SDK is not a drop-in for everything the CLI does today (uploads currently
  go through the CLI; a couple of app-tier surfaces have no public API at all and
  must stay on the browser transport regardless — see BOX_APP_SCHEMA.md and
  PUBLIC_API_GAPS.md).
- Authentication UX changes: the user would provide app credentials (CCG/JWT)
  to box-dispatch rather than relying on an existing `box login`. The CCG
  connection screen is a step in this direction.
- Scope: this is an evaluation, not a committed rewrite — measure how much of the
  current CLI surface the SDK covers before deciding.

## 3. Box App: migrate off the deprecating Meteor API to `/app-api/graphql`

**Decision (executed):** the Box App is now surfaced as a **manual** component
instead of being auto-provisioned, gated behind
`boxAppMeteorDeploySupported = false` in
`internal/lifecycle/box_private_adapters.go`. This is proactive — we don't know
when the API breaks — and it keeps the failure explicit (Roadmap item 1) rather
than letting a deploy die on a removed method.

**Why:** the Box App has no public API. box-dispatch drives Box's internal
"Meteor" app-API through a signed-in browser at
`POST /app-api/crooze/call-meteor-method/v1/<method>`. The Box App path is the
**only** Meteor dependency in the codebase; every other surface uses a different
transport:
- **Box App (Meteor):** `app.list`, `app.get`, `app.create`, `app.lock`,
  `app.update.all`, `app.cancelEdit` (deploy) and `app.remove` / `app.delete` /
  `app.archive` (teardown), plus `savedSearch.get` / `savedSearch.create` for
  metadata-bound charts. `chart.erid === savedSearch._id` (see BOX_APP_SCHEMA.md).
- **Box Form:** `/app-api/file-request-web/` (`/form`, `/form-version`,
  `/file-requests`) — a separate internal API, **not** Meteor; unaffected.
- **Box Automate workflow:** `/app-api/boxrelay` — token-gated, already
  browser/manual.
- **Everything public** (folders, files, metadata, Doc Gen, Hub, AI agents):
  public Box REST via the CCG token.

**Migration trigger:** Box is moving these surfaces to `/app-api/graphql` —
already observed serving live `POST` traffic in the Automate web UI. Flip
`boxAppMeteorDeploySupported` back to `true` (or replace the injected Meteor JS
with GraphQL mutations) once GraphQL exposes app/dashboard **and** savedSearch
mutations equivalent to the `app.*` methods above. Teardown (`app.remove`…) rides
the same API and needs the same migration.

**Cost / open questions:**
- No portable Box App definition exists to clone from anyway (the Meteor path
  required an existing app named the template title — see BOX_APP_SCHEMA.md), so
  demoting to manual loses little today.
- A GraphQL adapter would let the App re-join the automatic deploy set and drop
  the browser dependency for that surface entirely.
