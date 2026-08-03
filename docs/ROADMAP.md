# Roadmap

Larger directions, distinct from day-to-day bug fixes.

## 1. Never fall back silently

When a chosen authentication method, transport, endpoint, or provider adapter
cannot be used, fail clearly and explain the corrective action. Do not silently
substitute a lower-scope or less reliable mechanism.

Examples already addressed include configured CCG authentication degrading to a
different CLI identity and wrapped CLI errors being mistaken for empty provider
results.

## 2. Continue consolidating on the Box Go SDK

Prefer `github.com/unofficialbox/box-open-go-sdk` over shelling out to the Box
CLI when the SDK exposes the required public API. This gives Box Dispatch
deterministic per-call authentication, typed responses, and explicit status
handling without mutating the user's global CLI environment.

Uploads and any remaining CLI-backed calls should move only when equivalent SDK
coverage is proven. Keep the current toolchain when the SDK does not yet expose
an equivalent supported operation.

## 3. Keep a public-API-only capability boundary

Box Forms, Box Apps, and Box HTTPS Connectors are excluded because they do not
have complete public APIs. Box Dispatch does not use private web endpoints or
browser automation as a provider adapter. Partial API coverage, including the
missing create/delete operations for Automate workflows, is tracked in
[`PUBLIC_API_GAPS.md`](PUBLIC_API_GAPS.md).

Reconsider an excluded capability only when a documented public API satisfies
the lifecycle acceptance criteria in that gap document.
