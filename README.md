# box-dispatch

<p align="center">
  <img src="docs/landing.png" alt="box-dispatch interactive launch shell — the welcome screen with the BOX DISPATCH wordmark, deployment menu, and the SELECT STACK › CONNECT › PICK QUICKSTART › SHIP route" width="900">
</p>

`box-dispatch` is the CLI for **box-dispatch**, a set of building blocks and accelerators for Box-backed solution stacks (Box, Salesforce/Agentforce, Databricks, AWS Bedrock AgentCore).

This repo starts with a fast setup/check experience and expands toward scenario install and deployment workflows.

## Launch behavior

Running `box-dispatch` with no subcommand starts the full-screen solution launch shell when
stdout is an interactive terminal:

```bash
box-dispatch
```

The shell packages, validates, deploys, audits, and resets solution stacks. When stdout is
redirected or piped, the same no-subcommand invocation prints the plain connectivity report
instead of starting the full-screen UI. Passing `--json` also selects machine-readable
connectivity output.

Use the explicit `check` command when connectivity validation is the intended operation:

```bash
box-dispatch check
box-dispatch check --offline
box-dispatch check --json
```

`box-dispatch check` uses its interactive progress UI in a TTY, `--offline` skips live
provider calls, and `--json` is suitable for scripts and redirected output.

## First-time UX (FTUX)

1. `box-dispatch check` (interactive by default)
   - discovers required platform tools
   - validates local credentials/config
   - validates live connectivity (unless `--offline`)
   - prints clear next commands for any disconnected provider
2. `box-dispatch init --source-url <git-url>`
   - pulls scenario source and copies example runtime config
3. `box-dispatch setup`
   - creates the local runtime config if it does not exist
4. `box-dispatch resolve --scenario <name>`
   - resolves token-backed provider config and writes manifests
5. `box-dispatch bootstrap --scenario <name> --yes` (alias: `deploy`)
   - applies provider bootstrap steps and emits state

## Interactive check experience

Run:

```bash
box-dispatch check
```

- Uses a Bubble Tea terminal UI with a branded header and live progression.
- If a provider is not connected, `box-dispatch` prints copy/paste-ready commands to connect it.
- Works in both:
  - `TTY` interactive mode (default)
  - `--json` machine output for scripts
- Can skip network calls with `--offline` while still validating required config.

Platform filters:

```bash
box-dispatch check --platform box
box-dispatch check --platform salesforce
box-dispatch check --platform databricks
box-dispatch check --platform aws
```

## Command surface

- `check` - validate dependencies and connectivity
- `init` - initialize scenario source and example config
- `setup` - create runtime config baseline
- `resolve` - resolve templates and generate provider artifacts
- `bootstrap` - apply bootstrap steps and persist state
- `deploy` - alias for `bootstrap`
- `status` - report unresolved environment and scenario health
- `scenarios` - list configured scenarios
- `source` - show scenario source metadata
- `env` - render environment token mapping for a scenario
- `validate` - validate resolved artifacts and state
- `present` - generate presenter handoff notes
- `smoke` - run lightweight smoke checks
- `publish-check` - validate blockers before handoff
- `import` - import a deployment manifest into runtime config

## Configuration

Runtime configuration is isolated by profile. Use either:

- `--profile <name>` on CLI commands
- `BOX_DISPATCH_PROFILE=<name>` environment variable
- or `default` profile when neither is set

Profile state is stored under:

- `~/.config/box-dispatch/<profile>/environment.json`
- `~/.config/box-dispatch/<profile>/environment.example.bcl`
- `~/.config/box-dispatch/<profile>/validation-receipts.json`
- `~/.config/box-dispatch/<profile>/bootstrap-state.json`
- `~/.config/box-dispatch/<profile>/source-state.json`

If the new profile layout is unavailable, runtime config still falls back to repository
`config/runtime/environment.example.bcl` when initializing from existing scenario
repositories for compatibility.

The checker looks for:

- **Box**: `BOX_ACCESS_TOKEN`
- **Salesforce**: `SF_ALIAS` (or `SALESFORCE_ACCESS_TOKEN`)
- **Databricks**: `DATABRICKS_HOST`, `DATABRICKS_TOKEN`
- **AWS**: `AWS_PROFILE`, `AWS_REGION`/`AWS_DEFAULT_REGION`

For cleanup-safe teardown, Box Dispatch also captures deployment IDs when present:

- **Box**: `BOX_FOLDER_ID`, `BOX_ENTERPRISE_ID`
- **Salesforce**: `SF_ORG_ID` (optional)

When provided, these IDs are written to `<provider>-artifacts.bcl` as the single emitted artifact bundle for import and cleanup.
- `<provider>-artifacts.bcl` is Box Configuration Language in Terraform-like syntax.
- It includes Box Dispatch-specific extension metadata under `locals` (`bcl.ext.box`) so cleanup tooling can stay provider-aware without extra convention guessing.
Files are emitted during both `resolve` and `bootstrap` so they can feed cleanup tooling.

## Standardized import

See the artifact contract for full import/export rules: [BCL_ARTIFACT_CONTRACT.md](BCL_ARTIFACT_CONTRACT.md)

Use `box-dispatch import` as the single admin-facing entry point for loading deployment details:

```bash
box-dispatch import <path>
```

- Supported standard inputs:
  - `<path>.bcl` (Box Configuration Language with `locals.bcl` metadata)
  - `<path>/*-artifacts.bcl` (directory import)
Examples:

```bash
box-dispatch import ./generated/box-artifacts.bcl
box-dispatch import ./generated --force
```

## Development

```bash
go run ./cmd/box-dispatch check
go run ./cmd/box-dispatch check --json
go run ./cmd/box-dispatch check --platform aws --offline
go run ./cmd/box-dispatch init --source-url <git-url>
go run ./cmd/box-dispatch setup
go run ./cmd/box-dispatch resolve --scenario <name> --dry-run
go run ./cmd/box-dispatch bootstrap --scenario <name> --yes
```
