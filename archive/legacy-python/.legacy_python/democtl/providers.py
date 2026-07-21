from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Dict, Iterable, List


@dataclass(frozen=True)
class ProviderSpec:
    key: str
    display_name: str
    platform: str
    required_tools: List[str]
    required_env: List[str]
    bootstrap_steps: List[str]
    smoke_steps: List[str]


PROVIDERS: Dict[str, ProviderSpec] = {
    "box": ProviderSpec(
        key="box",
        display_name="Box",
        platform="box",
        required_tools=["box"],
        required_env=[
            "BOX_ACCESS_TOKEN",
            "BOX_FOLDER_ID",
        ],
        bootstrap_steps=[
            "validate-box-prereqs",
            "materialize-box-artifacts",
            "deploy-box-resources",
            "verify-box-end-state",
        ],
        smoke_steps=[
            "confirm box folder_id can be resolved",
            "confirm folder scaffold artifacts are available",
            "confirm access token source is configured",
        ],
    ),
    "salesforce-agentforce": ProviderSpec(
        key="salesforce-agentforce",
        display_name="Salesforce Agentforce",
        platform="salesforce",
        required_tools=["sf"],
        required_env=[
            "SF_ALIAS",
            "SF_PROJECT_DIR",
        ],
        bootstrap_steps=[
            "validate-salesforce-prereqs",
            "materialize-salesforce-artifacts",
            "deploy-salesforce-agentforce",
            "verify-salesforce-org-binding",
        ],
        smoke_steps=[
            "confirm salesforce alias is resolvable",
            "confirm agentforce metadata is configured",
        ],
    ),
    "databricks": ProviderSpec(
        key="databricks",
        display_name="Databricks",
        platform="databricks",
        required_tools=["databricks"],
        required_env=[
            "DATABRICKS_HOST",
            "DATABRICKS_TOKEN",
        ],
        bootstrap_steps=[
            "validate-databricks-prereqs",
            "materialize-databricks-artifacts",
            "register-databricks-agent-tools",
            "verify-databricks-endpoints",
        ],
        smoke_steps=[
            "confirm databricks token source is configured",
            "confirm databricks workspace endpoint artifacts are present",
        ],
    ),
    "bedrock-agentcore": ProviderSpec(
        key="bedrock-agentcore",
        display_name="AWS Bedrock AgentCore",
        platform="agentcore",
        required_tools=["aws"],
        required_env=[
            "AWS_PROFILE",
            "AWS_REGION",
            "AGENTCORE_AGENT_NAME",
        ],
        bootstrap_steps=[
            "validate-agentcore-prereqs",
            "materialize-agentcore-artifacts",
            "provision-agentcore-runtime",
            "verify-agentcore-runtime",
        ],
        smoke_steps=[
            "confirm AgentCore runtime name is configured",
            "confirm AWS profile and region are available",
        ],
    ),
}

