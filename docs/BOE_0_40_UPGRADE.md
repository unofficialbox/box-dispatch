# Box Open Elements 0.40.0 adoption

Approved scope: upgrade Dispatch's core and React adapter packages from 0.39.0 to the published 0.40.0 together, verify release/artifacts and actually used APIs, and run the required cloud and rendered regression checks. Fixed estimate: **1–2 additional active hours**. Actual upgrade work start: **2026-10-11 22:39:00 UTC**. This baseline/start will not be reset.

Base: approved PR46 merge `6b55673f70cd7fea8d0fe1d3452630a48e034c19`, whose tree matches independently cleared `70522cf7624fd2c5d28d87936b31d5aeb3465366`. Upgrade work is isolated on `codex/boe-0-40-upgrade`. The new upgrade PR will be draft and needs independent review plus further approval to merge. No deployment, live provider calls, operator credential/security changes or Riptide edits are authorized. Safari and human screen-reader checks are outside acceptance.

Artifact inspection, post-merge verification and matched 0.39/0.40 browser checks are complete; the tested adoption is submitted as a draft for independent review.

## Artifact and API verification

Both registry records and the `v0.40.0` tag identify release source `084f94dd16ccc3ff3666b25a6d974d78417ec765`. The later source main `e76b56be1d8ac6525e469f8db0ec0585f3fda6f2` changes only the registry pin in the upstream lockfile. Downloaded core and React tarballs match their registry SHA-512 integrity and SHA-1 hashes. All **978 core files** and **37 React files** in the final installed packages match those verified tarballs; the React adapter declares core peer `^0.40.0`.

Release changes cover Process Modeler #527–531 (keyboard removal/search/Escape, initial and saved view ownership, controlled selection reveal, last-run wording and inspector headings) plus the shared Flow Builder chooser. A source scan finds 42 BOE import entrypoints including type/test imports; Dispatch consumes neither Modeler nor Flow Builder. The changed native-type declaration adds an unused Modeler property. Actual React runtime files are unchanged; core runtime changes are confined to the unused pattern modules. Dispatch needs no API compatibility shim or component/design change, and no Modeler benefit is claimed for its present interface.

Both packages are pinned exactly to 0.40.0. The lockfile changes only those two versions, their integrity hashes and the adapter's core peer constraint. The frontend build produces **byte-identical embedded assets** to merged main, so no generated-asset diff is needed. The existing entry bundle remains 1,382.06 kB (393.42 kB gzip), retaining the warning above the 500 kB threshold.

## Cloud verification

Post-merge 0.39 main passed the required empty-gofmt/build/vet/full-Go-test gate, six-package race checks, 84 frontend unit tests, lint and all **43 Chromium E2E cases**. The 0.40 candidate passes the same Go/race, 84 frontend unit, lint/build and 43 browser checks. Existing cases cover interrupted/repeated workflows, failed or lost responses, stream reconciliation, stale-validation recovery, controlled sorting/selection and mobile/desktop light/dark flows.

Environment: saved Linux cloud workspace; Go 1.26.0, Bun 1.4.2, system Chromium 151; isolated mock service at `http://127.0.0.1:8788`, with no reused server or live deployment-provider calls. Browser plugin is unavailable, so regular Playwright is used. Viewport coverage includes 390/1440 px, light/dark and keyboard interaction. Meaningful page content, state transitions, console logs and screenshots are checked; screenshots were inspected.

## Matched accessibility evidence

A six-scenario rendered audit captures **40 matching states** on actual installed 0.39 and 0.40 packages. The same external audit helper waits for visible computed colors and opacity to remain stable before running axe and screenshots. Both versions produce identical recorded violation-node signatures, incomplete counts and overflow results: **6 serious contrast nodes** in the light connection-drawer samples, **456 incomplete checks**, **zero document-overflow states**, and **zero page errors**. Both retain the same 12 expected cold-mock HTTP 404 console entries and two intentional failure-flow HTTP 503 entries. These findings remain baseline qualifications, not global accessibility conformance or newly fixed issues.

The four additional full-rule stale-review audits also match exactly: one moderate `landmark-main-is-top-level` finding per view, with 6/16/6/15 incomplete contrast checks across mobile/desktop light/dark. Those counts describe a separate, narrower dataset and are not added to the broader totals.

Initial samples taken before computed-style stabilization yielded 51 versus 58 contrast nodes in three differing transient states despite byte-identical assets. Those raw results are retained; the separately repeated stable before/after captures have no recorded differences. No application CSS or design was changed to obtain the comparison. Safari and human screen-reader checks remain excluded, and earlier qualified Drawer/overlap/stress findings remain relevant.

Evidence: `/workspace/box-dispatch-cloud-evidence/boe040`, including registry/tarball records, installed-file comparison, import scan, baseline/candidate browser outputs, stable before/after screenshots and axe JSON, and `stable-matched-audit-comparison.json`. The draft upgrade head requires independent review and explicit approval before merge. Deployment remains held.
