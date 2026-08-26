# Box Dispatch web app

The browser-native Dispatch experience. It uses React 19, Vite, TypeScript, and
the published `@unofficialbox/box-open-elements` Web Components.

## Run locally

Normal use requires only the compiled executable:

```bash
box-dispatch
```

It starts the local Go service, serves the bundled browser interface, and opens the app. Vite
is only needed for frontend hot-reload development:

```bash
bun install
# From the repository root, keep the complete app service running without opening another tab:
go run ./cmd/box-dispatch serve --no-open
bun dev
```

The Vite development server proxies `/api` to `http://127.0.0.1:8787`. Production builds are
written to `internal/webui/dist` and embedded in the Go executable.

### Frontend refinement without providers

Start the deterministic in-memory API from the repository root. It never reads
provider credentials, clones a package, or calls Box or Salesforce:

```bash
go run ./cmd/box-dispatch mock --no-open
```

Then start Vite from `web/` with the mock proxy target:

```bash
bun run dev:mock
```

The complete Choose → Connect → Configure → Validate → Deploy flow is available
at the Vite URL. The same mock powers `bun run test:e2e`.

To refine the failed-validation UI without expiring a real provider session, run a
separate mock instance with an authentication failure:

```bash
go run ./cmd/box-dispatch mock --no-open --port 8790 --fail-validation-provider salesforce
```

### Record a successful browser/API workflow

The live server can write every browser API request and response—including the
completed server-sent event streams—to an owner-only JSON Lines file:

```bash
./box-dispatch --record-api .dispatch/recordings/clm-success.jsonl
```

Credential fields and OAuth authorization URLs are replaced with
`[REDACTED]`; callback query strings are never recorded. Stop the server after
the successful deployment to close the recording.

Choose **New deployment** to select a configured quickstart and the supported
providers it needs. The browser sends only those selections to the loopback Go
service; it resolves the repository, creates the local workspace, and saves the
resulting BCL plan. A separate **Apply validated changes** action is still
required before Dispatch calls any provider deploy adapter.

## Verify

```bash
bun run build
bun run lint
bun run test:e2e
```

The architecture and API migration sequence are in
[`../docs/WEB_APP_ARCHITECTURE.md`](../docs/WEB_APP_ARCHITECTURE.md).
