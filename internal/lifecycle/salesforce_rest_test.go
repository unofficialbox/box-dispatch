package lifecycle

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
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
