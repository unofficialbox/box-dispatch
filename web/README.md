# Box Dispatch web app

The browser-native Dispatch experience. It uses React 19, Vite, TypeScript, and
the published `@unofficialbox/box-open-elements` Web Components.

## Run locally

```bash
bun install
# In a second terminal, from the repository root:
go run ./cmd/box-dispatch serve
bun dev
```

The browser app proxies `/api` to `http://127.0.0.1:8787`. When no local API is
running, the initial screen uses credential-free demonstration state.

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
