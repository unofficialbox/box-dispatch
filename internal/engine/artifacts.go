package engine

import (
	"path/filepath"
	"strings"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

const artifactSourceEnv = "environment"

type artifactBundlePaths struct {
	BCL string
}

func writeArtifactBundle(outputDir, providerKey, scenarioName, context, generatedAt string, artifacts []config.DeployedArtifact) (artifactBundlePaths, error) {
	paths := artifactBundlePaths{}
	if len(artifacts) == 0 {
		return paths, nil
	}

	paths.BCL = filepath.Join(outputDir, providerKey+"-artifacts.bcl")
	doc := bcl.FromDeployedArtifacts(scenarioName, providerKey, context, generatedAt, artifacts)
	if err := bcl.WriteBCL(paths.BCL, doc); err != nil {
		return artifactBundlePaths{}, err
	}
	return paths, nil
}

func collectArtifactInventory(providerKey, scenarioName string, env map[string]string, createdAt string) []config.DeployedArtifact {
	out := []config.DeployedArtifact{}

	switch providerKey {
	case "box":
		out = appendArtifacts(out, config.DeployedArtifact{
			Provider:         providerKey,
			Scenario:         scenarioName,
			ArtifactType:     "box.folder",
			ProviderObjectID: strings.TrimSpace(env["BOX_FOLDER_ID"]),
			EnterpriseID:     strings.TrimSpace(env["BOX_ENTERPRISE_ID"]),
			ArtifactName:     "Box folder",
			Source:           artifactSourceEnv,
			CreatedAt:        createdAt,
		})
		out = appendArtifacts(out, config.DeployedArtifact{
			Provider:         providerKey,
			Scenario:         scenarioName,
			ArtifactType:     "box.enterprise",
			ProviderObjectID: strings.TrimSpace(env["BOX_ENTERPRISE_ID"]),
			ArtifactName:     "Box enterprise",
			Source:           artifactSourceEnv,
			CreatedAt:        createdAt,
		})
	case "salesforce-agentforce":
		out = appendArtifacts(out, config.DeployedArtifact{
			Provider:     providerKey,
			Scenario:     scenarioName,
			ArtifactType: "salesforce.org",
			ProviderObjectID: firstNonEmpty(
				strings.TrimSpace(env["SF_ORG_ID"]),
				strings.TrimSpace(env["SALESFORCE_ORG_ID"]),
				strings.TrimSpace(env["SF_ORGANIZATION_ID"]),
			),
			ArtifactName: "Salesforce org",
			Source:       artifactSourceEnv,
			CreatedAt:    createdAt,
		})
	}

	return out
}

func appendArtifacts(items []config.DeployedArtifact, artifact config.DeployedArtifact) []config.DeployedArtifact {
	if artifact.ProviderObjectID == "" {
		return items
	}
	return append(items, artifact)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
