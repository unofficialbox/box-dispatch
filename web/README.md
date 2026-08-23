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

Choose **New deployment** to select a configured quickstart and the supported
providers it needs. The browser sends only those selections to the loopback Go
service; it resolves the repository, creates the local workspace, and saves the
resulting BCL plan. A separate **Apply validated changes** action is still
required before Dispatch calls any provider deploy adapter.

## Verify

```bash
bun run build
bun run lint
```

The architecture and API migration sequence are in
[`../docs/WEB_APP_ARCHITECTURE.md`](../docs/WEB_APP_ARCHITECTURE.md).
