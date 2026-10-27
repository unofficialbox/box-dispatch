# Public API gaps

`box-dispatch` supports provider capabilities only when they can be inspected,
created or updated, and cleaned up through documented public APIs. It does not
ship browser automation or adapters built on private application endpoints.

## Unsupported deployment capabilities

| Capability ID | Public operations | Missing lifecycle operations | Dispatch behavior |
|---|---|---|---|
| `box_extract_agent` | None documented for the Box Automate Extract inventory | List, get, create, update, delete | Not modeled as an AI Studio custom agent; never packaged, validated, deployed, or reset |
| `box_form` | None | List, get, create, update, delete | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `box_app` | None | Portable list, get, create, update, delete, or template instantiation | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `https_connector` | None | List, get, create, update, delete | Cataloged; hidden by default; never packaged, validated, deployed, or reset |
| `automate_workflow` | List and start | Create, update, delete | Cataloged as partial API; hidden by default; never packaged, validated, deployed, or reset |

These entries remain in the solution capability catalog so Dispatch can explain
the gap. They cannot become deployable until Box publishes a supported API
covering the complete lifecycle needed by an automated environment. A private
web endpoint, reverse-engineered browser call, or UI script is not an acceptable
substitute.

### Box Automate Extract is not AI Studio

The **Automate > Extract** product surface manages Box Extract Agents. Box AI
Studio custom agents can separately contain an `extract` capability in their
AI Studio definition. Those objects share terminology but are not interchangeable.

Dispatch uses the documented AI Studio `/ai_agents` API only for custom agents.
It must not translate an Extract prompt file into an AI Studio custom agent and
report that object as a Box Extract Agent. Support for Box Automate Extract Agents
remains blocked until Box documents a public management API that meets the
lifecycle acceptance criteria below.

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
