# Public API gaps

`box-dispatch` supports provider capabilities only when they can be inspected,
created or updated, and cleaned up through documented public APIs. It does not
ship browser automation or adapters built on private application endpoints.

## Excluded capabilities

| Capability | Public API gap | Product decision |
|---|---|---|
| Box Forms | No public list, get, create, update, or delete API | Excluded from manifests, packaging, validation, deploy, and reset |
| Box Apps | No public CRUD API or portable template-instantiation API | Excluded from manifests, packaging, validation, deploy, and reset |
| Box HTTPS Connectors | No public lifecycle API | Excluded from manifests, packaging, validation, deploy, and reset |

These capabilities can return only when Box publishes a supported API covering
the complete lifecycle needed by an automated environment. A private web
endpoint, reverse-engineered browser call, or UI script is not an acceptable
substitute.

## Partial public APIs

| Capability | Available publicly | Missing operation | Current behavior |
|---|---|---|---|
| Box Automate workflows | List workflows; start a workflow | Create, update, and delete | Box Dispatch can inspect existing workflows, but does not deploy or remove them |
| Box user identity | User/account identity | Stable tenant web hostname | No browser transport depends on this value; it is not part of deployment |

Partial capabilities remain visible only where the supported operation is useful
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
