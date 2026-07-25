# BCL examples

`.bcl` files describe the artifacts a deployment created — the Box folder and
enterprise, the Salesforce org, and so on — so that `box-dispatch import` can
rebuild a runtime configuration from a real environment instead of hand-editing
one. This directory holds worked examples you can import as-is or copy and edit.

The IDs in these files are illustrative. Replace them with the object IDs from
your own tenant (folder ID, enterprise ID, Salesforce org ID).

## Two syntaxes

`box-dispatch` reads either of two on-disk syntaxes for the same document; both
decode to the same inventory, so pick whichever is easier to hand-edit.

- **JSON** — a plain `BCLDocument` object. See
  [`clm/salesforce-artifacts.bcl`](clm/salesforce-artifacts.bcl).
- **HCL locals** — the Terraform-style `locals { bcl = { … } }` form that
  `box-dispatch export` emits, optionally followed by `resource "null_resource"`
  blocks (the Terraform projection, ignored on import). See
  [`clm/box-artifacts.bcl`](clm/box-artifacts.bcl).

Every resource carries:

| field | where | meaning |
|-------|-------|---------|
| `provider` | resource | `box`, `salesforce-agentforce`, … |
| `type` | resource | artifact type: `box.folder`, `box.enterprise`, `salesforce.org` |
| `config.provider_object_id` | resource | the object's ID in the provider |
| `metadata.enterpriseId` | resource | Box enterprise ID (Box artifacts) |
| `metadata.artifactName` | resource | human label shown during import |
| `scenario` | document | target scenario, e.g. `clm` |

## Importing

Point `import` at a single file to bring in one provider's artifacts:

```sh
box-dispatch import examples/bcl/clm/salesforce-artifacts.bcl
```

Point it at a directory to aggregate every `*artifacts.bcl` file in it — here,
both Box and Salesforce become providers in one CLM runtime config:

```sh
box-dispatch import examples/bcl/clm
```

Import writes the active profile's runtime config; pass `--force` to overwrite an
existing one, and `--scenario <name>` to override the scenario recorded in the
files.
