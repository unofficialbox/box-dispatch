package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

func TestSalesforceRESTCredentialKeepsSecretsInLifecycleBoundary(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceInstanceURL:  "https://example.my.salesforce.com",
		SalesforceAccessToken:  "secret-token",
		SalesforceClientID:     "client-id",
		SalesforceClientSecret: "client-secret",
	}
	credential := salesforceRESTCredential(settings)
	if credential.InstanceURL != settings.SalesforceInstanceURL || credential.AccessToken != "secret-token" || credential.ClientID != "client-id" || credential.ClientSecret != "client-secret" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestSalesforceRESTCredentialUsesSelectedBrowserOAuthConnection(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "fallback-client"}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "selected", InstanceURL: "https://selected.example.com", AccessToken: "selected-token", ClientID: "selected-client",
	}, true)
	credential := salesforceRESTCredential(settings)
	if credential.InstanceURL != "https://selected.example.com" || credential.AccessToken != "selected-token" || credential.ClientID != "selected-client" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestSalesforceRESTCredentialDoesNotMixSelectedClientWithGlobalSecret(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceClientID: "custom-client", SalesforceClientSecret: "custom-secret",
	}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "selected", InstanceURL: "https://selected.example.com", AccessToken: "selected-token",
		RefreshToken: "selected-refresh", ClientID: salesforceapi.LoginClientID,
	}, true)

	credential := salesforceRESTCredential(settings)
	if credential.ClientID != salesforceapi.LoginClientID || credential.ClientSecret != "" {
		t.Fatalf("credential mixed OAuth client records: %#v", credential)
	}
}

func TestSalesforceRESTCredentialDefaultsToPlatformCLIClient(t *testing.T) {
	credential := salesforceRESTCredential(config.ConnectionSettings{SalesforceInstanceURL: "https://example.com", SalesforceAccessToken: "token"})
	if credential.ClientID != salesforceapi.LoginClientID {
		t.Fatalf("client ID = %q", credential.ClientID)
	}
}

func TestSalesforceRESTSessionRestoresStoredUsernameWhenUserInfoIsUnavailable(t *testing.T) {
	settings := config.ConnectionSettings{
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {Identity: "verified@example.com"},
		},
	}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "selected", Username: "scratch@example.com", OrgID: "00Dscratch",
		InstanceURL: "https://scratch.example.com", AccessToken: "token",
	}, true)
	session := newSalesforceRESTSession(settings)
	status := session.enrichOrgStatus(salesforceapi.OrgStatus{Available: true, Status: "Ready"})
	if status.Username != "scratch@example.com" || status.OrgID != "00Dscratch" || status.InstanceURL != "https://scratch.example.com" {
		t.Fatalf("status = %#v", status)
	}
}

func TestSalesforceSessionExpiredRecognizesSalesforceSessionFailures(t *testing.T) {
	for _, message := range []string{"INVALID_SESSION_ID: Session expired or invalid", "session invalid"} {
		if !salesforceSessionExpired(errors.New(message)) {
			t.Fatalf("expected session failure for %q", message)
		}
	}
	if salesforceSessionExpired(errors.New("managed package is missing")) {
		t.Fatal("package error must not be treated as an expired session")
	}
}

func TestBoxOAuthSessionExpiredRecognizesRejectedRefreshTokens(t *testing.T) {
	if !boxOAuthSessionExpired(errors.New(`Box OAuth2 token request returned 400: invalid_grant: Refresh token has expired`)) {
		t.Fatal("expected invalid_grant to require a Box reconnect")
	}
	if boxOAuthSessionExpired(errors.New("Box folder is unavailable")) {
		t.Fatal("folder error must not be treated as an expired Box OAuth session")
	}
}

func TestInstalledSalesforcePackagesPreserveRequirementIdentity(t *testing.T) {
	packages := installedSalesforcePackages([]salesforceapi.InstalledPackage{{
		PackageID: "033-package", Name: "Box for Salesforce", Namespace: "box",
		VersionID: "04t-version", VersionName: "5.43", VersionNumber: "5.43.0.1",
	}})
	if len(packages) != 1 || packages[0].SubscriberPackageID != "033-package" || packages[0].SubscriberPackageNamespace != "box" || packages[0].SubscriberPackageVersionID != "04t-version" || packages[0].SubscriberPackageVersionNumber != "5.43.0.1" {
		t.Fatalf("packages = %#v", packages)
	}
}

func TestMetadataTypesAreUniqueAndSorted(t *testing.T) {
	components := []string{"CustomField:Contract__c.Status__c", "ApexClass:Controller", "CustomField:Contract__c.Value__c", "invalid"}
	want := []string{"ApexClass", "CustomField"}
	if got := metadataTypes(components); !slices.Equal(got, want) {
		t.Fatalf("metadataTypes() = %#v, want %#v", got, want)
	}
}

func TestSalesforceRESTFailureIncludesBrowserVisibleDiagnostic(t *testing.T) {
	item := salesforceRESTFailure(Item{Provider: "salesforce"}, "Unable to read Salesforce metadata", errors.New("INVALID_SESSION_ID: Session expired"))
	if item.Status != StatusFailed || !strings.Contains(item.Detail, "Session expired") || !strings.Contains(item.Diagnostic, "INVALID_SESSION_ID") {
		t.Fatalf("item = %#v", item)
	}
}

func TestSalesforceRESTLabelUsesConfiguredAliasBeforeIdentity(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceAlias: "dispatch-scratch"}
	org := salesforceapi.OrgStatus{Username: "user@example.com", OrgID: "00Dorg"}
	if got := salesforceRESTLabel(settings, org); got != "dispatch-scratch" {
		t.Fatalf("salesforceRESTLabel() = %q", got)
	}
}

func TestSalesforceMetadataStagesEnableExperienceBeforeDeployingTheSite(t *testing.T) {
	stages := salesforceMetadataStages([]string{
		"Network:CLM_Experience",
		"UIBundle:clmreactapp",
		"Settings:Communities",
		"DigitalExperience:site/CLM_Experience1.sfdc_cms__site/CLM_Experience1",
		"DigitalExperienceBundle:site/CLM_Experience1",
		"CustomObject:CLM_Contract__c",
		"DigitalExperienceConfig:CLM_Experience1",
		"CustomSite:CLM_Experience",
		"Settings:ExperienceBundle",
	})
	if len(stages) != 3 || stages[0].Label != "org settings" || stages[1].Label != "application metadata" || stages[2].Label != "Experience Cloud site" {
		t.Fatalf("stages = %#v", stages)
	}
	if !slices.Equal(stages[0].Components, []string{"Settings:Communities", "Settings:ExperienceBundle"}) {
		t.Fatalf("settings stage = %#v", stages[0].Components)
	}
	if !slices.Equal(stages[1].Components, []string{"CustomObject:CLM_Contract__c", "UIBundle:clmreactapp"}) {
		t.Fatalf("application stage = %#v", stages[1].Components)
	}
	if !slices.Equal(stages[2].Components, []string{"CustomSite:CLM_Experience", "DigitalExperience:site/CLM_Experience1.sfdc_cms__site/CLM_Experience1", "DigitalExperienceBundle:site/CLM_Experience1", "DigitalExperienceConfig:CLM_Experience1", "Network:CLM_Experience"}) {
		t.Fatalf("site stage = %#v", stages[2].Components)
	}
}

func TestSalesforceExperienceDeploymentAlwaysIncludesEnablingSettings(t *testing.T) {
	inventory := map[string]bool{
		"Settings:Communities":                         true,
		"Settings:ExperienceBundle":                    true,
		"Network:CLM_Experience":                       true,
		"DigitalExperienceBundle:site/CLM_Experience1": true,
	}
	components := includeSalesforceOrgSettings([]string{"Network:CLM_Experience"}, inventory)
	want := []string{"Network:CLM_Experience", "Settings:Communities", "Settings:ExperienceBundle"}
	if !slices.Equal(components, want) {
		t.Fatalf("components = %#v, want %#v", components, want)
	}
	if !sourceIncludesSalesforceExperience(inventory) {
		t.Fatal("expected the source inventory to identify an Experience Cloud application")
	}
}

func TestSalesforceMultiFrameworkPreflightOnlyAppliesToUIBundles(t *testing.T) {
	if !sourceRequiresSalesforceMultiFramework(map[string]bool{"UIBundle:clmreactapp": true}) {
		t.Fatal("expected a UIBundle to require the Multi-Framework preflight")
	}
	if sourceRequiresSalesforceMultiFramework(map[string]bool{"ApexClass:ContractController": true}) {
		t.Fatal("ordinary Salesforce metadata must not require the Multi-Framework preflight")
	}
}

func TestSalesforceMultiFrameworkPreflightRejectsFirstPartyOrg(t *testing.T) {
	err := salesforceMultiFrameworkCompatibilityError(salesforceapi.MultiFrameworkEligibility{
		InstanceName: "CS248", LanguageLocale: "en_US", APIVersion: "67.0", Hyperforce: false, EnglishDefault: true, SupportedRelease: true,
	})
	if err == nil {
		t.Fatal("expected a first-party org to fail the Multi-Framework preflight")
	}
	for _, want := range []string{"Hyperforce", "CS248", "Hyperforce Dev Hub", "validation"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error omitted %q: %v", want, err)
		}
	}
}

func TestSalesforceMultiFrameworkPreflightRequiresEnglishDefault(t *testing.T) {
	err := salesforceMultiFrameworkCompatibilityError(salesforceapi.MultiFrameworkEligibility{
		InstanceName: "DEU52", LanguageLocale: "de_DE", APIVersion: "67.0", Hyperforce: true, EnglishDefault: false, SupportedRelease: true,
	})
	if err == nil || !strings.Contains(err.Error(), "English") || !strings.Contains(err.Error(), "de_DE") {
		t.Fatalf("error = %v", err)
	}
	if err := salesforceMultiFrameworkCompatibilityError(salesforceapi.MultiFrameworkEligibility{
		InstanceName: "USA470", LanguageLocale: "en_US", APIVersion: "67.0", Hyperforce: true, EnglishDefault: true, SupportedRelease: true,
	}); err != nil {
		t.Fatalf("eligible org failed preflight: %v", err)
	}
}

func TestSalesforceMultiFrameworkPreflightRequiresSummer26(t *testing.T) {
	err := salesforceMultiFrameworkCompatibilityError(salesforceapi.MultiFrameworkEligibility{
		InstanceName: "USA470", LanguageLocale: "en_US", APIVersion: "66.0", Hyperforce: true, EnglishDefault: true,
	})
	if err == nil || !strings.Contains(err.Error(), "Summer '26") || !strings.Contains(err.Error(), "66.0") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckSalesforceMultiFrameworkCompatibilityReadsUIBundleTargetOrg(t *testing.T) {
	project := t.TempDir()
	descriptor := filepath.Join(project, "force-app", "main", "default", "uiBundles", "clmreactapp", "clmreactapp.uibundle-meta.xml")
	if err := os.MkdirAll(filepath.Dir(descriptor), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor, []byte(`<UIBundle xmlns="http://soap.sforce.com/2006/04/metadata"><target>Experience</target></UIBundle>`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0"}})
		case "/services/data/v67.0/query":
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"InstanceName": "CS248", "LanguageLocaleKey": "en_US"}}})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	session := &salesforceRESTSession{
		credential: salesforceapi.Credential{InstanceURL: server.URL, AccessToken: "token"},
		client:     &salesforceapi.Client{HTTP: server.Client()},
	}
	var updates []ProgressUpdate
	err := checkSalesforceMultiFrameworkCompatibility(context.Background(), session, project, func(update ProgressUpdate) {
		updates = append(updates, update)
	})
	if err == nil || !strings.Contains(err.Error(), "CS248") {
		t.Fatalf("error = %v", err)
	}
	if len(updates) != 1 || updates[0].Message != "Checking Salesforce Multi-Framework compatibility" {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestSalesforceExperiencePublishUsesSourceInventoryAfterSiteMetadataMatches(t *testing.T) {
	inventory := map[string]bool{
		"Network:CLM_Experience":                       true,
		"DigitalExperienceBundle:site/CLM_Experience1": true,
	}
	metadata := includeSalesforceOrgSettings(nil, inventory)
	if hasSalesforceExperienceSiteMetadata(metadata) {
		t.Fatalf("settings-only deployment unexpectedly contains site metadata: %#v", metadata)
	}
	if !sourceIncludesSalesforceExperience(inventory) {
		t.Fatal("expected matching source site metadata to retain publish intent")
	}
	name, err := salesforceExperienceNetworkName(sortedInventoryComponents(inventory))
	if err != nil || name != "CLM_Experience" {
		t.Fatalf("network name = %q err = %v", name, err)
	}
}

func TestSalesforcePackageInstallMessageSurfacesStatusAndElapsedTime(t *testing.T) {
	message := salesforcePackageInstallMessage(salesforceapi.PackageInstallProgress{RequestID: "0Hf123", Status: "IN_PROGRESS", Polls: 4, Elapsed: 2*time.Minute + 7*time.Second})
	if message != "Salesforce reports in progress · 2m7s elapsed" {
		t.Fatalf("message = %q", message)
	}
}

func TestSalesforceExperienceNetworkNameRequiresOneNetwork(t *testing.T) {
	name, err := salesforceExperienceNetworkName([]string{
		"DigitalExperienceBundle:site/CLM_Experience1",
		"Network:CLM_Experience",
		"UIBundle:clmreactapp",
	})
	if err != nil || name != "CLM_Experience" {
		t.Fatalf("name = %q err = %v", name, err)
	}
	if _, err := salesforceExperienceNetworkName([]string{"UIBundle:clmreactapp"}); err == nil {
		t.Fatal("expected a missing Network to fail")
	}
}

func TestSalesforceSeedScriptPathRejectsTraversal(t *testing.T) {
	if _, err := salesforceSeedScriptPath(t.TempDir(), "../seed.apex"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	project := t.TempDir()
	path, err := salesforceSeedScriptPath(project, "scripts/seed.apex")
	if err != nil || !strings.HasSuffix(path, "scripts/seed.apex") {
		t.Fatalf("path = %q err = %v", path, err)
	}
}

func TestSalesforceSeedValidationDefersQueryUntilMissingObjectIsDeployed(t *testing.T) {
	item := Item{
		Provider:             "salesforce",
		Status:               StatusMissing,
		Deployable:           true,
		Missing:              []string{"CustomObject:CLM_Contract__c"},
		DeployableComponents: []string{"CustomObject:CLM_Contract__c"},
	}
	scripts := []solution.SalesforceSeedScript{{
		Label: "CLM sample contract records", Object: "CLM_Contract__c", ExternalIDField: "Contract_ID__c",
		ExternalIDs: []string{"CLM-1", "CLM-2"},
	}}
	var updates []ProgressUpdate

	result, err := inspectSalesforceSeedScripts(context.Background(), &salesforceapi.Client{}, salesforceapi.Credential{}, item, scripts, "scratch-org", func(update ProgressUpdate) {
		updates = append(updates, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	component := "Sample Records:CLM sample contract records"
	if !slices.Contains(result.Missing, component) || !slices.Contains(result.DeployableComponents, component) {
		t.Fatalf("result = %#v", result)
	}
	if len(updates) != 1 || updates[0].Component != component || updates[0].State != ProgressCompleted || !strings.Contains(updates[0].Message, "after the object is deployed") {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestSalesforceSeedValidationDefersQueryUntilPermissionsAreAssigned(t *testing.T) {
	item := Item{
		Provider:             "salesforce",
		Status:               StatusMissing,
		Deployable:           true,
		Missing:              []string{"Permission Set Assignment:CLM Demo Operator"},
		DeployableComponents: []string{"Permission Set Assignment:CLM Demo Operator"},
	}
	scripts := []solution.SalesforceSeedScript{{
		Label: "CLM sample contract records", Object: "CLM_Contract__c", ExternalIDField: "Contract_ID__c",
		ExternalIDs: []string{"CLM-1", "CLM-2"},
	}}
	var updates []ProgressUpdate

	result, err := inspectSalesforceSeedScripts(context.Background(), &salesforceapi.Client{}, salesforceapi.Credential{}, item, scripts, "scratch-org", func(update ProgressUpdate) {
		updates = append(updates, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	component := "Sample Records:CLM sample contract records"
	if !slices.Contains(result.Missing, component) || !slices.Contains(result.DeployableComponents, component) {
		t.Fatalf("result = %#v", result)
	}
	if len(updates) != 1 || updates[0].Component != component || updates[0].State != ProgressCompleted || !strings.Contains(updates[0].Message, "after required permissions are assigned") {
		t.Fatalf("updates = %#v", updates)
	}
}
