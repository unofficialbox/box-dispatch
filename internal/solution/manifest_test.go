package solution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
)

func TestManifestPayloadPreservesCapabilityCatalogButExcludesUnsupportedDeploymentDefaults(t *testing.T) {
	manifest, err := decodeManifestPayload([]byte(`{
  "schema_version": "1.0",
  "template_id": "test",
  "box": {
    "deployment_defaults": {
      "component_strategies": {
        "folder_structure": {"strategy": "reuse"},
        "box_form": {"strategy": "reuse"}
      },
      "components": {
        "mode": "custom",
        "selections": {"folder_structure": true, "box_form": true}
      }
    },
    "component_order": ["Folder Structure", "Automate Workflow", "Box Form", "Box App"],
    "capabilities": [
      {"id": "folder_structure", "component_type": "Folder Structure", "api": "public", "handler": "box.folder-tree"},
      {"id": "automate_workflow", "component_type": "Automate Workflow", "api": "public", "handler": "box.automate-workflow", "operations": ["validate"]},
      {"id": "box_form", "component_type": "Box Form", "api": "private"},
      {"id": "box_app", "component_type": "Box App", "api": "configuration"}
    ]
  }
}`), "test manifest")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Box.Capabilities) != 4 {
		t.Fatalf("capabilities = %#v, want complete capability catalog", manifest.Box.Capabilities)
	}
	if _, found := manifest.Box.DeploymentDefaults.ComponentStrategies["box_form"]; found {
		t.Fatal("non-public component strategy was retained")
	}
	if _, found := manifest.Box.DeploymentDefaults.Components.Selections["box_form"]; found {
		t.Fatal("non-public component selection was retained")
	}
	for _, component := range []string{"Automate Workflow", "Box Form", "Box App"} {
		capability, found := manifest.Capability(component)
		if !found {
			t.Fatalf("%s was removed from the capability catalog", component)
		}
		if capability.CanDeploy() || manifest.CapabilityEnabled(component, ComponentSelection{Mode: "all"}) {
			t.Fatalf("%s became deployable without a complete public API", component)
		}
	}
	folder, found := manifest.Capability("Folder Structure")
	if !found || !folder.CanDeploy() || !manifest.CapabilityEnabled("Folder Structure", ComponentSelection{Mode: "all"}) {
		t.Fatal("public folder capability is not deployable")
	}
}

func TestLoadBundledManifestFromBCL(t *testing.T) {
	manifest, err := LoadBundled("clm")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TemplateID != "clm" || manifest.Box.Workspace.Name != "CLM-2026-Northstar" {
		t.Fatalf("bundled manifest = %+v", manifest)
	}
	if len(manifest.Box.Capabilities) != 11 {
		t.Fatalf("got %d bundled capabilities, want 11", len(manifest.Box.Capabilities))
	}
}

func TestWriteManifestWritesCanonicalBCLAndRemovesObsoleteJSON(t *testing.T) {
	root := t.TempDir()
	for _, obsolete := range []string{"dispatch.json", ".dispatch/deployment.json"} {
		path := filepath.Join(root, filepath.FromSlash(obsolete))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := testManifest("bcl-template")
	if err := WriteManifest(root, manifest); err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{"dispatch.json", ".dispatch/deployment.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(obsolete))); !os.IsNotExist(err) {
			t.Fatalf("obsolete JSON remains at %s: %v", obsolete, err)
		}
	}
	bclPath := filepath.Join(root, ManifestFile)
	doc, err := bcl.LoadBCL(bclPath)
	if err != nil {
		t.Fatalf("canonical manifest is not valid BCL: %v", err)
	}
	if doc.Context != manifestContext || doc.Provider != manifestProvider {
		t.Fatalf("BCL envelope = provider %q context %q", doc.Provider, doc.Context)
	}
	raw, err := os.ReadFile(bclPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# Managed by box-dispatch") {
		t.Fatalf("canonical manifest was not emitted in BCL syntax:\n%s", raw)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TemplateID != manifest.TemplateID || loaded.Box.Workspace.Name != manifest.Box.Workspace.Name {
		t.Fatalf("round-trip manifest = %+v, want %+v", loaded, manifest)
	}
}

func TestLoadRejectsInvalidBCL(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ManifestFile), []byte("not valid BCL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unsupported BCL format") {
		t.Fatalf("Load error = %v, want invalid canonical BCL error", err)
	}
}

func TestWriteBundledCreatesCanonicalPackageManifest(t *testing.T) {
	root := t.TempDir()
	if err := WriteBundled(root, "clm"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ManifestFile)); err != nil {
		t.Fatalf("canonical manifest was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, defaultDeploymentConfig)); err != nil {
		t.Fatalf("deployment defaults were not initialized: %v", err)
	}
	if _, err := bcl.LoadBCL(filepath.Join(root, defaultDeploymentConfig)); err != nil {
		t.Fatalf("deployment defaults are not valid BCL: %v", err)
	}
}

func TestDeploymentSettingsRoundTripAsBCL(t *testing.T) {
	root := t.TempDir()
	want := DefaultDeploymentSettings()
	want.Box.GlobalStrategy = StrategyReuse
	want.Box.Components = ComponentSelection{Mode: "custom", Selections: map[string]bool{"folder_structure": true}}
	if err := WriteDeploymentSettings(root, defaultDeploymentConfig, want); err != nil {
		t.Fatal(err)
	}
	doc, err := bcl.LoadBCL(filepath.Join(root, defaultDeploymentConfig))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Context != deploymentContext {
		t.Fatalf("deployment BCL context = %q", doc.Context)
	}
	got, err := ReadDeploymentSettings(root, defaultDeploymentConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got.Box.GlobalStrategy != StrategyReuse || got.Box.Components.Mode != "custom" || !got.Box.Components.Selections["folder_structure"] {
		t.Fatalf("deployment settings = %+v", got)
	}
}

func TestDeploymentSettingsRejectJSONReference(t *testing.T) {
	root := t.TempDir()
	if err := WriteDeploymentSettings(root, ".dispatch/deployment.json", DefaultDeploymentSettings()); err == nil || !strings.Contains(err.Error(), ".bcl") {
		t.Fatalf("JSON deployment reference error = %v", err)
	}
}

func TestWriteManifestRejectsJSONDeploymentReference(t *testing.T) {
	manifest := testManifest("invalid-template")
	manifest.DeploymentConfig = ".dispatch/deployment.json"
	if err := WriteManifest(t.TempDir(), manifest); err == nil || !strings.Contains(err.Error(), ".bcl") {
		t.Fatalf("JSON manifest deployment reference error = %v", err)
	}
}

func testManifest(templateID string) Manifest {
	return Manifest{
		SchemaVersion:    "1.0",
		TemplateID:       templateID,
		DeploymentConfig: defaultDeploymentConfig,
		Box: BoxManifest{
			Workspace: Workspace{
				ComponentType: "Folder Structure",
				DisplayName:   "Test Workspace",
				Name:          "test-workspace",
			},
			Capabilities: []Capability{{
				ID:            "folder_structure",
				ComponentType: "Folder Structure",
				API:           "public",
				Handler:       "box.folder-tree",
			}},
		},
	}
}
