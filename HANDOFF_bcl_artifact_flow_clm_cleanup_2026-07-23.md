# Handoff: box-dispatch BCL artifact flow + CLM cleanup

## Date / context
- Date: 2026-07-23
- Goal: complete shift to standardized `.bcl` as the single import/export artifact format, remove legacy JSON/Terraform migration support, and align CLM configs to BCL.
- Primary repos:
  - `/Users/massnerder/Developer/unofficialbox/box-dispatch`
  - `/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm`

## Completed flow work
- Removed legacy migration/import format support in box-dispatch.
- BCL is now the single artifact contract for both export and import paths.
- Standardized CLI/help/README language around BCL-only emit/import.
- CLM config files were converted from `*.json` to `*.bcl` for active configuration paths.
- Legacy archived JSON snapshot dir was emptied and removed.

## Flow file changes (`box-dispatch`)
### Artifact contract + CLI code
- [BCL_ARTIFACT_CONTRACT.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/BCL_ARTIFACT_CONTRACT.md)
  - Canonical contract for BCL emit and import behavior.
- [cmd/box-dispatch/import.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/cmd/box-dispatch/import.go)
  - `--format` now constrained to `bcl`.
  - Usage now references the single BCL contract for import.
- [internal/engine/import.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/engine/import.go)
  - Removed support paths for non-BCL imports.
  - Supports `.bcl` single file and `*artifacts.bcl` directory import paths only.
  - Returns `unsupported import format` for anything else.
- [README.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/README.md)
  - Updated section and linked to the unified BCL contract.
- [internal/engine/artifacts.go](/Users/massnerder/Developer/unofficialbox/box-dispatch/internal/engine/artifacts.go)
  - Defines BCL artifact emit path and write behavior (`<provider>-artifacts.bcl`).
- [BCL_ARTIFACT_CONTRACT_POINTER.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/BCL_ARTIFACT_CONTRACT_POINTER.md)
  - Compatibility pointer to the unified artifact contract.

## CLM conversion work in `box-bedrock-for-clm`
### Active config: `.json` to `.bcl`
- Legacy `.json` configs under `config/` were removed from active flow.
- New `.bcl` equivalents were added under the same directories.
- The old archive folder used for these conversions was cleaned:
  - deleted: `/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/archive/json-config-20260723T115520Z` (contents removed earlier)
  - `config/archive` was now empty and then removed.
- Representative added `.bcl` files:
  - [config/box/metadata-templates.bcl](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/box/metadata-templates.bcl)
  - [config/box/extract-field-prompts.bcl](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/box/extract-field-prompts.bcl)
  - [config/agentcore/tool-contracts.bcl](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/agentcore/tool-contracts.bcl)
  - [config/clm/redline-finding.schema.bcl](/Users/massnerder/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/clm/redline-finding.schema.bcl)
  - [config/runtime/demo-environment.example.bcl](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/runtime/demo-environment.example.bcl)

### Removed `.json` examples/fixtures in CLM config tree
- Deleted under active config area (no longer source of truth):
  - [config/agentcore/agent-handoff-payloads.json](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/agentcore/agent-handoff-payloads.json)
  - [config/box/form-definition.json](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/box/form-definition.json)
  - [config/demo/screenshot-manifest.json](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/demo/screenshot-manifest.json)
  - [config/runtime/validation-receipts.example.json](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/runtime/validation-receipts.example.json)
  - [config/salesforce/clm-contract-record.json](/Users/massnerder/Developer/Code/box-bedrock-agentcore-demos/box-bedrock-for-clm/config/salesforce/clm-contract-record.json)

## Current workspace state to be aware of
- `box-dispatch` still has many pre-existing/previously introduced uncommitted changes beyond the BCL import scope (including deleted runtime JSON examples, new generated parser files, and docs updates).
- `box-bedrock-for-clm` has extensive `.json` → `.bcl` changes and other touched docs/tests/scripts from prior conversion work.

## Next actions
1. Validate end-to-end export+import:
   - `box-dispatch resolve --scenario <name>` (or `bootstrap`) to emit `<provider>-artifacts.bcl`
   - `box-dispatch import <path-to-bcl-or-generated-folder> --force`
   - verify runtime state writes successfully.
2. Confirm there are no remaining runtime consumers that still expect `*.json` in CLM paths.
3. Decide if any non-test references should be updated to remove historical `demo-` naming (you already asked to remove technical-demo coupling).
4. If you commit, keep commit narrowly scoped to avoid mixing unrelated CLM docs/code churn from earlier work.

## CLM session notes
- This project currently has zero external migration requirement outside current CLM files, so BCL-first is now feasible.
- Admin UX target is now one artifact contract in box-dispatch: BCL import + BCL emitted artifacts.
- If any legacy JSON format parser needs to be restored later, it should be done intentionally and documented as a compatibility mode, not the default path.

## Artifact contract links
- [BCL_ARTIFACT_CONTRACT.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/BCL_ARTIFACT_CONTRACT.md)
- [BCL_ARTIFACT_CONTRACT_POINTER.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/BCL_ARTIFACT_CONTRACT_POINTER.md) (compatibility alias)

## Next-session checklist
1. Read this handoff file and [BCL_ARTIFACT_CONTRACT.md](/Users/massnerder/Developer/unofficialbox/box-dispatch/BCL_ARTIFACT_CONTRACT.md) start-to-finish before making changes.
2. Confirm `box-dispatch resolve`/`bootstrap` emits `<provider>-artifacts.bcl` and `box-dispatch import` accepts it (`--force` if runtime exists).
3. Verify CLM active config references point to `.bcl` files (no runtime `.json` expectations remain).
4. Scan docs/scripts for any user-facing mentions of deprecated `*.json` or `*.tf.json` imports and remove if found.
5. Keep scope narrow for commits; only include BCL/import-related changes unless explicitly expanding.

## Future Handoff Template

### Context
- Date:
- Active repo/path:
- Branch:
- Goal for next session:

### What changed
- [ ] Code changes:
- [ ] Config/docs updates:
- [ ] Cleanup/migrations performed:

### Open questions / risks
- [ ]
- [ ]

### Resume actions
1. [ ]
2. [ ]
3. [ ]

### Validation (if run)
- Command:
- Result:

### Commit strategy
- Scope boundaries:
- Files to include:
- Files to exclude:
