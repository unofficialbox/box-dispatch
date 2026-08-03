package solution

import (
	"slices"
	"testing"
)

func TestParseExcludesCapabilitiesWithoutPublicAPIs(t *testing.T) {
	manifest, err := parse([]byte(`{
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
    "component_order": ["Folder Structure", "Box Form", "Box App"],
    "capabilities": [
      {"id": "folder_structure", "component_type": "Folder Structure", "api": "public"},
      {"id": "box_form", "component_type": "Box Form", "api": "private"},
      {"id": "box_app", "component_type": "Box App", "api": "configuration"}
    ]
  }
}`), "test manifest")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Box.Capabilities) != 1 || manifest.Box.Capabilities[0].ComponentType != "Folder Structure" {
		t.Fatalf("capabilities = %#v, want public capability only", manifest.Box.Capabilities)
	}
	if !slices.Equal(manifest.Box.ComponentOrder, []string{"Folder Structure"}) {
		t.Fatalf("component order = %#v, want public capability only", manifest.Box.ComponentOrder)
	}
	if _, found := manifest.Box.DeploymentDefaults.ComponentStrategies["box_form"]; found {
		t.Fatal("non-public component strategy was retained")
	}
	if _, found := manifest.Box.DeploymentDefaults.Components.Selections["box_form"]; found {
		t.Fatal("non-public component selection was retained")
	}
}
