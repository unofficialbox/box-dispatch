package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckUsesRESTUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/oauth2/userinfo" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"organization_id": "00D123", "preferred_username": "admin@example.com"})
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !status.Available || status.OrgID != "00D123" || status.Username != "admin@example.com" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCreateScratchUsesRESTAndExchangesAuthorizationCode(t *testing.T) {
	getCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "64.0"}, {"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["ConnectedAppConsumerKey"] != "client-id" || input["DurationDays"] != float64(30) {
				t.Fatalf("input = %#v", input)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "2SR123", "success": true})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/2SR123":
			getCount++
			status := "Pending"
			if getCount > 1 {
				status = "Active"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": status, "SignupUsername": "scratch@example.com", "ScratchOrg": "00Dscratch", "LoginUrl": server.URL, "AuthCode": "auth-code", "ExpirationDate": "2026-09-21"})
		case "/services/oauth2/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("code") != "auth-code" || r.Form.Get("client_id") != "client-id" {
				t.Fatalf("form = %#v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "scratch-token", "instance_url": server.URL})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{HTTP: server.Client(), PollInterval: time.Millisecond}
	org, err := client.CreateScratch(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "hub-token", ClientID: "client-id"}, ScratchRequest{Alias: "dispatch-demo"})
	if err != nil || org.Alias != "dispatch-demo" || org.AccessToken != "scratch-token" || org.OrgID != "00Dscratch" || org.Status != "Active" {
		t.Fatalf("org=%#v err=%v", org, err)
	}
}

func TestCreateScratchSurfacesSalesforceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "2SR123", "success": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": "Error", "ErrorCode": "LIMIT_EXCEEDED"})
		}
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).CreateScratch(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "hub-token", ClientID: "client-id"}, ScratchRequest{})
	if err == nil || err.Error() != "Salesforce scratch-org creation failed: LIMIT_EXCEEDED" {
		t.Fatalf("err = %v", err)
	}
}
