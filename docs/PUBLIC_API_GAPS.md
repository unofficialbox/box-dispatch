# Public API gaps

`box-dispatch` supports provider capabilities only when they can be inspected,
created or updated, and cleaned up through documented public APIs. It does not
ship browser automation or adapters built on private application endpoints.

## Unsupported deployment capabilities

| Capability ID | Public operations | Missing lifecycle operations | Dispatch behavior |
|---|---|---|---|
| `box_form` | None | List, get, create, update, delete | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `box_app` | None | Portable list, get, create, update, delete, or template instantiation | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `https_connector` | None | List, get, create, update, delete | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `automate_workflow` | List and start | Create, update, delete | Cataloged as partial API; hidden by default; never packaged, validated, deployed, or reset |

These entries remain in the solution capability catalog so Dispatch can explain
the gap. They cannot become deployable until Box publishes a supported API
covering the complete lifecycle needed by an automated environment. A private
web endpoint, reverse-engineered browser call, or UI script is not an acceptable
substitute.

## BCL visibility configuration

The launch shell writes project-local display preferences to
`.dispatch/ui-settings.bcl`. `metadata.boxComponentVisibility` maps capability
IDs to booleans:

```json
"boxComponentVisibility": {
  "folder_structure": true,
  "box_form": false,
  "box_app": false,
  "https_connector": false,
  "automate_workflow": false
}
```

Supported capabilities default to `true`. Unsupported and partial-API
capabilities default to `false`. Changing an unsupported entry to `true` shows a
gold, locked reference row on **Configure Box components**; it does not enable
selection, packaging, validation, deployment, or reset. See
[`config/runtime/ui-settings.example.bcl`](../config/runtime/ui-settings.example.bcl)
for the complete example.

## Partial public APIs

| Capability | Available publicly | Missing operation | Current behavior |
|---|---|---|---|
| Box user identity | User/account identity | Stable tenant web hostname | No browser transport depends on this value; it is not part of deployment |

Partial capabilities remain usable only where the supported operation is useful
and cannot imply lifecycle coverage that does not exist. In particular, reset
must report a recorded resource as unmanaged when no public delete operation is
available.

## API acceptance criteria

A capability may be added to an automated Box Dispatch package when:

1. The required endpoints are documented public APIs.
2. Authentication works through the same explicit provider connection as other
   API calls; no second browser session is required.
3. Create/update returns stable resource IDs for the deployment audit.
4. Validation can distinguish missing, present, and unauthorized states.
5. Reset can delete strictly by the recorded resource ID, or the capability is
   explicitly read-only and never represented as deployable.

This boundary keeps deployments reproducible, teardown safe, CI practical, and
provider failures diagnosable.
