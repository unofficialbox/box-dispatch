# Command reference

Dispatch is browser-first, but it also provides conventional non-interactive commands for
automation, diagnostics, and provider lifecycle work. Commands accept flags, write plain
text by default, and use JSON when `--json` is supplied.

## Common commands

| Command | Description | Example Usage |
|---------|-------------|---------------|
| `check` | Validate tool, config, and connectivity prerequisites | `box-dispatch check --offline`<br>`box-dispatch check --platform box --json` |
| `deploy` | Deploy resolved artifacts and provider configuration | `box-dispatch deploy --scenario clm --yes` |
| `status` | Show status and unresolved tokens for the selected scenario | `box-dispatch status --scenario clm` |
| `serve` | Serve the Dispatch web application | `box-dispatch serve --no-open --port 8787` |
| `validate` | Validate bootstrap artifacts and deployment state | `box-dispatch validate --scenario clm --skip-artifacts` |

## Advanced commands

| Command | Description |
|---------|-------------|
| `completion` | Generate the autocompletion script for the specified shell |
| `env` | Render required environment token state for a scenario |
| `help` | Help about any command |
| `import` | Import deployment manifest into the active profile |
| `init` | Initialize local Box Dispatch runtime from a scenario source repository |
| `mock` | Serve the browser workspace with an in-memory mock API |
| `present` | Generate a presenter handoff note for the active scenario |
| `publish-check` | Verify deploy blockers before publish handoff |
| `resolve` | Resolve provider configuration and materialize scenario artifacts |
| `scenarios` | List configured scenarios from runtime config |
| `setup` | Bootstrap runtime config from example template |
| `smoke` | Run smoke checks for the selected scenario |
| `source` | Show metadata about the currently configured scenario source |

Use `box-dispatch <command> --help` for detailed flags and examples.

## Runtime profiles

Use `--profile <name>`, set `BOX_DISPATCH_PROFILE=<name>`, or omit both to use the default
profile. Provider environment variables are documented in [`.env.sample`](../.env.sample).

Destructive reset is not exposed as a terminal command. It belongs in the web workflow,
where Dispatch can show audited resources and require explicit confirmation.
