# BCL Configuration and Artifact Contract (box-dispatch)

This document defines the contract for Box Configuration Language (BCL) documents used by
`box-dispatch` for solution configuration, local shell state, and deployment artifacts.

It covers:
- Solution template and package configuration
- Project-local shell and deployment settings
- Export/emit behavior (`resolve`, `bootstrap`) from provider state into BCL
- Import behavior (`box-dispatch import`) from BCL back into runtime config

## 1) Canonical format

- Supported format: `*.bcl`
- File forms accepted by import:
  - a single `<file>.bcl`
  - a directory containing one or more `*artifacts.bcl` files
- Parser: `bcl.LoadBCL`
  - canonical JSON payload support
  - HCL-like BCL inventory payload under `locals { bcl = { ... } }`

Every typed document uses `provider = "box-dispatch"` or its actual provider and a distinct
`context`. A context selects the schema; changing a file extension to `.bcl` is not enough.

## 2) Solution configuration contract

New solution packages use:

- `dispatch.bcl`
  - `context = "solution-manifest"`
  - `metadata.manifest` contains the typed template ID, deployment configuration reference,
    Box workspace, sample content, capability catalog, and deployment order
- `.dispatch/deployment.bcl`
  - `context = "deployment-settings"`
  - `metadata.settings` contains naming, rollback, strategy, and component selection
  - `metadata.boxComponentVisibility` controls presentation only and cannot grant deployment
    support

The bundled CLM source is
[`internal/solution/manifests/clm.bcl`](internal/solution/manifests/clm.bcl). Package builders
emit the canonical HCL-like form through `bcl.WriteBCL`.

Format rules:

- `dispatch.bcl` is the only solution-manifest format.
- `deployment_config` must reference a package-relative `.bcl` file.
- Invalid BCL fails explicitly; there is no JSON fallback.
- Packaging removes obsolete `dispatch.json` and `.dispatch/deployment.json` files if a
  cloned template still contains them.

## 3) Export contract (runtime -> BCL)

- Emitted at:
  - `resolve`: `<generatedDir>/<scenario>/<provider>-artifacts.bcl`
  - `bootstrap`: `<generatedDir>/<scenario>/<provider>-artifacts.bcl`
- Emission command path:
  - built by `writeArtifactBundle(...)` in [internal/engine/artifacts.go](internal/engine/artifacts.go)
  - file format via `bcl.WriteBCL(...)`
- Artifacts currently emitted:
  - `box`: `box.folder`, `box.enterprise` when IDs exist in env
  - `salesforce-agentforce`: `salesforce.org` when org ID exists in env
- BCL document fields:
  - top-level metadata: `version`, `scenario`, `provider`, `context`, `generatedAt`, `metadata`, `resources`, `ext`
  - each artifact is represented in `resources` with:
    - `provider`
    - `type` (artifact type)
    - `provider_object_id`
    - `artifact_type`
    - `enterprise_id` (where applicable)
    - `artifact_name`
- `ext` includes Box extension metadata under `bcl.ext.box` when Box artifacts are present:
  - `version`
  - `artifacts` with `name`, `artifactType`, `providerObjectId`, `enterpriseId`

## 4) Import contract (BCL -> runtime config)

- Supported import entry point:
  - [cmd/box-dispatch/import.go](cmd/box-dispatch/import.go)
  - format is validated as `bcl` only
- `internal/engine/import.go` performs:
  - file path check (`*.bcl`) or directory scan (`*artifacts.bcl`)
  - extraction via `bcl.ExtractArtifactsFromBCL(...)`
  - transform to runtime config via `buildConfigFromArtifacts(...)`
- Artifact extraction rules (from `bcl.ExtractArtifactsFromBCL`):
  - resource must include non-empty `provider` and `type`
  - `providerObjectId` required from:
    - `config.provider_object_id` or `metadata.providerObjectId`
  - duplicates suppressed by `(provider, artifactType, providerObjectId)`
  - optional fields:
    - `scenario`: from `metadata.scenario` or top-level BCL `scenario`
    - `enterpriseId`: from `metadata.enterpriseId` or `config.enterprise_id`
    - `artifactName`: from `metadata.artifactName` or `config.artifact_name` or resource name
    - `createdAt`
- Built runtime config includes:
  - scenario resolution (`--scenario` > BCL scenario > first artifact scenario > `clm`)
  - `Scenarios` entry with provider order
  - provider `env` mapping:
    - `BOX_FOLDER_ID` from `box.folder`
    - `BOX_ENTERPRISE_ID` from `box.enterprise`
    - `SF_ORG_ID` from `salesforce.org`
    - required tokens kept as placeholders when absent
- Import fails if:
  - no runtime config path exists and `--force` is not set
  - file is not `.bcl`
  - directory has zero importable `*artifacts.bcl` resources
  - parsed artifacts are empty

## 5) Operational contract

- Artifact import has no `*.tf.json` or custom JSON path.
- `*.bcl` is the single supported admin-facing interchange format.
- Import source is also the cleanup-ready artifact source for rehydrating environment state.

## 6) Relevant references

- [internal/bcl/bcl.go](internal/bcl/bcl.go)
- [internal/solution/manifest.go](internal/solution/manifest.go)
- [internal/solution/manifests/clm.bcl](internal/solution/manifests/clm.bcl)
- [internal/engine/artifacts.go](internal/engine/artifacts.go)
- [internal/engine/import.go](internal/engine/import.go)
- [README.md import section](README.md#standardized-import)
