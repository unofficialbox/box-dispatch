package webapi

import (
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

func TestSalesforceRESTInputAcceptsTargetOrDevHub(t *testing.T) {
	cases := []struct {
		name  string
		input salesforceRESTInput
		ok    bool
	}{
		{name: "target", input: salesforceRESTInput{InstanceURL: "https://org.example.com", AccessToken: "token"}, ok: true},
		{name: "dev hub", input: salesforceRESTInput{DevHubURL: "https://hub.example.com", DevHubToken: "token", ClientID: "client"}, ok: true},
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
