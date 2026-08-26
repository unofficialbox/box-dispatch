# Changelog

## Unreleased

- Removed the legacy full-screen terminal workflow and all Charmbracelet dependencies.
- Connectivity and deployment subcommands now use flags with plain text or JSON output only.
- Moved all interactive configuration, validation, deployment, and destructive confirmation to the web workspace.

## 0.1.0

- Added Go CLI bootstrap and command scaffolding for `box-dispatch`.
- Added the original interactive dependency and connectivity check (removed in Unreleased).
- Added per-provider check flow for:
  - Box (`BOX_ACCESS_TOKEN`)
  - Salesforce (Salesforce CLI + org display check)
  - Databricks (`DATABRICKS_HOST`, `DATABRICKS_TOKEN`)
  - AWS (`aws` CLI + STS identity)
- Added guidance engine that prints concrete remediation steps when a provider is missing config or not connected.
- Added JSON output mode for scripted use (`box-dispatch check --json`).
- Added command surface expansion for `init`, `setup`, `resolve`, `bootstrap`/`deploy`, `status`, `source`, `env`, `validate`, `present`, `smoke`, and `publish-check`.
