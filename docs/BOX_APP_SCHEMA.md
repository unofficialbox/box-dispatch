# Box App internal schema (captured 2026-07-24)

Captured live from `kadams.ent.box.com` via the app-tier API
(`/app-api/crooze/call-meteor-method/v1/app.get`), so box-dispatch can build a
Box App from a portable definition instead of cloning a live one.

This is an **undocumented internal API**. Everything here is observation, not
contract, and can change without notice.

## Why this matters

`deployBoxPrivateAdapters` clones an existing app named by the manifest's
`template` field. That cannot work in a new Box enterprise, where no such app
exists. The manifest already declares `"source": "box-app-blueprint.md"` and
calls it "a portable layout specification" — the intent was always a definition
that travels with the solution. Only the implementation diverged.

## Object shapes

### App
```
_id, name, enterpriseId, versionNumber, lock, permissions, accessList,
icon (base64), pages[], createdAt/createdBy, updatedAt/updatedBy
```

### Page
```
_id, appId, name, sections[], items[], createdAt/createdBy, updatedAt/updatedBy
```

### Section
```
id, title, layout ("grid"), position {x,y}, size {h,w},
createdAt/createdBy, modifiedAt/modifiedBy
```

### Item (block)
```
id, name, type, sectionId, position {x,y}, size {w,h,contents},
data {...}, erid, savedSearch {...}, searchFields[],
theme, icon, isShowIcon, isShowItemCount, version,
createdAt/createdBy, modifiedAt/modifiedBy
```

**Item types observed:** `chart`, `shortcut`, `fileFolder`, `search`.
The app builder UI additionally offers: View, Chart, Item List, File or Folder,
Form, App, AutoFiler, Shortcut.

### Chart `data`
```json
{
  "chartType": "donutChart",
  "templateKey": "grantApplication",
  "attributeToVisualize": "applicationStatus",
  "barChartOrientation": "horizontal",
  "categoriesCount": 8
}
```

Charts bind to a **metadata template key plus an attribute** — which is exactly
how `box-app-blueprint.md` already describes the CLM charts
(`clmDocument.approvalStatus`, `clmDocument.clauseRisk`, `clmClause.position`).

### savedSearch (referenced by `erid`)
```
_id, appId, appPageId, dashboardId, createdBy, createdDate, version,
settings { metadataTemplateID, columns[], gridFields[], search[],
           sortInfo {column,direction}, permission, thumbnailShow, ... }
```

## How each block binds to a resource (`erid`)

Captured from a live app. This is the contract a portable definition has to
express symbolically and re-resolve per environment.

| Block type | `erid` holds | `data` carries | Symbolic form in a definition |
|---|---|---|---|
| `fileFolder` | the **numeric folder id** (`375833318039`) | `contentType: "folder"`, `contentName` | folder name, e.g. `01 - Intake` |
| `shortcut` | a **URL** (`https://<tenant>.ent.box.com/f/…`, `/s/…`, `/sign/inbox`) | — | named target, e.g. the Hub, or a literal path like `/sign/inbox` |
| `form` | `"form_" + formId` | — | form title, e.g. `New Contract Request` |
| `chart` / `search` | a **savedSearch id** | `chartType`, `templateKey`, `attributeToVisualize`, `categoriesCount`, `barChartOrientation` | metadata template key + attribute (already environment-independent) |

Charts and searches are the only blocks whose binding is not a plain id or URL:
they point at a `savedSearch` record, which is why that record has to be created
(or derived) at deploy rather than simply rewritten.

Note `shortcut` erids are absolute tenant URLs, so they must be rewritten to the
target tenant host — not just repointed at a new id.

### Read-only cross-check

The public API exposes `GET /folders/{id}/app_item_associations` and
`GET /files/{id}/app_item_associations`, which report the app items associated
with a folder or file. These are **read-only** — useful for verifying what a
deploy produced, and for confirming the JSON shape, but not a write path. There
is no public `/app_items/{id}` (404).

## What a portable definition must strip and rebind

| Field | Handling |
|---|---|
| `_id`, `id`, `appId`, `appPageId`, `dashboardId`, `savedSearch._id` | regenerate per deploy (17-char ids) |
| `enterpriseId` | rewrite to the target enterprise |
| `createdBy`, `modifiedBy` | drop; set by the target |
| `erid` on `fileFolder` / `form` / `shortcut` | **symbolic reference** — store the folder name, form title or hub name and resolve to the target's real id at deploy |
| `templateKey`, `metadataTemplateID`, `attributeToVisualize` | keep as-is; these are solution-level names the deploy already creates |
| `position`, `size`, `layout`, `title`, `name`, `type`, `chartType` | keep as-is; this is the layout being made portable |

The symbolic rebinding machinery **already exists**: `privateAdapterRequest`
passes a `resources` map of name → `{id, kind, url}`, and the current clone path
uses it to set `erid` for `fileFolder`, `form` and `shortcut` blocks.

## Two defects found while capturing this

1. **The template lookup can never match a deployed app.** The manifest's
   `template` is the raw name (`Contract Lifecycle Management`), but the
   `create_new` naming strategy produces run-suffixed instances
   (`Contract Lifecycle Management - 20260722T203030Z`). The JS compares
   `app.name === templateTitle` exactly, so a previously deployed app is never
   recognised as a template.
2. **A Box App failure discards a successfully created Box Form.** The injected
   script handles the form first and the app second; an app error rejects the
   whole promise, so the form's result — and its resource record — is lost, which
   orphans a form that was actually created.
