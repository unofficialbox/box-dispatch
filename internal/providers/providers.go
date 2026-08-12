package providers

type ProviderSpec struct {
	Key            string
	DisplayName    string
	Platform       string
	RequiredTools  []string
	RequiredTokens []string
	BootstrapSteps []string
	SmokeSteps     []string
}

var Specs = map[string]ProviderSpec{
	"box": {
		Key:            "box",
		DisplayName:    "Box",
		Platform:       "box",
		RequiredTools:  []string{"box"},
		RequiredTokens: []string{"BOX_ACCESS_TOKEN", "BOX_FOLDER_ID"},
		BootstrapSteps: []string{"validate-box-prereqs", "materialize-box-artifacts", "deploy-box-resources", "verify-box-end-state"},
		SmokeSteps:     []string{"confirm box folder_id can be resolved", "confirm folder scaffold artifacts are available", "confirm access token source is configured"},
	},
	"salesforce-agentforce": {
		Key:            "salesforce-agentforce",
		DisplayName:    "Salesforce",
		Platform:       "salesforce",
		RequiredTools:  []string{"sf"},
		RequiredTokens: []string{"SF_ALIAS", "SF_PROJECT_DIR"},
		BootstrapSteps: []string{"validate-salesforce-prereqs", "materialize-salesforce-artifacts", "deploy-salesforce-agentforce", "verify-salesforce-org-binding"},
		SmokeSteps:     []string{"confirm salesforce alias is resolvable", "confirm salesforce metadata is configured"},
	},
	"databricks": {
		Key:            "databricks",
		DisplayName:    "Databricks",
		Platform:       "databricks",
		RequiredTools:  []string{"databricks"},
		RequiredTokens: []string{"DATABRICKS_HOST", "DATABRICKS_TOKEN"},
		BootstrapSteps: []string{"validate-databricks-prereqs", "materialize-databricks-artifacts", "register-databricks-agent-tools", "verify-databricks-endpoints"},
		SmokeSteps:     []string{"confirm databricks token source is configured", "confirm databricks workspace endpoint artifacts are present"},
	},
	"bedrock-agentcore": {
		Key:            "bedrock-agentcore",
		DisplayName:    "AWS Bedrock AgentCore",
		Platform:       "agentcore",
		RequiredTools:  []string{"aws"},
		RequiredTokens: []string{"AWS_PROFILE", "AWS_REGION", "AGENTCORE_AGENT_NAME"},
		BootstrapSteps: []string{"validate-agentcore-prereqs", "materialize-agentcore-artifacts", "provision-agentcore-runtime", "verify-agentcore-runtime"},
		SmokeSteps:     []string{"confirm AgentCore runtime name is configured", "confirm AWS profile and region are available"},
	},
}
