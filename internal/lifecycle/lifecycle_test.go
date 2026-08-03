package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/solution"
)

// TestReadBoxConfigObjectPrefersBCL confirms the migrated BCL artifact wins over
// a legacy JSON file of the same stem, and that its envelope fields are stripped.
func TestReadBoxConfigObjectPrefersBCL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata-templates.json"), []byte(`{"templates":[{"templateKey":"from-json"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	bclSrc := `locals {
  bcl = {
    "resources" = [
      {
        "config" = {
          "artifact_type" = "config"
          "templates" = [
            { "templateKey" = "from-bcl" },
          ]
        }
      },
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "metadata-templates.bcl"), []byte(bclSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	obj, err := readBoxConfigObject(dir, "metadata-templates")
	if err != nil {
		t.Fatalf("readBoxConfigObject: %v", err)
	}
	if _, ok := obj["artifact_type"]; ok {
		t.Error("envelope key artifact_type was not stripped")
	}
	var tmpls []struct {
		TemplateKey string `json:"templateKey"`
	}
	if err := json.Unmarshal(obj["templates"], &tmpls); err != nil {
		t.Fatal(err)
	}
	if len(tmpls) != 1 || tmpls[0].TemplateKey != "from-bcl" {
		t.Fatalf("expected the BCL payload to win, got %+v", tmpls)
	}
}

// TestReadBoxConfigObjectFallsBackToJSON confirms older JSON-only packages still
// read when no BCL artifact is present.
func TestReadBoxConfigObjectFallsBackToJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata-templates.json"), []byte(`{"templates":[{"templateKey":"legacy"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	obj, err := readBoxConfigObject(dir, "metadata-templates")
	if err != nil {
		t.Fatalf("readBoxConfigObject: %v", err)
	}
	var tmpls []struct {
		TemplateKey string `json:"templateKey"`
	}
	if err := json.Unmarshal(obj["templates"], &tmpls); err != nil {
		t.Fatal(err)
	}
	if len(tmpls) != 1 || tmpls[0].TemplateKey != "legacy" {
		t.Fatalf("expected the JSON payload, got %+v", tmpls)
	}
}

func TestBoxAccessTokenParsesCLIJSONWithoutPersistingIt(t *testing.T) {
	want := strings.Repeat("a", 32)
	if got := boxAccessToken([]byte(`{"accessToken":"` + want + `"}`)); got != want {
		t.Fatalf("token parser returned %q", got)
	}
}

func TestBoxUserIDParsesAuthenticatedCLIIdentity(t *testing.T) {
	if got := boxUserID([]byte(`{"id":"123456","name":"Demo User"}`)); got != "123456" {
		t.Fatalf("user ID parser returned %q", got)
	}
}

func TestBoxRootFolderRequiresExplicitApproval(t *testing.T) {
	t.Setenv("BOX_PARENT_FOLDER_ID", "0")
	t.Setenv("BOX_ALLOW_ROOT_FOLDER", "")
	if _, err := loadBoxTarget(t.TempDir(), "Workspace"); err == nil {
		t.Fatal("root-folder target should require explicit approval")
	}
}

func TestBoxComponentsFollowDeploymentDependencyOrder(t *testing.T) {
	manifest, err := solution.Load(testCLMPackage(t))
	if err != nil {
		t.Fatal(err)
	}
	components := []string{"Box App:App", "Metadata Template:Contract", "Folder Structure:Workspace", "AI Agent:Agent"}
	slices.SortStableFunc(components, func(a, b string) int { return manifest.Rank(a) - manifest.Rank(b) })
	want := []string{"Folder Structure:Workspace", "Metadata Template:Contract", "AI Agent:Agent", "Box App:App"}
	if !slices.Equal(components, want) {
		t.Fatalf("ordered components = %#v, want %#v", components, want)
	}
}

func TestCLMDeploymentOrder(t *testing.T) {
	manifest, err := solution.Load(testCLMPackage(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Folder Structure",
		"Metadata Template",
		"Doc Gen Template",
		"Extract Configuration",
		"AI Agent",
		"HTTPS Connector",
		"Box Form",
		"Automate Workflow",
		"Box Hub",
		"Sample Content",
		"Box App",
	}
	if !slices.Equal(manifest.Box.ComponentOrder, want) {
		t.Fatalf("deployment order = %#v, want %#v", manifest.Box.ComponentOrder, want)
	}
}

func TestCLMPrivateSurfacesHaveDeployAdapters(t *testing.T) {
	manifest, err := solution.Load(testCLMPackage(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, componentType := range []string{"Box Form", "Box App"} {
		capability, found := manifest.Capability(componentType)
		if !found || capability.API != "private" || capability.Handler == "" || !slices.Contains(capability.Operations, "deploy") {
			t.Fatalf("%s private adapter = %#v", componentType, capability)
		}
	}
}

func TestValidatePrivateSurfacesAreDeployableWithoutBrowser(t *testing.T) {
	root := testCLMPackage(t)
	manifest, err := solution.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := solution.ReadDeploymentSettings(root, manifest.DeploymentConfig)
	if err != nil {
		t.Fatal(err)
	}
	// Real packages get a run_id at build time; supply a stable one so the
	// create_new naming strategy resolves.
	settings.Box.RunID = "20260101T000000Z"
	item := &Item{Provider: "box"}
	// Must not touch the browser. The Box Form is hidden while boxFormEnabled is
	// off, so it is not classified at all; the Box App is demoted to manual
	// because its Meteor deploy path is deprecated (boxAppMeteorDeploySupported).
	if err := validateBoxPrivateAdapters(root, manifest, settings.Box, settings.Box.Components, item); err != nil {
		t.Fatal(err)
	}
	const form, app = "Box Form:New Contract Request", "Box App:Contract Lifecycle Management"
	if boxFormEnabled {
		if !slices.Contains(item.DeployableComponents, form) {
			t.Fatalf("Box Form not marked deployable: %+v", item.DeployableComponents)
		}
	} else {
		for _, bucket := range [][]string{item.DeployableComponents, item.Missing, item.Present} {
			if slices.Contains(bucket, form) {
				t.Fatalf("Box Form must not be surfaced while disabled: %+v", bucket)
			}
		}
	}
	if boxAppMeteorDeploySupported {
		if !slices.Contains(item.DeployableComponents, app) {
			t.Fatalf("Box App should be deployable while Meteor is supported: %+v", item.DeployableComponents)
		}
	} else {
		if slices.Contains(item.DeployableComponents, app) {
			t.Fatalf("Box App must not be auto-deployable while Meteor is deprecated: %+v", item.DeployableComponents)
		}
		if !slices.Contains(item.Missing, app) {
			t.Fatalf("Box App should be tracked as a manual (missing) component: %+v", item.Missing)
		}
	}
	if slices.Contains(item.Present, app) {
		t.Fatalf("Box App should not be reported present without inspection: %+v", item.Present)
	}
}

func TestLocalFileSHA1MatchesBoxFormat(t *testing.T) {
	// Box stores a lowercase hex SHA-1 of the file content; localFileSHA1 must
	// produce the same string so the two can be compared for change detection.
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello box\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := localFileSHA1(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "59330051bc28538fb1927469d166b9cbe55b0a50"
	if got != want {
		t.Fatalf("localFileSHA1 = %q, want %q", got, want)
	}
}

func testCLMPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, ".dispatch")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "package.json"), []byte(`{"template_id":"clm"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestValidateUsesReceiptsAndMarksUnsupportedAssets(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config/box/spec.json", "{}")
	write("config/agentcore/spec.json", "{}")
	write("config/runtime/validation-receipts.json", `{"receipts":[{"platform":"Box","status":"passed"}]}`)

	items, err := Validate(root, []string{"box", "aws"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Status != StatusPresent {
		t.Fatalf("Box status = %q, want present", items[0].Status)
	}
	if items[1].Status != StatusManual || items[1].Deployable {
		t.Fatalf("AWS result = %#v, want explicit manual action", items[1])
	}
}

func TestSummarizeSalesforceErrorOmitsStack(t *testing.T) {
	output := []byte(`{"name":"ExpectedSourceFilesError","message":"dist is missing","actions":["Run npm build"],"stack":"very long stack"}`)
	detail := summarizeSalesforceError(output, os.ErrInvalid)
	if detail != "ExpectedSourceFilesError: dist is missing: Next: Run npm build" {
		t.Fatalf("unexpected summary: %q", detail)
	}
}

func TestSalesforceUIBundleWithExistingOutputNeedsNoBuild(t *testing.T) {
	project := t.TempDir()
	bundle := filepath.Join(project, "force-app", "main", "default", "uiBundles", "example")
	if err := os.MkdirAll(filepath.Join(bundle, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "ui-bundle.json"), []byte(`{"outputDir":"dist"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "dist", "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := buildSalesforceUIBundles(project); err != nil {
		t.Fatalf("existing UI Bundle output should not rebuild: %v", err)
	}
}

func TestSalesforceInventorySkipsDeployWhenEverythingExists(t *testing.T) {
	item := Item{Provider: "salesforce", Name: "Salesforce metadata"}
	expected := map[string]bool{"CustomObject:Contract__c": true, "CustomField:Contract__c.Status__c": true}
	result := classifySalesforceInventory(item, expected, expected, "demo")
	if result.Status != StatusPresent || result.Deployable || len(result.Missing) != 0 {
		t.Fatalf("inventory result = %#v, want present and not deployable", result)
	}
}

func TestSalesforceInventoryDeploysOnlyWhenComponentsAreMissing(t *testing.T) {
	item := Item{Provider: "salesforce", Name: "Salesforce metadata"}
	expected := map[string]bool{"CustomObject:Contract__c": true, "CustomField:Contract__c.Status__c": true}
	existing := map[string]bool{"CustomObject:Contract__c": true}
	result := classifySalesforceInventory(item, expected, existing, "demo")
	if result.Status != StatusMissing || !result.Deployable || len(result.Present) != 1 || len(result.Missing) != 1 {
		t.Fatalf("inventory result = %#v, want one existing and one missing component", result)
	}
}

func TestBoxComponentsAreParsedFromPackagedConfiguration(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ai-agent-specs.json", `{"agents":[{"key":"risk","name":"Risk Agent"}]}`)
	write("automate-workflows.json", `{"workflows":[{"key":"intake","name":"Intake"}]}`)
	write("https-connectors.json", `{"connectors":[{"key":"salesforce","name":"Salesforce"}]}`)
	write("metadata-templates.json", `{"templates":[{"templateKey":"contract","displayName":"Contract"}]}`)
	write("docgen-template-data.json", `{"approvalMemo":{}}`)
	write("extract-field-prompts.json", `{"contractIntake":{}}`)
	for _, name := range []string{"box-app-blueprint.md", "folder-template.md", "form-definition.json", "hub-blueprint.md"} {
		write(name, "# Blueprint")
	}
	write("form-definition.json", `{}`)
	manifest, err := solution.LoadBundled("clm")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := boxComponentEntries(root, manifest, solution.ComponentSelection{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 12 {
		t.Fatalf("got %d Box components, want 12: %#v", len(entries), entries)
	}
}

func TestBoxComponentsIncludeManifestWorkspaceWithoutMarkerFile(t *testing.T) {
	manifest, err := solution.LoadBundled("clm")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := boxComponentEntries(t.TempDir(), manifest, solution.ComponentSelection{
		Mode:       "custom",
		Selections: map[string]bool{"folder_structure": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := manifest.Box.Workspace.ComponentType + ":" + manifest.Box.Workspace.DisplayName
	if !slices.Contains(entries, want) {
		t.Fatalf("workspace component missing without marker file: got %#v, want %q", entries, want)
	}
}

func TestBoxRequestBodyUnwrapsCLIEnvelope(t *testing.T) {
	// `box request` wraps responses; parsing the envelope instead of the body
	// made IDs come back empty and existing objects look absent.
	envelope := []byte(`{"statusCode":201,"headers":{"date":"x"},"body":{"id":"12345","type":"hub"}}`)
	body, err := boxRequestBody(envelope, "POST", "/hubs")
	if err != nil {
		t.Fatal(err)
	}
	if got := boxResourceID(body); got != "12345" {
		t.Fatalf("id = %q, want 12345", got)
	}

	// A non-2xx status must be an error, not a silently parsed result.
	failure := []byte(`{"statusCode":403,"headers":{},"body":{"message":"forbidden"}}`)
	if _, err := boxRequestBody(failure, "POST", "/hubs"); err == nil {
		t.Fatal("a 403 response must surface as an error")
	}

	// Output that is not an envelope passes through untouched.
	plain := []byte(`{"entries":[{"title":"a"}]}`)
	body, err = boxRequestBody(plain, "GET", "/enterprise_hubs")
	if err != nil || string(body) != string(plain) {
		t.Fatalf("plain output should pass through, got %q err %v", body, err)
	}
}

func TestPrivateSurfaceScriptIsolatesFormAndAppFailures(t *testing.T) {
	script := boxPrivateBrowserScript(`{"operation":"deploy"}`)
	// Each surface must catch its own error, so a failing Box App cannot discard
	// a Box Form that was actually created and leave it unrecorded.
	for _, want := range []string{
		`if(request.form){try{`,
		`if(request.app){try{`,
		`catch(error){result.form=`,
		`catch(error){result.app=`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("injected script is missing %q", want)
		}
	}
	// The template error should explain the real limitation rather than implying
	// the operator should go and create a template app.
	if !strings.Contains(script, "no portable Box App definition") {
		t.Fatal("template failure should explain that the solution has no portable app definition")
	}
}
