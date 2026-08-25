# box-dispatch

Box Dispatch is a browser-first workspace for configuring, validating, and deploying
Box-backed solution stacks. A single Go executable serves the embedded React application
and its loopback API; Go owns credentials, provider calls, package assembly, deployment,
and audit records.

## Run Dispatch

```bash
go run ./cmd/box-dispatch
```

With no subcommand, Dispatch listens on `127.0.0.1:8787` and opens the browser.
Use flags when a browser should not be opened or a different loopback port is required:

```bash
go run ./cmd/box-dispatch --no-open
go run ./cmd/box-dispatch --port 8790
```

The browser is the only interactive user experience. The executable has no full-screen
terminal mode, prompts, keyboard navigation, or terminal presentation dependencies.

## Mock backend and API recording

Run the complete embedded workspace against a deterministic in-memory API:

```bash
go run ./cmd/box-dispatch mock
go run ./cmd/box-dispatch mock --no-open --port 8788
```

The mock does not read credentials, clone packages, or call providers.

A live successful workflow can be recorded as credential-redacted JSON Lines:

```bash
go run ./cmd/box-dispatch --record-api .dispatch/recordings/clm-success.jsonl
```

## Build

Build the React application and Go executable:

```bash
cd web && bun install && bun run build
cd .. && go build -o box-dispatch ./cmd/box-dispatch
```

## Plain command interface

Subcommands are conventional, non-interactive commands. They accept flags and write plain
text, or JSON when `--json` is supplied.

```bash
box-dispatch check --offline
box-dispatch check --platform box --json
box-dispatch status --scenario clm
box-dispatch deploy --scenario clm --yes
box-dispatch validate --scenario clm --skip-artifacts
box-dispatch serve --no-open --port 8787
```

Common commands:

- `check` validates configuration and connectivity.
- `deploy` applies resolved artifacts and provider configuration; `bootstrap` remains an alias.
- `status` reports scenario state and unresolved values.

Authoring and diagnostic commands remain available for scripts: `init`, `setup`,
`resolve`, `source`, `scenarios`, `env`, `import`, `validate`, `present`,
`smoke`, and `publish-check`.

Destructive reset is not exposed as a terminal command. It belongs in the web workflow,
where Dispatch can show the audited resources and require an explicit confirmation.

## Configuration

Use `--profile <name>`, `BOX_DISPATCH_PROFILE=<name>`, or the default profile.
Provider values are documented in [`.env.sample`](.env.sample). Real `.env` files and
tokens must never be committed.

Configuration and deployment-plan state are stored locally. The browser receives no
provider credentials or raw provider responses.

## Browser workflow

The web workspace guides operators through:

1. Choose a supported solution and provider set.
2. Connect and verify Box and Salesforce.
3. Configure deployment strategy and supported components.
4. Validate provider state and deployment prerequisites.
5. Apply the validated deployment and review the audit result.

Databricks and Amazon Bedrock AgentCore remain visible as unavailable roadmap items until
their supported lifecycle is complete.

## Security boundaries

- The browser receives no provider credentials or raw provider responses.
- The Go service remains loopback-only and returns `Cache-Control: no-store`.
- Deployment and cleanup use recorded immutable resource IDs.
- Unsupported/private provider APIs are excluded and documented in
  [`docs/PUBLIC_API_GAPS.md`](docs/PUBLIC_API_GAPS.md).
- Generated solution packages use BCL as the portable artifact contract; see
  [`BCL_ARTIFACT_CONTRACT.md`](BCL_ARTIFACT_CONTRACT.md).

Architecture and endpoint details are documented in
[`docs/WEB_APP_ARCHITECTURE.md`](docs/WEB_APP_ARCHITECTURE.md).

## Verification

Before committing:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
cd web && bun run lint && bun run test && bun run build && bunx playwright test
```
