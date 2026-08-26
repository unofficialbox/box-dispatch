# Build and release helpers for the full Dispatch stack.

.PHONY: build web-build go-build clean

# Build both the React web UI and the Go backend into the local binary.
build: web-build go-build

web-build:
	cd web && bun install && bun run build

go-build:
	go build -o box-dispatch ./cmd/box-dispatch

clean:
	rm -f box-dispatch
