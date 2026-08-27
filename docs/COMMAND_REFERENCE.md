# Command reference

Dispatch is browser-first, but it also provides conventional non-interactive commands for
automation, diagnostics, and provider lifecycle work. Commands accept flags, write plain
text by default, and use JSON when `--json` is supplied.

## Common commands

```bash
box-dispatch check --offline
box-dispatch check --platform box --json
box-dispatch status --scenario clm
box-dispatch deploy --scenario clm --yes
box-dispatch validate --scenario clm --skip-artifacts
box-dispatch serve --no-open --port 8787
```

- `check` validates configuration and connectivity.
- `deploy` applies resolved artifacts and provider configuration; `bootstrap` remains an alias.
- `status` reports scenario state and unresolved values.
- `serve` starts the embedded browser workspace and loopback API.

## Authoring and diagnostics

Additional commands remain available for scripts and repository maintenance:

- `init`
- `setup`
- `resolve`
- `source`
- `scenarios`
- `env`
- `import`
- `validate`
- `present`
- `smoke`
- `publish-check`

Use `<command> --help` for its flags and examples.

## Runtime profiles

Use `--profile <name>`, set `BOX_DISPATCH_PROFILE=<name>`, or omit both to use the default
profile. Provider environment variables are documented in [`.env.sample`](../.env.sample).

Destructive reset is not exposed as a terminal command. It belongs in the web workflow,
where Dispatch can show audited resources and require explicit confirmation.
