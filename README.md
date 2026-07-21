# windlass

`windlass` is the CLI for **box-windlass**, a set of building blocks and accelerators for Box-backed solution stacks (Box, Salesforce/Agentforce, Databricks, AWS Bedrock AgentCore).

This repo starts with a fast setup/check experience and expands toward scenario install and deployment workflows.

## First-time UX (FTUX)

1. `windlass check` (interactive by default)
   - discovers required platform tools
   - validates local credentials/config
   - validates live connectivity (unless `--offline`)
   - prints clear next commands for any disconnected provider
2. `windlass scenario --name <scenario>` (planned)
   - scaffold scenario-specific configuration for your selected architecture
3. `windlass deploy --scenario <name>` (planned)
   - apply infra and agent/runtime resources in order

## Interactive check experience

Run:

```bash
windlass check
```

- Uses a Bubble Tea terminal UI with a branded header and live progression.
- If a provider is not connected, `windlass` prints copy/paste-ready commands to connect it.
- Works in both:
  - `TTY` interactive mode (default)
  - `--json` machine output for scripts
- Can skip network calls with `--offline` while still validating required config.

Platform filters:

```bash
windlass check --platform box
windlass check --platform salesforce
windlass check --platform databricks
windlass check --platform aws
```

## Planned command surface

- `check` - validate dependencies and connectivity
- `scenario` - initialize project/pack configuration for a scenario (coming next)
- `deploy` - deploy prerequisites and services (coming next)

## Configuration

The checker looks for:

- **Box**: `BOX_ACCESS_TOKEN`
- **Salesforce**: `SF_ALIAS` (or `SALESFORCE_ACCESS_TOKEN`)
- **Databricks**: `DATABRICKS_HOST`, `DATABRICKS_TOKEN`
- **AWS**: `AWS_PROFILE`, `AWS_REGION`/`AWS_DEFAULT_REGION`

## Development

```bash
go run ./cmd/windlass check
go run ./cmd/windlass check --json
go run ./cmd/windlass check --platform aws --offline
```
