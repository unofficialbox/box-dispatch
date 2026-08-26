package webapi

import (
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/solution"
)

func TestSalesforceRESTInputAcceptsTargetOrDevHub(t *testing.T) {
	cases := []struct {
		name  string
		input salesforceRESTInput
		ok    bool
	}{
		{name: "target", input: salesforceRESTInput{InstanceURL: "https://org.example.com", AccessToken: "token"}, ok: true},
		{name: "dev hub", input: salesforceRESTInput{DevHubURL: "https://hub.example.com", DevHubToken: "token"}, ok: true},
		{name: "partial target", input: salesforceRESTInput{InstanceURL: "https://org.example.com"}},
		{name: "partial dev hub", input: salesforceRESTInput{DevHubURL: "https://hub.example.com", ClientID: "client"}},
		{name: "empty", input: salesforceRESTInput{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.input.validate(); (err == nil) != test.ok {
				t.Fatalf("validate() error = %v, want valid=%t", err, test.ok)
			}
		})
	}
}

func TestSaveSalesforceRESTAssignsDefaultAliasForTargetOrg(t *testing.T) {
	got := saveSalesforceREST(config.ConnectionSettings{}, salesforceRESTInput{InstanceURL: "https://org.example.com", AccessToken: "token"})
	if got.SalesforceAlias != "Connected Salesforce org" || got.SalesforceOrgStatus != "Needs availability check" {
		t.Fatalf("got = %#v", got)
	}
}

func TestSaveSalesforceRESTPreservesUnsubmittedCredentialGroup(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceInstanceURL: "https://org.example.com",
		SalesforceAccessToken: "org-token",
		SalesforceDevHubURL:   "https://hub.example.com",
		SalesforceDevHubToken: "hub-token",
		SalesforceClientID:    "client",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {VerifiedAt: "2026-08-22T12:00:00Z"},
		},
	}

	got := saveSalesforceREST(settings, salesforceRESTInput{DevHubURL: "https://new-hub.example.com", DevHubToken: "new-token", ClientID: "new-client"})
	if got.SalesforceInstanceURL != settings.SalesforceInstanceURL || got.SalesforceAccessToken != settings.SalesforceAccessToken {
		t.Fatalf("target credential was overwritten: %#v", got)
	}
	if _, ok := got.VerifiedConnections["salesforce"]; !ok {
		t.Fatal("Dev Hub-only update invalidated the target-org verification")
	}
}

func TestSaveSalesforceRESTClearsRefreshTokensForPastedCredentials(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceRefreshToken:       "org-refresh",
		SalesforceDevHubRefreshToken: "hub-refresh",
	}
	got := saveSalesforceREST(settings, salesforceRESTInput{
		InstanceURL: "https://org.example.com", AccessToken: "pasted-token",
		DevHubURL: "https://hub.example.com", DevHubToken: "hub-token", ClientID: "client",
	})
	if got.SalesforceRefreshToken != "" || got.SalesforceDevHubRefreshToken != "" {
		t.Fatalf("pasted credentials kept OAuth refresh tokens: %#v", got)
	}
}

func TestTargetCredentialDoesNotMixSelectedClientWithGlobalSecret(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceClientID: "custom-client", SalesforceClientSecret: "custom-secret",
	}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "selected", InstanceURL: "https://selected.example.com", AccessToken: "selected-token",
		RefreshToken: "selected-refresh", ClientID: salesforceapi.LoginClientID,
	}, true)

	credential := targetCredential(settings)
	if credential.ClientID != salesforceapi.LoginClientID || credential.ClientSecret != "" {
		t.Fatalf("credential mixed OAuth client records: %#v", credential)
	}
}

func TestSalesforcePackageInstalledAcceptsRequiredOrNewerVersion(t *testing.T) {
	requirement := solution.SalesforcePackageRequirement{Namespace: "box", VersionID: "04trequired", VersionNumber: "5.43.0.1"}
	for _, test := range []struct {
		name      string
		installed salesforceapi.InstalledPackage
		want      bool
	}{
		{name: "exact version id", installed: salesforceapi.InstalledPackage{Namespace: "box", VersionID: "04trequired", VersionNumber: "5.43.0.1"}, want: true},
		{name: "newer version", installed: salesforceapi.InstalledPackage{Namespace: "box", VersionID: "04tnewer", VersionNumber: "5.44.0.1"}, want: true},
		{name: "older version", installed: salesforceapi.InstalledPackage{Namespace: "box", VersionID: "04tolder", VersionNumber: "5.42.0.1"}, want: false},
		{name: "wrong package", installed: salesforceapi.InstalledPackage{Namespace: "other", VersionID: "04trequired", VersionNumber: "5.43.0.1"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := salesforcePackageInstalled(requirement, []salesforceapi.InstalledPackage{test.installed}); got != test.want {
				t.Fatalf("salesforcePackageInstalled() = %t, want %t", got, test.want)
			}
		})
	}
}
