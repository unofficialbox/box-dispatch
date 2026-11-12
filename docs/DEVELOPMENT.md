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

## Large live folder comparison fixture and targets

Folder comparison is bounded by contract rather than by browser memory:

- lazy root browsing returns at most 500 child folders per request;
- live folder search returns at most 50 matches and can reach unexpanded branches;
- a comparison returns at most 500 folders and 1,000 files or web links per side,
  to a maximum depth of 4 beneath each selected root;
- hitting any comparison bound must render the result as **partial**; it must never
  look complete by omission;
- Cancel aborts the browser request and both Go provider contexts. No later page of
  either traversal may continue after cancellation is observed.

`TestCrawlFolderTreeLargeFixtureIsExplicitlyPartial` generates 600 folders and 1,200
files in memory and verifies both caps and both partial flags. The Compare Playwright
flow verifies lazy root loading, a live search result outside the visible branch,
selected-root request IDs, cancellation, explicit partial presentation, recovery,
and narrow-screen overflow. The folder-promotion Playwright flow selects a portable
node, verifies the exact promotion request, and carries its safe path/type preview through
validation, deployment summary, and history. Go tests cover stale selection rejection,
server-only package payloads, stable created IDs, path confinement, and ID-only reset.
The deterministic mock roots are:

| Connection | Root folders | Nested folder |
| --- | --- | --- |
| Acme Production | Contracts, Legal | Contracts / Archive |
| Acme Sandbox | Contracts, Operations | Contracts / Templates |

For a live baseline, record the root-child response time, a selected-root comparison
duration, and cancellation behavior without changing either Box enterprise. Provider
latency is not treated as a fixed SLA; payload bounds, explicit partial state, and prompt
cancellation are the enforceable performance protections.

Live promotion verification must use disposable QA folders created for the test. Re-run
the live comparison immediately before preparation, validate before applying, confirm the
audit record contains only created resource IDs, and use reset to remove those exact IDs.
Never use a production folder or folder ID `0` as a teardown target.

## GitHub Actions CI

Pull requests targeting `main` and pushes to `main` run three independent checks:

- **Go gate** — formatting, build, vet, and the hermetic Go suite;
- **Frontend** — frozen Bun install, unit tests, lint, and production build;
- **Playwright** — frozen Bun install, Chromium installation, and the complete
  provider-free mock browser suite.

The workflow has read-only repository permissions, cancels superseded runs for the same
branch, and uploads Playwright evidence only when the browser job fails. It does not read
provider credentials or call live Box or Salesforce environments.
