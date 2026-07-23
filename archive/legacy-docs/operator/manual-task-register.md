# Manual Task Register

This file tracks manual UI/browser steps that `box-dispatch` cannot execute directly.

## Box

- `box.environment` - Provision Box folder and app context manually if token-based bootstrap is not available.
- `box.environment.unresolved` - Resolve Box access token, admin user, and folder id placeholders.

## Salesforce Agentforce

- `salesforce-agentforce.environment` - Bind Salesforce alias and verify Agentforce package path.

## Databricks

- `databricks.environment` - Validate Databricks host/token/workspace access in target account.

## Bedrock AgentCore

- `bedrock-agentcore.environment` - Confirm AWS profile/region and runtime name in target tenancy.

## Publish guardrails

- `publish-check` requires all providers to report `passed` in `config/runtime/bootstrap-state.json` before UI publish/share/activate operations.
