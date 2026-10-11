package config

import "testing"

func TestObservedCheckPreservesRotatedTokensAndOtherProviderUpdates(t *testing.T) {
	baseline := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{ID: "box", ClientID: "client", SubjectID: "subject", RefreshToken: "original"}, true)
	baseline = baseline.UpsertSalesforceOrg(SalesforceOrgConnection{ID: "sf", OrgID: "org", InstanceURL: "https://org.test", AccessToken: "original-sf"}, true)
	next, current := CloneConnectionSettings(baseline), CloneConnectionSettings(baseline)
	snapshot := CloneConnectionSettings(baseline)
	next.ObservedConnectionState = &snapshot
	next.BoxConnections[0].VerifiedAt = "checked"
	current.BoxConnections[0].RefreshToken = "new-token"
	current.SalesforceOrgs[0].AccessToken = "new-sf-token"
	current = current.HydrateBoxConnections().HydrateSalesforceOrgs()
	merged, err := MergeObservedConnectionUpdate(next, current)
	if err != nil {
		t.Fatal(err)
	}
	if merged.BoxConnections[0].RefreshToken != "new-token" || merged.SalesforceOrgs[0].AccessToken != "new-sf-token" || merged.BoxConnections[0].VerifiedAt != "checked" {
		t.Fatal("observed check lost a concurrent update")
	}
	if merged.ObservedConnectionState != nil {
		t.Fatal("transient baseline escaped the update")
	}
}

func TestObservedSaveRejectsNewDestinationAndConflictingRefresh(t *testing.T) {
	baseline := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{ID: "box", ClientID: "client", SubjectID: "subject", RefreshToken: "original"}, true)
	next := CloneConnectionSettings(baseline)
	snapshot := CloneConnectionSettings(baseline)
	next.ObservedConnectionState = &snapshot
	next.BoxConnections[0].RefreshToken = "this-refresh"
	current := CloneConnectionSettings(baseline)
	current.BoxConnections[0].RefreshToken = "other-refresh"
	if _, err := MergeObservedConnectionUpdate(next, current); err == nil {
		t.Fatal("conflicting refresh overwrote the current token")
	}
	current = CloneConnectionSettings(baseline)
	current.BoxConnections[0].SubjectID = "new-subject"
	if _, err := MergeObservedConnectionUpdate(next, current); err == nil {
		t.Fatal("stale save restored a previous target")
	}
}

func TestDeploymentTargetFingerprintBindsEffectiveClientFallbacks(t *testing.T) {
	t.Setenv("BOX_CLIENT_ID", "box-fallback-a")
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{ID: "box", Identity: "owner", Account: "account", Enterprise: "enterprise", RefreshToken: "fixture-token"}, true)
	settings = settings.UpsertSalesforceOrg(SalesforceOrgConnection{ID: "sf", OrgID: "org", InstanceURL: "https://org.example.test"}, true)
	settings.SalesforceClientID = "sf-fallback-a"
	before := DeploymentTargetFingerprint(settings)
	t.Setenv("BOX_CLIENT_ID", "box-fallback-b")
	if DeploymentTargetFingerprint(settings) == before {
		t.Fatal("effective Box client change was missed")
	}
	t.Setenv("BOX_CLIENT_ID", "box-fallback-a")
	settings.SalesforceClientID = "sf-fallback-b"
	if DeploymentTargetFingerprint(settings) == before {
		t.Fatal("effective Salesforce client change was missed")
	}
}
