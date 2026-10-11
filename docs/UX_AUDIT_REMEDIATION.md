# UX audit remediation — October 11, 2026

Implementation baseline: `f10524bd2e36c6cad36b398e0a4531ea123af593`.
The original cloud continuation checked the exact requested `f47bcd782362d22cc1f1e0d2fd3be4957db3f18d`; subsequent reviewed upgrades and accessibility changes are already merged on this baseline. This follow-up addresses the nine defects in the cloud UX audit. Full component exclusion support is out of scope.

## Fixed effort baselines and actual starts

These are the estimates approved before implementation; they are not revised as work proceeds. Times are UTC on October 11, 2026.

| Defect | Baseline | Actual start | Status |
| --- | --- | --- | --- |
| Duplicate deployment starts | 4–6 h | 18:07:36 | Verified in cloud |
| Misleading component controls | 1–2 h | 18:13:17 | Verified in cloud |
| Saved connection action clipping | 1–2 h | 18:17:23 | Verified in cloud |
| Popup cancellation / login timeout | 2–4 h | 18:17:40 | Verified in cloud |
| Diagnostic loading failure | 1–2 h | 18:19:15 | Verified in cloud |
| Failed-run recovery | 2–4 h | 18:19:32 | Verified in cloud |
| Interrupted event stream | 2–4 h | 18:20:00 | Verified in cloud |
| Initial API error / false-empty state | 2–4 h | 18:20:34 | Verified in cloud |
| Help destination | 0.5–1 h | 18:21:22 | Verified in cloud |

## P1 execution and scope safeguards

A successful validation can claim exactly one deployment run. The claim and queued run are saved together under the run-manager lock before provider execution. Concurrent requests and replay after a lost response, failure, completion, or service restart return that same run. A failed save starts no executor and allows retry after storage recovers. An intentional attempt to apply again requires fresh validation.

The browser synchronously guards the request and disables continuation while waiting for its response. Sanitized API failure guidance remains visible. Provider inclusion is still supported; browser-only component switches and invented inventory counts have been removed. Configure and Review explain that the prepared package defines component scope. No CLI/API package-scoping contract is changed.

Focused backend tests send 32 concurrent requests, replay failure, restore persisted history, validate again for an intentional retry, and simulate storage failure. Browser tests hold a real mock-backend deployment response and repeat the accepted HTTP request at 390px and 1440px in light/dark themes. They assert one run, a disabled pending action, and honest scope text. New deployment navigation while a start response is pending returns to the accepted workflow; it cannot prepare another package until that response resolves. Intentional New deployment works after completion.

No live provider calls, credential or authorization-grant changes, deployment, or merge are part of this work. Acceptance uses Chromium, keyboard checks, and automated accessibility. The existing large-bundle build warning remains.

P1 gate: empty `gofmt -l .`; Go build/vet/tests and webapi race tests pass. Frontend lint, 80 unit tests, production build, and all 9 isolated Chromium E2E scenarios pass. Rendered pending/completed screenshots are retained under `/workspace/box-dispatch-cloud-evidence/ux-remediation/p1`. No theme files changed.

## Recovery, navigation, and readable saved connections

The follow-up branch is stacked on the P1 branch so its Retry validation action uses the one-validation/one-deployment claim contract. Failed runs retain diagnostics and history; Edit connections and Retry validation provide explicit recovery. A rejected deployment-start request keeps its sanitized guidance visible and releases the pending guard.

Saved connection rows use a shrinking grid track and minimum width so long names cannot push their action controls out of the card. Browser geometry checks cover 1440px, 390px, and 320px in both themes.

Local OAuth watchers cancel on drawer closure, a replacement attempt, and unmount; stop polling after a terminal result; bound pending work to two minutes; and reject late responses from canceled attempts. A final local-status check distinguishes a completed callback that closes its popup from a canceled login. Existing server-side job expiry is now enforced during status polling too. Credentials, login grants, callback configuration, scopes, and provider implementations are unchanged. Closing a drawer stops its watcher and does not revoke completed provider authorization. Only an active OAuth attempt can release its shared busy state on closure; completed attempts clear their ownership. Pending select/remove operations stay busy across closure and reopening. Browser regressions cover both operations for both providers after a completed local login.

Diagnostics have separate loading and error states, visible Retry guidance, and abort cleanup. Startup reads distinguish unavailable plan, catalog, defaults, deployment history, run history, and connections from legitimate empty results. Affected pages show an explicit error with Retry loading, while navigation and Help remain available. Retry restores the saved run when it still owns the workflow. A new draft increments a generation guard, so late retry responses cannot restore the old name/run/phase or overwrite draft selections. Retry does not start provider work.

Interrupted SSE shows feedback and reconciles through read-only run-status requests with bounded backoff after failures. Terminal events cannot be overwritten by an older running-status response; new runs and unmount cancel old watchers. Persistent HTTP stream failures still reconcile through REST even when Chromium closes that EventSource; closed streams are reopened at the bounded retry boundary. A successful interrupted SSE connection also reconnects normally. If REST confirms a terminal run before its terminal stream event is observed, Dispatch retains an explicit warning that activity history is incomplete and may show an earlier state. It does not fabricate component completion from the summary.

Help has a real local `#help` destination, a primary page heading, current navigation state, and accurate workflow/recovery guidance. Direct reload and Back are tested.

## Verification and review boundaries

The final combined regression gate includes 84 frontend unit tests, lint, TypeScript/production build, and 35 isolated Chromium E2E scenarios. Go format/build/vet/full tests and the webapi race suite are required before publication. The browser suite includes the original five workflows, four P1 theme/viewport cases, twelve recovery flows in light/dark at 390px/1440px, five individual startup API failure/retry cases, a canceled delayed login start, a terminal SSE/stale REST response race, reconnection after a permanently closed HTTP-error stream, four select/remove drawer-closure races after completed local login, incomplete terminal validation activity, and a history retry interrupted by New deployment.

The separate rendered audit runs six workflows and captures 41 states. Its results match the verified 0.39.0 baseline: six full-page drawer contrast nodes, 453 contrast incompletes, and four label/content-name incompletes. These qualified shared-component/measurement findings are retained rather than claimed resolved. Application-owned scoped accessibility assertions pass, with no runtime page errors or measured page overflow in the audited states. Acceptance uses Chromium, keyboard checks, and automated accessibility.

Evidence: `/workspace/box-dispatch-cloud-evidence/ux-remediation/`; rendered JSON/screenshots under `rendered`, regression screenshots under `p1` and `recovery`, and gate logs under `final`. Toolchain: Go 1.26.0, Bun 1.4.2, system Chromium 151. The managed browser download was unavailable (403); the cloud uses `/usr/bin/chromium` through an isolated Playwright config. No Mac dependency is required. The existing large-chunk warning remains (approximately 1.38 MB main JS, 393 KB gzip).

P1 draft: https://github.com/unofficialbox/box-dispatch/pull/44 at `087b7945c98fa687f8c8cc7ff0340209c98214c1`, targeting main `f10524bd2e36c6cad36b398e0a4531ea123af593`. The recovery draft targets `codex/dispatch-p1-execution-scope`; review it as a stack, then reconcile its base after P1 is merged. Neither draft authorizes merging or live deployment. Full component exclusion support remains out of scope. No dependency/theme/security configuration or unrelated files are changed.

## Pre-existing stale-validation gap — open review decision

The one-validation/one-deployment claim prevents duplicate execution; it does not bind execution to an immutable validated context. `startDeployment` reuses the validation plan/items, while `lifecycle.DeployProvider` reloads current connection settings and builds from the package path. `PUT /api/plan` can rewrite strategy, package assembly can change files, and connection selection/scratch/OAuth saves can change the destination independently of the claim. These paths and behavior predate this PR.

A plan-only comparison or browser invalidation would leave target/package changes and concurrent mutations uncovered. A bounded server guard must reject unclaimed validations after relevant plan/package/selected-target changes, persist that invalidation across restart, and serialize validation/claim against those writes. It must preserve replay of already claimed runs and avoid invalidating on same-target token refresh or verification timestamps. It must also define what happens if inputs change after a deployment is claimed, because the executor reloads them later. Binding the actual package and destination snapshot through execution is the stronger alternative.

No partial guard is presented as closing this gap in these drafts. This remains an actionable parent review decision before approval of live execution; no provider deployment was performed. The original effort estimates and actual starts above remain unchanged.
