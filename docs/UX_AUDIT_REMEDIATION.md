# UX audit remediation — October 11, 2026

Implementation baseline: `f10524bd2e36c6cad36b398e0a4531ea123af593`.
The original cloud continuation checked the exact requested `f47bcd782362d22cc1f1e0d2fd3be4957db3f18d`; subsequent reviewed upgrades and accessibility changes are already merged on this baseline. This follow-up addresses the nine defects in the cloud UX audit. Full component exclusion support is out of scope.

## Fixed effort baselines and actual starts

These are the estimates approved before implementation; they are not revised as work proceeds. Times are UTC on October 11, 2026.

| Defect | Baseline | Actual start | Status |
| --- | --- | --- | --- |
| Duplicate deployment starts | 4–6 h | 18:07:36 | Verified in cloud |
| Misleading component controls | 1–2 h | 18:13:17 | Verified in cloud |
| Saved connection action clipping | 1–2 h | — | Pending |
| Popup cancellation / login timeout | 2–4 h | — | Pending |
| Diagnostic loading failure | 1–2 h | — | Pending |
| Failed-run recovery | 2–4 h | — | Pending |
| Interrupted event stream | 2–4 h | — | Pending |
| Initial API error / false-empty state | 2–4 h | — | Pending |
| Help destination | 0.5–1 h | — | Pending |

## P1 execution and scope safeguards

A successful validation can claim exactly one deployment run. The claim and queued run are saved together under the run-manager lock before provider execution. Concurrent requests and replay after a lost response, failure, completion, or service restart return that same run. A failed save starts no executor and allows retry after storage recovers. An intentional attempt to apply again requires fresh validation.

The browser synchronously guards the request and disables continuation while waiting for its response. Sanitized API failure guidance remains visible. Provider inclusion is still supported; browser-only component switches and invented inventory counts have been removed. Configure and Review explain that the prepared package defines component scope. No CLI/API package-scoping contract is changed.

Focused backend tests send 32 concurrent requests, replay failure, restore persisted history, validate again for an intentional retry, and simulate storage failure. Browser tests hold a real mock-backend deployment response and repeat the accepted HTTP request at 390px and 1440px in light/dark themes. They assert one run, a disabled pending action, and honest scope text.

No live provider calls, credential or authorization-grant changes, deployment, or merge are part of this work. Acceptance uses Chromium, keyboard checks, and automated accessibility. The existing large-bundle build warning remains.

P1 gate: empty `gofmt -l .`; Go build/vet/tests and webapi race tests pass. Frontend lint, 80 unit tests, production build, and all 9 isolated Chromium E2E scenarios pass. Rendered pending/completed screenshots are retained under `/workspace/box-dispatch-cloud-evidence/ux-remediation/p1`. No theme files changed.
