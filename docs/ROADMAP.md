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

Box Forms, Box Apps, Box HTTPS Connectors, and Box Automate workflows remain in
the capability catalog but are nondeployable because they do not have complete
public lifecycle APIs. They are hidden from the component picker by default
in the browser capability catalog as locked reference information.
Box Dispatch does not use private web endpoints or browser automation as a
provider adapter. Partial API coverage, including Automate's list/start-only
surface, is tracked in [`PUBLIC_API_GAPS.md`](PUBLIC_API_GAPS.md).

Reconsider a nondeployable capability only when a documented public API satisfies
the lifecycle acceptance criteria in that gap document.

## 4. Add providers only with a complete lifecycle

Databricks and Amazon Bedrock AgentCore remain visible in the browser as unavailable roadmap
providers. Enable them only after Dispatch can configure, validate, deploy, diagnose, and
audit their supported public lifecycle without exposing credentials to browser code.
