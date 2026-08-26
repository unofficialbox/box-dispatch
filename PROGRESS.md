# Progress

## 2026-08-25

- ✅ Made the embedded browser workspace the only interactive Dispatch experience.
- ✅ Removed the Bubble Tea shell, animated check UI, terminal-only settings, and Charmbracelet dependencies.
- ✅ Kept plain flag-based commands with text or JSON output for automation.

## 2026-07-21

- ✅ Started repository as Go CLI project named `box-dispatch`.
- ✅ Added `cmd/box-dispatch/main.go` entrypoint with `check` command.
- ✅ Added interactive Bubble Tea check experience.
- ✅ Added live connectivity checks for Box, Salesforce, Databricks, and AWS.
- ✅ Added guidance output for failed provider connectivity.
- ✅ Added initial `README`, `CHANGELOG`, and `PROGRESS`.
- ✅ Wired command surface in `cmd/box-dispatch/main.go` for `init`, `setup`, `resolve`, `bootstrap` (and `deploy` alias), `status`, `scenarios`, `source`, `env`, `validate`, `present`, `smoke`, and `publish-check`.
- ✅ Added profile-aware runtime config resolution (`--profile` / `BOX_DISPATCH_PROFILE`) with state under `~/.config/box-dispatch/<profile>/`.

## Next

- Keep profile references clean: verify all engine calls route through profile-scoped paths.
- Keep command output deterministic and non-interactive.
