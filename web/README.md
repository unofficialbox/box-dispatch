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

The web flow validates an already assembled package first. A separate **Apply
validated changes** action is required before Dispatch calls any provider deploy
adapter.

## Verify

```bash
bun run build
bun run lint
```

The architecture and API migration sequence are in
[`../docs/WEB_APP_ARCHITECTURE.md`](../docs/WEB_APP_ARCHITECTURE.md).
