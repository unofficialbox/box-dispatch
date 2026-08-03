---
name: verify-build
description: Run the box-dispatch Go verification gate (gofmt, build, vet, test) before committing. Use after any Go change, before every commit/push/merge, or when asked to "verify", "check the build", or "make sure it's green".
---

# verify-build

The standing quality gate for the `box-dispatch` Go module. Run it after any change to
`*.go` and **before every commit, push, or merge**. All four steps must be clean before
you report the work done. This module targets `go 1.26` (see `go.mod`).

## The gate (run in order, from the repo root)

```bash
gofmt -l .          # lists files that need formatting; empty output = clean
go build ./...      # must exit 0
go vet ./...        # must exit 0, no diagnostics
go test ./...       # all packages ok / no-test-files
```

One-liner for a fast pass/fail:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./...
```

## Rules

- **`gofmt -l .` printing any path is a failure**, not a warning. Fix it before continuing:
  ```bash
  gofmt -w <file.go>     # or: gofmt -w .
  ```
  Two gofmt slips (struct-field alignment, trailing newline) have shipped here before —
  never skip this step.
- Do not use `go fmt ./...` as the check; `gofmt -l .` is the authoritative lister.
- If `go test` fails, report the actual failing package and output — never describe the
  suite as green when it isn't.
- Tests are hermetic (no live provider calls). If a test wants network/credentials,
  that's a bug in the test, not a reason to skip the gate.

## What "done" means

Report the work complete only after all four steps pass. When you commit, the commit
message should reflect that the gate is green. If any step fails and you can't fix it,
surface the exact output and stop — don't push a red tree.
