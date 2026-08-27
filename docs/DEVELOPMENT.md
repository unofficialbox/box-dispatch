# Development and verification

This guide covers local development, the deterministic mock backend, API recording, and
the checks required before a change is committed.

## Build each application layer

The normal build compiles the React application into `internal/webui/dist` before building
the Go executable:

```bash
make build
```

To build each layer directly:

```bash
cd web
bun install
bun run build
cd ..
go build -o box-dispatch ./cmd/box-dispatch
```

The hashed assets under `internal/webui/dist/assets/` are generated output and must be
committed with the source changes that produced them.

## Frontend development

Run the Go application on `127.0.0.1:8787`, then start Vite in another terminal:

```bash
cd web
bun install
bun run dev
```

Vite proxies local API requests to Dispatch. Keep credentials and provider operations in
the Go service; browser code consumes only sanitized local API responses.

## Mock backend

Run the complete embedded workspace against a deterministic in-memory API:

```bash
go run ./cmd/box-dispatch mock
go run ./cmd/box-dispatch mock --no-open --port 8788
go run ./cmd/box-dispatch mock --no-open --port 8788 --fail-connection-provider salesforce
```

The mock does not read credentials, clone packages, or call providers. Use
`--fail-connection-provider` or `--fail-validation-provider` with `box` or `salesforce`
to exercise recovery and diagnostic states.

For UI-only development against the mock service:

```bash
go run ./cmd/box-dispatch mock --no-open --port 8788
cd web
bun run dev:mock
```

## API recording

A live successful workflow can be recorded as credential-redacted JSON Lines:

```bash
go run ./cmd/box-dispatch --record-api .dispatch/recordings/clm-success.jsonl
```

Review recordings before sharing them even though the recorder removes credential-shaped
fields and callback query strings.

## Verification gate

Run the Go gate from the repository root:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```

Any path printed by `gofmt -l .` is a failure. Format that file with `gofmt -w` and rerun
the complete gate.

Run the frontend gate from `web/`:

```bash
bun run lint
bun run test
bun run build
bun run test:e2e
```

The Playwright command rebuilds the embedded frontend and runs the full mock-workflow suite.
