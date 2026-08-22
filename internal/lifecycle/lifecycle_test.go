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

func TestReportValidationResultsStreamsEveryComponentState(t *testing.T) {
	updates := make([]ProgressUpdate, 0)
	reportValidationResults(Reporter(func(update ProgressUpdate) {
		updates = append(updates, update)
	}), []string{"CustomField:Contract__c.Status__c", "CustomObject:Contract__c"}, []string{"CustomObject:Contract__c"}, []string{"CustomField:Contract__c.Status__c"})

	if len(updates) != 4 {
		t.Fatalf("updates = %#v", updates)
	}
	for index := 0; index < len(updates); index += 2 {
		if updates[index].State != ProgressRunning || updates[index+1].State != ProgressCompleted {
			t.Fatalf("component updates = %#v", updates[index:index+2])
		}
		if updates[index+1].Current != index/2+1 || updates[index+1].Total != 2 {
			t.Fatalf("progress counters = %#v", updates[index+1])
		}
	}
}

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
	components := []string{"Box Hub:Hub", "Metadata Template:Contract", "Folder Structure:Workspace", "AI Agent:Agent"}
	slices.SortStableFunc(components, func(a, b string) int { return manifest.Rank(a) - manifest.Rank(b) })
	want := []string{"Folder Structure:Workspace", "Metadata Template:Contract", "AI Agent:Agent", "Box Hub:Hub"}
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
		"Box Hub",
		"Sample Content",
	}
	if !slices.Equal(manifest.Box.ComponentOrder, want) {
		t.Fatalf("deployment order = %#v, want %#v", manifest.Box.ComponentOrder, want)
	}
}

func TestCLMCatalogRetainsUnsupportedCapabilitiesAsNonDeployable(t *testing.T) {
	manifest, err := solution.Load(testCLMPackage(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range []string{"Box Form", "Box App", "HTTPS Connector", "Automate Workflow"} {
		capability, found := manifest.Capability(component)
		if !found {
			t.Fatalf("%s must remain documented in the capability catalog", component)
		}
		if capability.CanDeploy() || manifest.CapabilityEnabled(component, solution.ComponentSelection{Mode: "all"}) {
			t.Fatalf("%s must not be deployable without a complete public API", component)
		}
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
	output := []byte("Warning: @salesforce/cli update available.\n" + `{"name":"ExpectedSourceFilesError","message":"dist is missing","actions":["Run npm build"],"stack":"very long stack"}`)
	detail := summarizeSalesforceError(output, os.ErrInvalid)
	if detail != "ExpectedSourceFilesError: dist is missing: Next: Run npm build" {
		t.Fatalf("unexpected summary: %q", detail)
	}
	_, diagnostic := salesforceErrorDetails(output, os.ErrInvalid)
	if !strings.Contains(diagnostic, "very long stack") {
		t.Fatalf("full diagnostic omitted Salesforce stack: %q", diagnostic)
	}
}

func TestReadSalesforceInventorySkipsCLIWarning(t *testing.T) {
	output := []byte("Warning: @salesforce/cli update available.\n" + `{"result":{"fileProperties":[{"fullName":"CLM_Demo","type":"CustomApplication"},{"fullName":"unpackaged/package.xml","type":"Package"}]}}`)
	existing, err := readSalesforceInventory(output, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !existing["CustomApplication:CLM_Demo"] {
		t.Fatalf("inventory = %v, want packaged CustomApplication", existing)
	}
	if existing["Package:unpackaged/package.xml"] {
		t.Fatalf("inventory = %v, package manifest should be ignored", existing)
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
	ignore, err := os.ReadFile(filepath.Join(project, ".forceignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignore), "node_modules/") {
		t.Fatalf("UI Bundle deploy exclusions missing node_modules: %q", ignore)
	}
}

func TestSalesforceProjectForceIgnorePreservesExistingRules(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".forceignore")
	if err := os.WriteFile(path, []byte("dist/local-only.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSalesforceProjectForceIgnore(project); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "dist/local-only.txt") || strings.Count(got, "node_modules/") != 1 {
		t.Fatalf("unexpected .forceignore contents: %q", got)
	}
	if err := ensureSalesforceProjectForceIgnore(project); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), "node_modules/") != 1 {
		t.Fatalf("node_modules exclusion duplicated: %q", data)
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

func TestSalesforceMetadataDeployTargetsOnlyMissingMetadata(t *testing.T) {
	missing := []string{
		"Permission Set Assignment:Box Sign",
		"ListView:CLM_Contract__c.All",
		"Managed Package:Box for Salesforce 5.43",
		"FlexiPage:CLM_Contract_Record_Page",
	}
	metadata := missingSalesforceMetadata(missing)
	want := []string{"FlexiPage:CLM_Contract_Record_Page", "ListView:CLM_Contract__c.All"}
	if !slices.Equal(metadata, want) {
		t.Fatalf("missing Salesforce metadata = %#v, want %#v", metadata, want)
	}

	args := salesforceMetadataDeployArgs("dispatch-scratch", metadata)
	joined := strings.Join(args, " ")
	for _, component := range want {
		if !strings.Contains(joined, "--metadata "+component) {
			t.Fatalf("Salesforce deploy args omitted %q: %q", component, joined)
		}
	}
	for _, unwanted := range []string{"--source-dir", "Permission Set Assignment", "Managed Package"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("Salesforce deploy args unexpectedly contain %q: %q", unwanted, joined)
		}
	}
}

func TestSalesforceMetadataDeploySkipsWhenOnlyPrerequisitesRemain(t *testing.T) {
	missing := []string{
		"Permission Set Assignment:Box Sign",
		"Managed Package:Box for Salesforce 5.43",
	}
	if metadata := missingSalesforceMetadata(missing); len(metadata) != 0 {
		t.Fatalf("missing Salesforce metadata = %#v, want none", metadata)
	}
}

func TestSalesforceMetadataPreviewTreatsConflictsAsExisting(t *testing.T) {
	output := []byte("Warning: Salesforce CLI update available\n" + `{
  "status": 0,
  "result": {
    "toDeploy": [{"type":"FlexiPage","fullName":"CLM_Contract_Record_Page"}],
    "conflicts": [{"type":"ListView","fullName":"CLM_Contract__c.All"}]
  }
}`)
	conflicts, err := readSalesforceMetadataConflicts(output)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ListView:CLM_Contract__c.All"}
	if !slices.Equal(conflicts, want) {
		t.Fatalf("Salesforce metadata conflicts = %#v, want %#v", conflicts, want)
	}

	expected := map[string]bool{
		"FlexiPage:CLM_Contract_Record_Page": true,
		"ListView:CLM_Contract__c.All":       true,
	}
	existing := map[string]bool{}
	for _, component := range conflicts {
		existing[component] = true
	}
	missing := missingSalesforceComponents(expected, existing)
	if !slices.Equal(missing, []string{"FlexiPage:CLM_Contract_Record_Page"}) {
		t.Fatalf("missing Salesforce components = %#v", missing)
	}
	args := strings.Join(salesforceMetadataPreviewArgs("dispatch-scratch", missing), " ")
	if !strings.Contains(args, "project deploy preview") || !strings.Contains(args, "--metadata FlexiPage:CLM_Contract_Record_Page") {
		t.Fatalf("Salesforce metadata preview args = %q", args)
	}
}

func TestSalesforcePlanIncludesManagedPackageFromStart(t *testing.T) {
	root := t.TempDir()
	if err := solution.WriteBundled(root, "clm"); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "salesforce-project")
	if err := os.MkdirAll(filepath.Join(project, "force-app", "main", "default", "classes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "sfdx-project.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "force-app", "main", "default", "classes", "Demo.cls-meta.xml"), []byte(`<ApexClass xmlns="http://soap.sforce.com/2006/04/metadata"><apiVersion>66.0</apiVersion></ApexClass>`), 0o600); err != nil {
		t.Fatal(err)
	}
	item, err := PlanProvider(root, "salesforce")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(item.Planned, "Managed Package:Box for Salesforce 5.43") {
		t.Fatalf("Salesforce plan omitted managed package: %#v", item.Planned)
	}
	if !slices.ContainsFunc(item.Planned, func(component string) bool { return strings.HasPrefix(component, "ApexClass:") }) {
		t.Fatalf("Salesforce plan omitted local metadata categories: %#v", item.Planned)
	}
	if len(item.ComponentOrder) == 0 || item.ComponentOrder[0] != "Managed Package" {
		t.Fatalf("Salesforce component order = %#v", item.ComponentOrder)
	}
	if !slices.Contains(item.Planned, "Permission Set Assignment:Box Admin (All Licenses)") || !slices.Contains(item.Planned, "Permission Set Assignment:CLM Demo Operator") {
		t.Fatalf("Salesforce plan omitted required permission-set assignments: %#v", item.Planned)
	}
}

func TestSalesforcePackageInventoryRequiresExactVersion(t *testing.T) {
	required := []solution.SalesforcePackageRequirement{{
		Name: "Box for Salesforce", Namespace: "box", PackageID: "033700000004yvWAAQ",
		VersionID: "04tKi000000gPNZIA2", VersionName: "5.43", VersionNumber: "5.43.0.1",
	}}
	installed := []installedSalesforcePackage{{
		SubscriberPackageID: "033700000004yvWAAQ", SubscriberPackageName: "Box for Salesforce",
		SubscriberPackageNamespace: "box", SubscriberPackageVersionID: "04tOlder",
	}}
	if missing := missingSalesforcePackages(required, installed); len(missing) != 1 {
		t.Fatalf("older package satisfied exact prerequisite: %#v", missing)
	}
	installed[0].SubscriberPackageVersionID = "04tKi000000gPNZIA2"
	if missing := missingSalesforcePackages(required, installed); len(missing) != 0 {
		t.Fatalf("exact package version was reported missing: %#v", missing)
	}
}

func TestMissingSalesforcePackageBecomesDeployablePrerequisite(t *testing.T) {
	item := classifySalesforceInventory(
		Item{Provider: "salesforce", Name: "Salesforce metadata"},
		map[string]bool{"ApexClass:BoxEmailAttachmentUploader": true},
		map[string]bool{"ApexClass:BoxEmailAttachmentUploader": true},
		"scratch",
	)
	requirement := solution.SalesforcePackageRequirement{
		Name: "Box for Salesforce", Namespace: "box", VersionID: "04tKi000000gPNZIA2",
		VersionName: "5.43", VersionNumber: "5.43.0.1",
	}
	result := addMissingSalesforcePackages(item, []solution.SalesforcePackageRequirement{requirement}, "scratch")
	if result.Status != StatusMissing || !result.Deployable || !slices.Contains(result.Missing, "Managed Package:Box for Salesforce 5.43") {
		t.Fatalf("package prerequisite result = %#v", result)
	}
	for _, want := range []string{"Box for Salesforce 5.43.0.1", "before metadata deployment"} {
		if !strings.Contains(result.Detail, want) {
			t.Fatalf("detail %q does not contain %q", result.Detail, want)
		}
	}
}

func TestInstalledSalesforcePackageRemainsVisibleInChecklist(t *testing.T) {
	requirement := solution.SalesforcePackageRequirement{
		Name: "Box for Salesforce", Namespace: "box", VersionID: "04tKi000000gPNZIA2", VersionName: "5.43",
	}
	installed := []installedSalesforcePackage{{
		SubscriberPackageNamespace: "box", SubscriberPackageVersionID: "04tKi000000gPNZIA2",
	}}
	result := addSalesforcePackageResults(Item{Provider: "salesforce", Status: StatusPresent}, []solution.SalesforcePackageRequirement{requirement}, installed, "scratch")
	if !slices.Contains(result.Present, "Managed Package:Box for Salesforce 5.43") || len(result.Missing) != 0 {
		t.Fatalf("installed managed package checklist result = %#v", result)
	}
}

func TestSalesforcePackageInstallArgsAreNonInteractiveAndVersionPinned(t *testing.T) {
	requirement := solution.SalesforcePackageRequirement{VersionID: "04tKi000000gPNZIA2", SecurityType: "AdminsOnly"}
	args := strings.Join(salesforcePackageInstallArgs(requirement, "scratch"), " ")
	for _, want := range []string{"package install", "--package 04tKi000000gPNZIA2", "--target-org scratch", "--security-type AdminsOnly", "--wait 30", "--no-prompt", "--json"} {
		if !strings.Contains(args, want) {
			t.Fatalf("install args %q do not contain %q", args, want)
		}
	}
}

func TestDecodeSalesforceUserPermissionInventoryNormalizesNamespaces(t *testing.T) {
	payload := []byte(`{
  "status": 0,
  "result": {
    "records": [{
      "Profile": {"Name": "System Administrator"},
      "PermissionSetAssignments": {"records": [
        {"PermissionSet": {"Name": "Box_Admin_All_Licenses", "NamespacePrefix": "box"}},
        {"PermissionSet": {"Name": "CLM_Demo_Operator", "NamespacePrefix": null}}
      ]}
    }]
  }
}`)
	inventory, err := decodeSalesforceUserPermissionInventory(payload, "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Profile != "System Administrator" || !inventory.Assigned["box__box_admin_all_licenses"] || !inventory.Assigned["clm_demo_operator"] {
		t.Fatalf("permission inventory = %#v", inventory)
	}
}

func TestMissingSalesforcePermissionSetsBecomeDeployableChecklistRows(t *testing.T) {
	required := []solution.SalesforcePermissionSetRequirement{
		{Name: "box__Box_Admin_All_Licenses", Label: "Box Admin (All Licenses)"},
		{Name: "CLM_Demo_Operator", Label: "CLM Demo Operator"},
	}
	assigned := map[string]bool{"box__box_admin_all_licenses": true}
	result := addSalesforcePermissionSetResults(Item{Provider: "salesforce", Status: StatusPresent}, required, assigned, "scratch")
	if result.Status != StatusMissing || !result.Deployable {
		t.Fatalf("permission-set result = %#v, want deployable missing", result)
	}
	if !slices.Contains(result.Present, "Permission Set Assignment:Box Admin (All Licenses)") || !slices.Contains(result.Missing, "Permission Set Assignment:CLM Demo Operator") {
		t.Fatalf("permission-set checklist result = %#v", result)
	}
	if !strings.Contains(result.Detail, "authenticated System Administrator") {
		t.Fatalf("permission-set guidance is not explicit about its target user: %q", result.Detail)
	}
}

func TestSOQLStringEscaping(t *testing.T) {
	if got, want := escapeSOQLString(`admin'o\\example.test`), `admin\'o\\\\example.test`; got != want {
		t.Fatalf("escaped SOQL = %q, want %q", got, want)
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
	write("metadata-templates.json", `{"templates":[{"templateKey":"contract","displayName":"Contract"}]}`)
	write("docgen-template-data.json", `{"approvalMemo":{}}`)
	write("extract-field-prompts.json", `{"contractIntake":{}}`)
	for _, name := range []string{"folder-template.md", "hub-blueprint.md"} {
		write(name, "# Blueprint")
	}
	manifest, err := solution.LoadBundled("clm")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := boxComponentEntries(root, manifest, solution.ComponentSelection{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 8 {
		t.Fatalf("got %d Box components, want 8: %#v", len(entries), entries)
	}
	if slices.Contains(entries, "Automate Workflow:Intake") {
		t.Fatalf("validate-only Automate workflow leaked into package components: %#v", entries)
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
