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
