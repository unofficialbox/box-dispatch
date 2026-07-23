# BCL Artifact Contract (box-dispatch)

This document defines the contract for Box Configuration Language (BCL) artifacts used by `box-dispatch` as the single interchange format.

It covers:
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

## 2) Export contract (runtime -> BCL)

- Emitted at:
  - `resolve`: `<generatedDir>/<scenario>/<provider>-artifacts.bcl`
  - `bootstrap`: `<generatedDir>/<scenario>/<provider>-artifacts.bcl`
- Emission command path:
  - built by `writeArtifactBundle(...)` in [internal/engine/artifacts.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/engine/artifacts.go)
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

## 3) Import contract (BCL -> runtime config)

- Supported import entry point:
  - [cmd/box-dispatch/import.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/cmd/box-dispatch/import.go)
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

## 4) Operational contract

- There is no legacy `*.tf.json` or custom JSON migration path at import time.
- `*.bcl` is the single supported admin-facing interchange format.
- Import source is also the cleanup-ready artifact source for rehydrating environment state.

## 5) Relevant references

- [internal/bcl/bcl.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/bcl/bcl.go)
- [internal/engine/artifacts.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/engine/artifacts.go)
- [internal/engine/import.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/engine/import.go)
- [README.md import section](/Users/massnerder/Developer/unofficialbox/box-dispatch/README.md)
