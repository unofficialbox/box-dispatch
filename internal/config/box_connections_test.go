package config

import "testing"

func TestUpsertBoxConnectionKeepsExistingAppsAndSelectsNew(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a",
		SubjectType: "enterprise", SubjectID: "111",
	}, true)
	if !settings.HasBoxCCG() || settings.BoxCCGClientID != "client-a" || settings.BoxCCGAlias != "Legal Box" {
		t.Fatalf("first app should become the selection: %#v", settings)
	}

	settings = settings.UpsertBoxConnection(BoxAppConnection{
		Alias: "Finance Box", ClientID: "client-b", ClientSecret: "secret-b",
		SubjectType: "user", SubjectID: "222",
	}, true)
	if len(settings.BoxConnections) != 2 {
		t.Fatalf("apps = %#v", settings.BoxConnections)
	}
	if settings.BoxCCGClientID != "client-b" || settings.BoxCCGAlias != "Finance Box" {
		t.Fatalf("second save should select the new app: %#v", settings)
	}

	selected, err := settings.SelectBoxConnection(settings.BoxConnections[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.BoxCCGClientID != "client-a" || selected.BoxCCGAlias != "Legal Box" {
		t.Fatalf("select first = %#v", selected)
	}
}

func TestUpsertBoxConnectionKeepsSameCredentialsUnderNewAlias(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a",
		SubjectType: "user", SubjectID: "111",
	}, true)
	settings = settings.UpsertBoxConnection(BoxAppConnection{
		Alias: "Finance Box", ClientID: "client-a", ClientSecret: "secret-a",
		SubjectType: "user", SubjectID: "111",
	}, true)
	if len(settings.BoxConnections) != 2 || settings.BoxCCGAlias != "Finance Box" || settings.BoxConnections[0].Alias != "Legal Box" {
		t.Fatalf("second alias should add a connection: %#v", settings.BoxConnections)
	}
}

func TestRemoveBoxConnectionKeepsTheOtherApp(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a",
		SubjectType: "enterprise", SubjectID: "111",
	}, true)
	settings = settings.UpsertBoxConnection(BoxAppConnection{
		Alias: "Finance Box", RefreshToken: "refresh-b", Identity: "finance@example.com", Account: "222",
	}, true)
	removed, err := settings.RemoveBoxConnection(settings.BoxSelectedConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.BoxConnections) != 1 || removed.BoxCCGAlias != "Legal Box" || removed.BoxConnections[0].Alias != "Legal Box" {
		t.Fatalf("remaining = %#v", removed)
	}
}

func TestRemoveBoxConnectionClearsTheLastApp(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", RefreshToken: "refresh", Identity: "box-user@example.com", Account: "123",
	}, true)
	settings = settings.MarkBoxVerified(VerifiedConnection{VerifiedAt: "2026-08-23T20:00:00Z", Identity: "box-user@example.com"})
	cleared, err := settings.RemoveBoxConnection(settings.BoxSelectedConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.HasBoxConnection() || cleared.BoxCCGAlias != "" || cleared.VerifiedConnections["box"].VerifiedAt != "" {
		t.Fatalf("last connection should be cleared: %#v", cleared)
	}
}

func TestMarkBoxVerifiedPersistsRotatedOAuthRefresh(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", RefreshToken: "old-refresh", Identity: "box-user@example.com", Account: "12345",
	}, true)
	settings = settings.MarkBoxVerified(VerifiedConnection{
		VerifiedAt: "2026-08-23T20:00:00Z", Identity: "box-user@example.com", Account: "12345",
		RefreshToken: "new-refresh", AuthType: "OAuth2",
	})
	if settings.BoxConnections[0].RefreshToken != "new-refresh" || !settings.HasBoxOAuth() {
		t.Fatalf("rotated refresh was not saved: %#v", settings.BoxConnections[0])
	}
}

func TestMarkSelectedBoxUnverifiedKeepsOAuthConnectionForReconnect(t *testing.T) {
	settings := ConnectionSettings{}.UpsertBoxConnection(BoxAppConnection{
		Alias: "Legal Box", RefreshToken: "refresh", Identity: "box-user@example.com", Account: "12345",
	}, true)
	settings = settings.MarkBoxVerified(VerifiedConnection{VerifiedAt: "2026-08-23T20:00:00Z", Identity: "box-user@example.com"})
	updated := settings.MarkSelectedBoxUnverified()
	selected, ok := updated.SelectedBoxConnection()
	if !ok || selected.RefreshToken != "refresh" || selected.VerifiedAt != "" || selected.Identity != "box-user@example.com" {
		t.Fatalf("selected connection = %#v", selected)
	}
	if _, verified := updated.VerifiedConnections["box"]; verified {
		t.Fatalf("stale verification was retained: %#v", updated.VerifiedConnections)
	}
}

func TestHydrateBoxConnectionsPromotesLegacyCCG(t *testing.T) {
	settings := ConnectionSettings{
		BoxCCGAlias: "Legal Box", BoxCCGClientID: "client-id", BoxCCGClientSecret: "secret",
		BoxCCGSubjectType: "user", BoxCCGSubjectID: "12345",
		VerifiedConnections: map[string]VerifiedConnection{"box": {VerifiedAt: "2026-08-22T20:00:00Z", Identity: "box-user"}},
	}.HydrateBoxConnections()
	if len(settings.BoxConnections) != 1 || settings.BoxSelectedConnectionID != "legacy-box" {
		t.Fatalf("hydrated = %#v", settings)
	}
	if settings.BoxConnections[0].VerifiedAt == "" || settings.BoxCCGClientSecret != "secret" {
		t.Fatalf("legacy snapshot was dropped: %#v", settings.BoxConnections[0])
	}
}
