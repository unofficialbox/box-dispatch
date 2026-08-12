# box-dispatch

<p align="center">
  <img src="docs/landing.png" alt="box-dispatch interactive launch shell — the welcome screen with the BOX DISPATCH wordmark and five-stage Choose, Connect, Configure, Review, Deploy route" width="900">
</p>

`box-dispatch` is the CLI for **box-dispatch**, a set of building blocks and accelerators for Box-backed solution stacks (Box, Salesforce, Databricks, AWS Bedrock AgentCore).

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

### Interactive deployment workflow

The default shell presents one five-stage path:

1. **Choose** — select a solution quickstart and its providers together.
2. **Connect** — verify access only for those providers.
3. **Configure** — choose the destination, deployment strategy, and supported Box capabilities.
   **Reuse existing** is selected by default; switch to **Create new** when the deployment needs
   a uniquely named workspace.
4. **Review** — preview the complete plan before anything is created or changed.
5. **Deploy** — assemble the package, validate provider state and prerequisites, install required
   managed packages, deploy supported missing configuration, and verify permission assignments.

Databricks and AWS Bedrock AgentCore remain visible in **Choose** as dimmed **Coming soon** roadmap
items. They are read-only and cannot be added to the active provider plan yet.

Packaging and validation remain visible inside **Deploy**; they are not separate navigation
destinations. The active provider carries the progress animation while completed providers
collapse to one-line results. Press `v` to switch between the focused summary and full provider
checklists. Reset is available from **Deployment history**, where Dispatch shows the recorded
resources and requires explicit confirmation before removing anything.

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
5. `box-dispatch deploy --scenario <name> --yes` (`bootstrap` remains a compatibility alias)
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

Default help keeps the normal operator path short:

- `check` - validate dependencies and connectivity
- `deploy` - apply resolved artifacts and provider configuration (`bootstrap` remains an alias)
- `status` - report unresolved environment and scenario health
- `reset` - interactively preview and reset a recorded deployment by audited resource ID

Advanced help contains the authoring, diagnostic, and presentation commands: `init`, `setup`,
`resolve`, `source`, `scenarios`, `env`, `import`, `validate`, `present`, `smoke`, and
`publish-check`. Existing command names remain available for compatibility.

`--offline` consistently means “skip live connectivity checks” on connectivity commands.
Validation uses the explicit `--skip-artifacts` flag; its former `--offline` spelling remains
as a hidden deprecated alias for existing scripts.

The staged plan for simplifying the interactive flow and default command help is tracked in
[`docs/EXECPLAN_CLI_UX_STREAMLINING.md`](docs/EXECPLAN_CLI_UX_STREAMLINING.md).

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

The interactive launch shell also keeps project-local presentation settings in
`.dispatch/ui-settings.bcl`. Its `metadata.boxComponentVisibility` map controls which Box
capability catalog rows appear on **Configure Box components**. Supported capabilities
default visible; capabilities with incomplete or missing public APIs default hidden. Making
an unsupported capability visible only shows a locked reference row—it cannot be packaged
or deployed. See [`docs/PUBLIC_API_GAPS.md`](docs/PUBLIC_API_GAPS.md) and the tracked
[`config/runtime/ui-settings.example.bcl`](config/runtime/ui-settings.example.bcl).

Press `?` or `F1` anywhere outside an active text field to open the expanded keyboard and
accessibility help. Set `metadata.accessibleForms` to `true` in `.dispatch/ui-settings.bcl` to
use Huh's screen-reader-oriented form prompts. Set the standard `NO_COLOR` environment variable
or pass `--no-color` to disable ANSI colors. `TERM=dumb` also disables color and selects the
plain, non-full-screen command output.

### Salesforce org lifecycle safety

The launch shell treats Salesforce org health as part of connectivity, not as a one-time
login result. It inspects the selected org at **Connect**, immediately before **Validate**,
and immediately before **Deploy**. A cached Salesforce CLI profile for a deleted, expired,
or otherwise inactive scratch org is rejected before any metadata request is sent.

**Connect > Salesforce** presents only the next useful choices: continue with the connected
org, use an existing Salesforce org, or create/replace a 30-day scratch org. Choosing a local
Salesforce CLI profile validates it immediately; the same screen can open Salesforce login
for a new profile. Press `r` to recheck the current org without adding another menu action.

Scratch-org creation discovers authenticated Dev Hubs and asks which one to use. If none are
available, Dispatch opens Dev Hub authentication inside the scratch-org flow, then resumes
discovery. The selected Dev Hub is passed explicitly to Salesforce CLI, so a CLI-wide default
is not required. Creation is confirmation-gated and the selected Dev Hub plus generated alias,
org ID, org type, status, and expiration date are persisted in
`.dispatch/connection-settings.bcl`; no Salesforce access token is stored.
Scratch-org creation itself remains provider-neutral. During solution validation, Dispatch
checks the managed-package versions declared in `dispatch.bcl` and shows missing packages as
explicit deployment prerequisites. After the operator confirms **Deploy**, Dispatch installs
or upgrades each pinned `04t` package version non-interactively before sending solution
metadata. Dispatch sends only the metadata components validation found missing; it does not
resend existing org metadata merely to finish prerequisites or permission-set assignments.
For source-tracked orgs, validation previews the missing selectors and treats conflicts as
existing tenant configuration rather than silently overwriting them.
The bundled CLM contract requires Box for Salesforce 5.43.0.1
(`04tKi000000gPNZIA2`) with admin-only access. The same contract declares the five
required Box and CLM permission sets. Validation checks their assignment on the
authenticated **System Administrator**; after metadata deployment makes local permission
sets available, Dispatch assigns only the missing sets and verifies the result. Existing
manual assignments are reported as present and are not recreated.

After a deployment, press `b` to open the Box Admin Console for the connected enterprise.
When the selected Salesforce target is a scratch org, press `s` to open it through Salesforce
CLI. The Box browser session determines the enterprise shown; Dispatch includes the connected
EID in the action label so the operator can confirm the target. The completion screen defaults to
created, existing, and failed counts; press `v` to inspect the deployed-resource table and full
credential-free audit path.

Normal failures show a concise remediation message. Press `d` when offered to open the full
Salesforce CLI JSON diagnostic, including `stack` and `error.data`; token-, secret-, password-,
and session-shaped fields are redacted.

```mermaid
flowchart LR
    A["Connect Salesforce"] --> B["Inspect org status and expiration"]
    B -->|"active"| C["Persist non-secret lifecycle metadata in BCL"]
    B -->|"deleted or expired"| D["Block metadata work and offer replacement"]
    D --> E["Discover and choose authenticated Dev Hub"]
    E --> F1["Confirm 30-day scratch-org creation"]
    F1 --> B
    C --> F["Preflight again before Validate"]
    F --> G["Inventory required managed-package versions"]
    G --> H1["Confirm Deploy"]
    H1 --> I["Install missing pinned packages"]
    I --> J["Send solution metadata"]
    J --> K["Assign and verify required permission sets"]
    B --> H["d: full sanitized CLI diagnostics"]
```

Generated solution packages use BCL for their complete package contract:

- `dispatch.bcl` — template, workspace, sample-content, capability, managed-package and
  permission-set prerequisites, and deployment-order definition
- `.dispatch/deployment.bcl` — component selection, naming strategy, and rollback settings

New deployment settings default to `reuse`, so matching resources are reused rather than copied
under a run-specific name. Select `create_new` explicitly when isolation is required.

JSON solution manifests and deployment settings are unsupported. An invalid `dispatch.bcl`
fails explicitly, and `deployment_config` must reference a package-relative `.bcl` file.
Packaging removes obsolete `dispatch.json` and `.dispatch/deployment.json` files if they are
encountered in a cloned template. See
[`BCL_ARTIFACT_CONTRACT.md`](BCL_ARTIFACT_CONTRACT.md#2-solution-configuration-contract).

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
go run ./cmd/box-dispatch deploy --scenario <name> --yes
```
