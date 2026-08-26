package config

import "testing"

func TestUpsertSalesforceOrgKeepsExistingOrgsAndSelectsNewLogin(t *testing.T) {
	settings := ConnectionSettings{SalesforceClientID: "dispatch-app"}
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub@example.com", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", RefreshToken: "hub-refresh",
		OrgType: "persistent", Status: "Active", DevHub: true,
	}, true)
	if !settings.HasSalesforceDevHub() || settings.SalesforceDevHubToken != "hub-token" || settings.SalesforceAccessToken != "hub-token" {
		t.Fatalf("explicit Dev Hub should stay the Dev Hub and selection: %#v", settings)
	}

	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "scratch@example.com", Username: "scratch@example.com", OrgID: "00Dscratch",
		InstanceURL: "https://scratch.example.com", AccessToken: "scratch-token",
		OrgType: "scratch", Status: "Active",
	}, true)
	if len(settings.SalesforceOrgs) != 2 {
		t.Fatalf("orgs = %#v", settings.SalesforceOrgs)
	}
	if settings.SalesforceAccessToken != "scratch-token" || settings.SalesforceDevHubToken != "hub-token" {
		t.Fatalf("scratch select overwrote the Dev Hub: %#v", settings)
	}

	selected, err := settings.SelectSalesforceOrg(settings.SalesforceOrgs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.SalesforceAccessToken != "hub-token" || selected.SalesforceDevHubToken != "hub-token" {
		t.Fatalf("select hub = %#v", selected)
	}
}

func TestUpsertSalesforceOrgKeepsCompletedOrgWhenNewLoginHasNoOrgID(t *testing.T) {
	settings := ConnectionSettings{SalesforceClientID: "app"}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent",
	}, true)
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "Connected Salesforce org", InstanceURL: "https://prod.example.com",
		AccessToken: "prod-token", OrgType: "persistent",
	}, true)
	if len(settings.SalesforceOrgs) != 2 || settings.SalesforceOrgs[0].OrgID != "00Dhub" || settings.SalesforceAccessToken != "prod-token" {
		t.Fatalf("second login replaced the Dev Hub: %#v", settings)
	}
}

func TestUpsertSalesforceOrgKeepsPerOrgConsumerKeys(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", OrgID: "00Dhub", InstanceURL: "https://hub.example.com",
		AccessToken: "hub-token", ClientID: "key-a", OrgType: "persistent",
	}, true)
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "prod", OrgID: "00Dprod", InstanceURL: "https://prod.example.com",
		AccessToken: "prod-token", ClientID: "key-b", OrgType: "persistent",
	}, true)
	if len(settings.SalesforceOrgs) != 2 || settings.SalesforceOrgs[0].ClientID != "key-a" || settings.SalesforceOrgs[1].ClientID != "key-b" {
		t.Fatalf("client IDs = %#v", settings.SalesforceOrgs)
	}
}

func TestUpsertSalesforceOrgClearsSecretWhenOAuthClientChanges(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "prod", OrgID: "00Dprod", InstanceURL: "https://prod.example.com",
		AccessToken: "old-token", ClientID: "custom-client", ClientSecret: "custom-secret", OrgType: "persistent",
	}, true)
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "prod", OrgID: "00Dprod", InstanceURL: "https://prod.example.com",
		AccessToken: "oauth-token", RefreshToken: "oauth-refresh", ClientID: "PlatformCLI", OrgType: "persistent",
	}, true)
	selected, ok := settings.SelectedSalesforceOrg()
	if !ok || selected.ClientID != "PlatformCLI" || selected.ClientSecret != "" {
		t.Fatalf("selected org retained an incompatible client secret: %#v", selected)
	}
}

func TestInvalidateSelectedSalesforceVerificationKeepsConnection(t *testing.T) {
	settings := ConnectionSettings{
		VerifiedConnections: map[string]VerifiedConnection{
			"box":        {VerifiedAt: "2026-08-25T12:00:00Z"},
			"salesforce": {VerifiedAt: "2026-08-25T12:00:00Z"},
		},
	}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "prod", OrgID: "00Dprod", InstanceURL: "https://prod.example.com",
		AccessToken: "stale-token", RefreshToken: "refresh-token", Status: "Active",
	}, true)

	invalidated := settings.InvalidateSelectedSalesforceVerification("Unavailable")
	selected, ok := invalidated.SelectedSalesforceOrg()
	if !ok || selected.Status != "Unavailable" || selected.AccessToken != "stale-token" {
		t.Fatalf("selected org = %#v", selected)
	}
	if _, verified := invalidated.VerifiedConnections["salesforce"]; verified {
		t.Fatalf("Salesforce readiness was retained: %#v", invalidated.VerifiedConnections)
	}
	if invalidated.VerifiedConnections["box"].VerifiedAt == "" {
		t.Fatalf("Box readiness was cleared: %#v", invalidated.VerifiedConnections)
	}
}

func TestRemoveSalesforceOrgKeepsTheOtherOrg(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent", DevHub: true,
	}, true)
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "scratch", Username: "scratch@example.com", OrgID: "00Dscratch",
		InstanceURL: "https://scratch.example.com", AccessToken: "scratch-token", OrgType: "scratch",
	}, true)
	removed, err := settings.RemoveSalesforceOrg(settings.SalesforceSelectedOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.SalesforceOrgs) != 1 || removed.SalesforceAccessToken != "hub-token" || !removed.HasSalesforceDevHub() {
		t.Fatalf("remaining = %#v", removed)
	}
}

func TestRemoveSalesforceOrgClearsTheLastOrg(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent",
	}, true)
	cleared, err := settings.RemoveSalesforceOrg(settings.SalesforceSelectedOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.HasSalesforceREST() || cleared.HasSalesforceDevHub() || len(cleared.SalesforceOrgs) != 0 || cleared.SalesforceAlias != "" {
		t.Fatalf("last org should be cleared: %#v", cleared)
	}
}

func TestDevHubOrgRequiresAnExplicitDevHubFlag(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "prod", Username: "prod@example.com", OrgID: "00Dprod",
		InstanceURL: "https://prod.example.com", AccessToken: "prod-token", OrgType: "persistent",
	}, true)
	if settings.HasSalesforceDevHub() {
		t.Fatalf("a regular org login should not become the Dev Hub: %#v", settings)
	}
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent", DevHub: true,
	}, false)
	hub, ok := settings.DevHubOrg()
	if !ok || hub.OrgID != "00Dhub" || settings.SalesforceAccessToken != "prod-token" {
		t.Fatalf("Dev Hub login should not replace the selected org: %#v", settings)
	}
}

func TestMarkSelectedAsDevHubIgnoresScratchOrgs(t *testing.T) {
	settings := ConnectionSettings{}.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "scratch", Username: "scratch@example.com", OrgID: "00Dscratch",
		InstanceURL: "https://scratch.example.com", AccessToken: "scratch-token", OrgType: "scratch",
	}, true)
	if settings.MarkSelectedAsDevHub().HasSalesforceDevHub() {
		t.Fatalf("scratch orgs cannot be the Dev Hub: %#v", settings)
	}
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{
		Alias: "hub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent",
	}, true)
	marked := settings.MarkSelectedAsDevHub()
	hub, ok := marked.DevHubOrg()
	if !ok || hub.OrgID != "00Dhub" || marked.SalesforceAccessToken != "hub-token" {
		t.Fatalf("selected persistent org should become the Dev Hub: %#v", marked)
	}
}

func TestHydrateSalesforceOrgsSplitsLegacyTargetAndDevHub(t *testing.T) {
	settings := ConnectionSettings{
		SalesforceAlias: "Connected Salesforce org", SalesforceInstanceURL: "https://org.example.com",
		SalesforceAccessToken: "org-token", SalesforceDevHubURL: "https://hub.example.com",
		SalesforceDevHubToken: "hub-token", SalesforceClientID: "client",
	}.HydrateSalesforceOrgs()
	if len(settings.SalesforceOrgs) != 2 || settings.SalesforceDevHubToken != "hub-token" || settings.SalesforceAccessToken != "org-token" {
		t.Fatalf("hydrated = %#v", settings)
	}
}
