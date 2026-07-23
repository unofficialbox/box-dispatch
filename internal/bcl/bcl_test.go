package bcl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

func TestFromDeployedArtifacts(t *testing.T) {
	artifacts := []config.DeployedArtifact{
		{
			Provider:         "box",
			Scenario:         "acme",
			ArtifactType:     "box.folder",
			ProviderObjectID: "111",
			EnterpriseID:     "222",
			ArtifactName:     "Folder",
			Source:           "environment",
			CreatedAt:        "2026-07-23T12:00:00Z",
		},
	}
	doc := FromDeployedArtifacts("acme", "box", "bootstrap", "2026-07-23T12:00:00Z", artifacts)
	if len(doc.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(doc.Resources))
	}
	if doc.Resources[0].TerraformType != "null_resource" {
		t.Fatalf("expected null_resource terraform type, got %q", doc.Resources[0].TerraformType)
	}
	if _, ok := doc.Ext["box"]; !ok {
		t.Fatalf("expected box extension namespace in document")
	}
}

func TestWriteDeploymentBundle(t *testing.T) {
	artifacts := []config.DeployedArtifact{
		{
			Provider:         "box",
			Scenario:         "acme",
			ArtifactType:     "box.folder",
			ProviderObjectID: "111",
			EnterpriseID:     "222",
			ArtifactName:     "Shared Folder",
			Source:           "environment",
			CreatedAt:        "2026-07-23T12:00:00Z",
		},
	}
	doc := FromDeployedArtifacts("acme", "box", "resolve", "2026-07-23T12:00:00Z", artifacts)
	dir := t.TempDir()
	bclPath := filepath.Join(dir, "artifacts.bcl")
	tfPath := filepath.Join(dir, "artifacts.tf")
	tfJSONPath := filepath.Join(dir, "artifacts.tf.json")

	if err := WriteDeploymentBundle(bclPath, tfPath, tfJSONPath, doc); err != nil {
		t.Fatalf("WriteDeploymentBundle returned error: %v", err)
	}

	raw, err := os.ReadFile(bclPath)
	if err != nil {
		t.Fatalf("failed to read BCL file: %v", err)
	}
	rawStr := string(raw)
	if !strings.Contains(string(raw), "resource \"null_resource\"") {
		t.Fatalf("expected BCL output to contain terraform resource block, got: %s", string(raw))
	}
	if !strings.Contains(string(raw), "# Managed by box-dispatch") {
		t.Fatalf("expected BCL output to include box-dispatch header")
	}
	if !strings.Contains(rawStr, `"version" = "1.0"`) {
		t.Fatalf("expected bcl.version in emitted BCL payload, got: %s", rawStr)
	}
	if !strings.Contains(rawStr, `"ext" = {`) {
		t.Fatalf("expected bcl.ext in emitted BCL payload, got: %s", rawStr)
	}
	if !regexp.MustCompile(`"box"\s*=\s*\{[\s\S]*?"version"\s*=\s*"1.0"`).MatchString(rawStr) {
		t.Fatalf("expected ext.box namespace, got: %s", rawStr)
	}

	rawTF, err := os.ReadFile(tfPath)
	if err != nil {
		t.Fatalf("failed to read TF file: %v", err)
	}
	if !strings.Contains(string(rawTF), "resource \"null_resource\"") {
		t.Fatalf("expected TF output to contain null_resource block")
	}

	rawTf, err := os.ReadFile(tfJSONPath)
	if err != nil {
		t.Fatalf("failed to read TF JSON file: %v", err)
	}
	var tfDoc TFJSONDocument
	if err := json.Unmarshal(rawTf, &tfDoc); err != nil {
		t.Fatalf("invalid TF JSON: %v", err)
	}
	res := tfDoc.Resource["null_resource"]
	if res == nil {
		t.Fatalf("expected null_resource in tf translation")
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 null_resource resource, got %d", len(res))
	}
}

func TestSanitizeResourceNameFallsBackToDefault(t *testing.T) {
	name := sanitizeResourceName("", "")
	if name != "resource" {
		t.Fatalf("expected fallback name to be resource, got %q", name)
	}
}
