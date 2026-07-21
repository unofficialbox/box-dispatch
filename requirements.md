# Box Windlass CLI — First-Principles Requirements

## 1) Foundation
- The CLI configures and deploys a solution composed of:
  - Box (unstructured data + data-aware AI agents)
  - Salesforce/Agentforce (structured records + frontend + agent orchestration)
  - Databricks (analytics and AI feature workloads)
  - AWS Bedrock AgentCore (runtime agent orchestration)
- Source of truth for each solution run is a user-supplied Git repo containing demo scenario templates, templates, and environment manifests.
- The CLI must never assume a single environment; all target integrations must be optional and environment-profile driven.

## 2) UX Goals
- First-class interactive setup for first-time users:
  - Guided prompts for prerequisites and credentials
  - Clear dependency matrix and actionable remediation steps
  - Smooth defaults, progress visibility, and friendly recovery
- High-quality silent mode for automation and CI:
  - Non-interactive flags only
  - Deterministic execution and strict machine-readable output
- Command surface must be composable:
  - `init`, `check`, `configure`, `deploy`, `status`, `teardown`, `source`
  - A consistent JSON output mode for scripts

## 3) Security and Safety
- Store secrets only in secure locations or provider-native keychains where possible.
- Never print credentials to terminal.
- Validate destructive operations before execution and always show a preview path.
- Keep operations idempotent and resumable where possible.

## 4) Portability and Distribution
- Single binary distribution for macOS, Linux, Windows.
- Deterministic install and offline-friendly caches.
- Minimal mandatory dependencies, with optional external CLIs detected and validated.

## 5) Implementation Strategy (Go-first)
- Go CLI with a Bubble Tea terminal experience (`github.com/charmbracelet/bubbletea`) and rich markdown/status rendering using `github.com/charmbracelet/glamour`.
- Keep command layer thin and policy layer centralized:
  - Engine coordinates steps and state
  - Providers encapsulate Box/Salesforce/Databricks/AgentCore adapters
- Use explicit source manifests and generated runtime config instead of mutable “magic” defaults.

## 6) Initial Command Contract
- `windlass init`:
  - Capture scenario source repo and sync required project files into local working tree.
- `windlass check`:
  - Verify required tooling, credentials, and endpoint reachability.
- `windlass setup`:
  - Apply credentials/config templates and bootstrap infra hooks.
- `windlass deploy`:
  - Deploy solution topology and activate services.
- `windlass status`:
  - Show health and runtime summaries.
- `windlass teardown`:
  - Controlled deletion and rollback actions.
