package shellstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/bcl"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

// isolateRoot points config.Paths().Root at a temp dir by switching the working
// directory, so state files land somewhere disposable.
func isolateRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

func TestConnectionSettingsRoundTripAsBCL(t *testing.T) {
	root := isolateRoot(t)
	want := config.ConnectionSettings{
		SalesforceAlias:       "agentforce",
		SalesforceDevHubAlias: "devhub",
		DatabricksProfile:     "clm",
		AWSProfile:            "demo",
		AWSRegion:             "us-east-1",
	}
	if err := SaveConnectionSettings(want); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, stateDirName, connectionSettingsBCL)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to be written: %v", connectionSettingsBCL, err)
	}
	// The file must be valid BCL, not just JSON with a .bcl name.
	if _, err := bcl.LoadBCL(path); err != nil {
		t.Fatalf("written file is not valid BCL: %v", err)
	}

	got, err := LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestSolutionPlanRoundTripAsBCL(t *testing.T) {
	isolateRoot(t)
	want := config.SolutionPlan{
		Components: []string{"box", "salesforce"},
		TemplateID: "clm",
		Template:   "Contract Lifecycle Management",
		Repository: "https://github.com/unofficialbox/box-bedrock-for-clm",
	}
	if err := SaveSolutionPlan(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSolutionPlan()
	if err != nil {
		t.Fatal(err)
	}
	if got.TemplateID != want.TemplateID || got.Repository != want.Repository || len(got.Components) != 2 {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestUISettingsRoundTripAsBCL(t *testing.T) {
	root := isolateRoot(t)
	want := config.UISettings{BoxComponentVisibility: map[string]bool{
		"folder_structure": true,
		"box_form":         false,
	}}
	if err := SaveUISettings(want); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, stateDirName, uiSettingsBCL)
	if _, err := bcl.LoadBCL(path); err != nil {
		t.Fatalf("written file is not valid BCL: %v", err)
	}
	got, err := LoadUISettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BoxComponentVisibility) != 2 || !got.BoxComponentVisibility["folder_structure"] || got.BoxComponentVisibility["box_form"] {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}
}

func TestLoadMigratesLegacyJSON(t *testing.T) {
	root := isolateRoot(t)
	// Only a legacy JSON file (under the old .windlass dir) exists; no .bcl yet.
	legacyDir := filepath.Join(root, legacyStateDirName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"salesforceAlias":"legacy-alias","awsRegion":"us-west-2"}`)
	if err := os.WriteFile(filepath.Join(legacyDir, connectionSettingsJSON), legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.SalesforceAlias != "legacy-alias" || got.AWSRegion != "us-west-2" {
		t.Fatalf("legacy JSON not migrated: %+v", got)
	}
}

func TestMissingStateReturnsZeroValue(t *testing.T) {
	isolateRoot(t)
	got, err := LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got != (config.ConnectionSettings{}) {
		t.Fatalf("expected zero value for absent state, got %+v", got)
	}
}
