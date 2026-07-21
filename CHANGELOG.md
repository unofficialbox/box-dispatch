# Changelog

## 0.1.0

- Added Go CLI bootstrap and command scaffolding for `windlass`.
- Added interactive dependency + connectivity check with Charm Bracelet Bubble Tea.
- Added per-provider check flow for:
  - Box (`BOX_ACCESS_TOKEN`)
  - Salesforce (Salesforce CLI + org display check)
  - Databricks (`DATABRICKS_HOST`, `DATABRICKS_TOKEN`)
  - AWS (`aws` CLI + STS identity)
- Added guidance engine that prints concrete remediation steps when a provider is missing config or not connected.
- Added JSON output mode for scripted use (`windlass check --json`).
