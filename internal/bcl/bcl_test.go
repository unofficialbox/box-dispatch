package bcl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestWriteBCLRoundTripsNestedResourceConfig(t *testing.T) {
	doc := NewDocument("promotion", "generic", "test", "2026-10-26T12:00:00Z", nil)
	doc.Resources = []BCLResource{{
		Name: "agents", Type: "config_file", Provider: "generic",
		Config: map[string]any{"agents": []any{
			map[string]any{"name": "Legal Assistant", "configuration": map[string]any{"ask": map[string]any{"suggested_questions": []any{"What is the term?"}}}},
		}},
	}}
	path := filepath.Join(t.TempDir(), "agents.bcl")
	if err := WriteBCL(path, doc); err != nil {
		t.Fatal(err)
	}
	object, err := ConfigObject(path)
	if err != nil {
		t.Fatal(err)
	}
	var agents []struct {
		Name          string         `json:"name"`
		Configuration map[string]any `json:"configuration"`
	}
	if err := json.Unmarshal(object["agents"], &agents); err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Name != "Legal Assistant" {
		t.Fatalf("agents = %#v", agents)
	}
	questions := agents[0].Configuration["ask"].(map[string]any)["suggested_questions"].([]any)
	if len(questions) != 1 || questions[0] != "What is the term?" {
		t.Fatalf("suggested questions = %#v", questions)
	}
}

func TestWriteBCLRoundTripsStringSlices(t *testing.T) {
	doc := NewDocument("promotion", "generic", "test", "2026-10-28T12:00:00Z", nil)
	doc.Resources = []BCLResource{{
		Name: "templates", Type: "config_file", Provider: "generic",
		Config: map[string]any{"fields": []any{
			map[string]any{"key": "status", "options": []string{"Draft", "Final"}},
			map[string]any{"key": "summary", "options": []string{}},
		}},
	}}
	path := filepath.Join(t.TempDir(), "templates.bcl")
	if err := WriteBCL(path, doc); err != nil {
		t.Fatal(err)
	}
	object, err := ConfigObject(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields []struct {
		Key     string   `json:"key"`
		Options []string `json:"options"`
	}
	if err := json.Unmarshal(object["fields"], &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || !reflect.DeepEqual(fields[0].Options, []string{"Draft", "Final"}) || len(fields[1].Options) != 0 {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestSanitizeResourceNameFallsBackToDefault(t *testing.T) {
	name := sanitizeResourceName("", "")
	if name != "resource" {
		t.Fatalf("expected fallback name to be resource, got %q", name)
	}
}
