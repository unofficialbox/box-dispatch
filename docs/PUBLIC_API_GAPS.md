# Cost of Missing Public APIs

What it costs `box-dispatch` to support Box capabilities that have no public API,
measured against what the same capabilities cost when a public API exists.

## How these numbers were derived

- **"LOC written"** is measured from this repository, not estimated: whole-file
  counts (`wc -l`) for files that exist solely as workarounds, and brace-matched
  function bodies for individual routines.
- **"LOC with a public API"** is not a guess either. It is the *measured* cost of
  comparable capabilities in this same codebase that do have public APIs:

  | Comparable capability | list + create + delete, both backends |
  |---|---|
  | Box Hub | **52 lines** |
  | Box AI Agent | **63 lines** |

  Plus roughly 18 lines of validate/deploy wiring per capability. So a fully
  supported public-API capability costs **~70–80 lines** here. That is the
  benchmark used in the estimate column.
- Line counts **understate** the private-surface cost. The injected browser
  script is written as dense one-liners: **11,366 characters and ~124 statements
  compressed into 26 physical lines.** Formatted normally it would be ~124 lines.

## The table

| Product area | Public API today | LOC written to work around it | LOC if a public API existed | Multiplier |
|---|---|---|---|---|
| **Box Forms** | none | ~160 (share of adapter + injected JS) | ~75 | ~2× |
| **Box Apps** | none | ~160 (share of adapter + injected JS) | ~75 | ~2× |
| **Browser transport** — CDP attach, tab selection, session preflight | n/a — exists *only* because Forms/Apps have no API | **252** | **0** | ∞ |
| **Browser lifecycle** — cross-platform discovery, launch, dedicated profile, tenant-URL derivation | n/a — same reason | **171** | **0** | ∞ |
| **Tests for the above** | n/a | **121** | **0** | ∞ |
| **Automate Workflow — delete** | `List` + `Start` only | **0 — cannot be implemented at all** | ~10 | capability unavailable |
| **Tenant web host discovery** | `users/me` does not return it; scraped from `avatar_url` | **17** | ~1 (read a field) | ~17× |
| **Totals** | | **~864 lines of Go + ~124 JS statements** | **~150** | **~6×** |

## Costs that are not lines of code

These matter more than the LOC multiplier and should carry the argument:

| Cost | Detail |
|---|---|
| **6 new dependencies** | `chromedp`, `cdproto`, `sysutil`, `gobwas/ws`, `gobwas/pool`, `gobwas/httphead` — 14 `go.sum` entries, pulled in purely to drive a browser |
| **+3 MB binary** | 17 MB → 20 MB |
| **A browser is now a runtime dependency** | Chrome/Chromium/Edge must be installed, launched with `--remote-debugging-port`, and signed in — for two component types |
| **A second authentication context** | Operators authenticate twice: OAuth for the public API, and an interactive browser session for Forms/Apps. The two share nothing |
| **Unverifiable correctness** | Delete calls are reverse-engineered method names (`app.remove` / `app.delete` / `app.archive`). There is no contract to test against |
| **Silent-failure risk** | The app tier answers an unauthenticated request with **HTTP 200 and an empty list**, not 401. That once made `box-dispatch` report a Form as deleted when it had touched nothing — a false success in a destructive operation. It took an explicit host check plus refusing to treat "absent" as "deleted" to make it safe |
| **Breaks on any UI change** | Cloning a Box App means replicating the web client's own document model: deep-copying the page/section/item tree, rewriting `enterprise_<id>` references, regenerating 17-character IDs, repointing `erid` references, and driving a lock → update → cancelEdit cycle |
| **Local security surface** | The DevTools port is unauthenticated. Any local process can attach and act as the signed-in Box user while the browser is running |

## What would remove the cost

1. **CRUD for Box Forms** — create, list, get, delete.
2. **CRUD for Box Apps**, plus "instantiate from template" so nobody has to clone
   page trees by hand.
3. **Delete for Automate workflows.** Today a workflow can be started via API but
   never removed — so an environment can be provisioned but not reset.
4. **Return the tenant web host from `users/me`.** It is currently only obtainable
   by parsing it out of `avatar_url`, which is incidental rather than contractual.
5. **Return 401, not 200 + empty data, when unauthenticated.** Clients cannot
   distinguish "nothing exists" from "you cannot see it," which turns an auth
   failure into silent data loss in any automation.
6. **A parity principle:** anything creatable in the web UI should be creatable
   *and deletable* through the public API. The business case is reproducible demo,
   test, and CI environments — today they can be built but not reliably torn down.

## Bottom line

Two component types with no public API cost **~6× the code** of an equivalent
public capability, plus a browser dependency, six libraries, a second auth
context, and a class of silent-failure bug that does not exist on the public API
path. Everything else in the solution — folders, files, metadata templates, Doc
Gen, Extract, AI agents, hubs, Automate deployment — is served by the public API
and costs a few dozen lines each.
