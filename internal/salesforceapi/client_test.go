package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckUsesRESTUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/oauth2/userinfo" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organization_id": "00D123", "preferred_username": "admin@example.com",
			"urls": map[string]string{"custom_domain": "https://org.develop.my.salesforce.com"},
		})
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !status.Available || status.OrgID != "00D123" || status.Username != "admin@example.com" || status.InstanceURL != "https://org.develop.my.salesforce.com" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCheckPrefersRESTAPIHostOverLightningDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organization_id": "00D123", "preferred_username": "admin@example.com",
			"urls": map[string]string{
				"custom_domain": "https://org.develop.lightning.force.com",
				"rest":          "https://org.develop.my.salesforce.com/services/data/v65.0/",
			},
		})
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || status.InstanceURL != "https://org.develop.my.salesforce.com" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCheckFallsBackToRESTWhenUserInfoIsForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/userinfo":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient_scope"}`))
		case r.URL.Path == "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case strings.HasPrefix(r.URL.Path, "/services/data/v65.0/query"):
			query := r.URL.Query().Get("q")
			switch {
			case strings.Contains(query, "FROM Organization"):
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Id": "00Dscratch"}}})
			case strings.Contains(query, "FROM User"):
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Username": "scratch@example.com"}}})
			default:
				t.Fatalf("query = %q", r.URL.RawQuery)
			}
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "scratch-token"})
	if err != nil || !status.Available || status.OrgID != "00Dscratch" || status.Username != "scratch@example.com" || status.Status != "Ready" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCheckDoesNotGuessRESTUserWhenMultipleStandardUsersExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/userinfo":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient_scope"}`))
		case r.URL.Path == "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case strings.HasPrefix(r.URL.Path, "/services/data/v65.0/query"):
			if strings.Contains(r.URL.Query().Get("q"), "FROM Organization") {
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Id": "00Dorg"}}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Username": "one@example.com"}, {"Username": "two@example.com"}}})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || status.Username != "" || status.OrgID != "00Dorg" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCheckFailsWhenUserInfoAndRESTAreUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"insufficient_scope"}`))
	}))
	defer server.Close()

	status, err := (&Client{HTTP: server.Client()}).Check(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err == nil || status.Available || !strings.Contains(err.Error(), "Salesforce org is unavailable") {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestCreateScratchUsesRESTAndExchangesAuthorizationCode(t *testing.T) {
	getCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/oauth2/userinfo":
			writeSalesforceUserInfo(w, "")
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "64.0"}, {"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if _, ok := input["Alias"]; ok {
				t.Fatalf("Alias is not a ScratchOrgInfo field: %#v", input)
			}
			if input["ConnectedAppConsumerKey"] != ScratchSignupClientID || input["ConnectedAppCallbackUrl"] != ScratchSignupCallbackURL || input["DurationDays"] != float64(30) {
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
			if r.Form.Get("code") != "auth-code" || r.Form.Get("client_id") != ScratchSignupClientID || r.Form.Get("redirect_uri") != ScratchSignupCallbackURL {
				t.Fatalf("form = %#v", r.Form)
			}
			if _, ok := r.Form["code_verifier"]; ok {
				t.Fatalf("scratch signup must not send PKCE: %#v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "scratch-token", "refresh_token": "scratch-refresh", "instance_url": server.URL})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{HTTP: server.Client(), PollInterval: time.Millisecond}
	org, err := client.CreateScratch(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "hub-token", ClientID: "client-id"}, ScratchRequest{Alias: "dispatch-demo"})
	if err != nil || org.Alias != "dispatch-demo" || org.AccessToken != "scratch-token" || org.RefreshToken != "scratch-refresh" || org.OrgID != "00Dscratch" || org.Status != "Active" {
		t.Fatalf("org=%#v err=%v", org, err)
	}
}

func TestCreateScratchPostsToUserInfoInstanceHost(t *testing.T) {
	var canonical *httptest.Server
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services/oauth2/userinfo" {
			writeSalesforceUserInfo(w, canonical.URL)
			return
		}
		if r.Method == http.MethodPost {
			t.Fatalf("POST must not hit the stale instance: %s", r.URL.Path)
		}
		t.Fatalf("unexpected stale request: %s %s", r.Method, r.URL.Path)
	}))
	defer stale.Close()

	canonical = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "2SR123", "success": true})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/2SR123":
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": "Active", "SignupUsername": "scratch@example.com", "ScratchOrg": "00Dscratch", "LoginUrl": canonical.URL, "AuthCode": "auth-code"})
		case "/services/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "scratch-token", "instance_url": canonical.URL})
		default:
			t.Fatalf("unexpected canonical request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer canonical.Close()

	org, err := (&Client{HTTP: &http.Client{}, PollInterval: time.Millisecond}).CreateScratch(context.Background(), Credential{InstanceURL: stale.URL, AccessToken: "hub-token"}, ScratchRequest{Alias: "dispatch-demo"})
	if err != nil || org.AccessToken != "scratch-token" || org.OrgID != "00Dscratch" {
		t.Fatalf("org=%#v err=%v", org, err)
	}
}

func TestCreateScratchRetriesPOSTAfterHostRedirect(t *testing.T) {
	var canonical *httptest.Server
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services/oauth2/userinfo" {
			writeSalesforceUserInfo(w, "")
			return
		}
		http.Redirect(w, r, canonical.URL+r.URL.RequestURI(), http.StatusMovedPermanently)
	}))
	defer stale.Close()

	canonical = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "NOT_FOUND", "message": "The requested resource does not exist"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "2SR123", "success": true})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/2SR123":
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": "Active", "SignupUsername": "scratch@example.com", "ScratchOrg": "00Dscratch", "LoginUrl": canonical.URL, "AuthCode": "auth-code"})
		case "/services/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "scratch-token", "instance_url": canonical.URL})
		default:
			t.Fatalf("unexpected canonical request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer canonical.Close()

	org, err := (&Client{HTTP: &http.Client{}, PollInterval: time.Millisecond}).CreateScratch(context.Background(), Credential{InstanceURL: stale.URL, AccessToken: "hub-token"}, ScratchRequest{})
	if err != nil || org.AccessToken != "scratch-token" {
		t.Fatalf("org=%#v err=%v", org, err)
	}
}

func TestHasDevHubUsesOrganizationFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query":
			if r.URL.Query().Get("q") != "SELECT IsDevHub FROM Organization" {
				t.Fatalf("query = %q", r.URL.Query().Get("q"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"IsDevHub": true}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasDevHubTrustsOrganizationFlagWhenScratchOrgInfoIsHidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query":
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"IsDevHub": true}}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/describe":
			t.Fatal("ScratchOrgInfo/describe should not run when Organization.IsDevHub is readable")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasDevHubReportsDisabledOrganizationFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query":
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"IsDevHub": false}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasDevHubUsesToolingQueryWhenRestQueryFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "INVALID_FIELD", "message": "No such column 'IsDevHub'"}})
		case "/services/data/v65.0/tooling/query":
			if r.URL.Query().Get("q") != "SELECT IsDevHub FROM Organization" {
				t.Fatalf("query = %q", r.URL.Query().Get("q"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"IsDevHub": true}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasDevHubTreatsMissingIsDevHubColumnAsNotEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query", "/services/data/v65.0/tooling/query":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "INVALID_FIELD", "message": "No such column 'IsDevHub' on entity 'Organization'"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/describe", "/services/data/v65.0/tooling/sobjects/ScratchOrgInfo/describe":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "NOT_FOUND", "message": "The requested resource does not exist"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasDevHubFallsBackToScratchOrgInfoDescribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/query", "/services/data/v65.0/tooling/query":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "INVALID_FIELD", "message": "No such column 'IsDevHub'"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo/describe":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "ScratchOrgInfo"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ok, err := (&Client{HTTP: server.Client()}).HasDevHub(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestCreateScratchExplainsMissingScratchSignup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/oauth2/userinfo":
			writeSalesforceUserInfo(w, "")
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo", "/services/data/v65.0/tooling/sobjects/ScratchOrgInfo":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "NOT_FOUND", "message": "The requested resource does not exist"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client()}).CreateScratch(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "hub-token"}, ScratchRequest{})
	if err == nil || !strings.Contains(err.Error(), "scratch-org signup") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateScratchSurfacesSalesforceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/oauth2/userinfo":
			writeSalesforceUserInfo(w, "")
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

func TestCreateScratchFallsBackToToolingWhenRESTObjectIsMissing(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/oauth2/userinfo":
			writeSalesforceUserInfo(w, "")
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "65.0"}})
		case "/services/data/v65.0/sobjects/ScratchOrgInfo":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode([]map[string]string{{"errorCode": "NOT_FOUND", "message": "The requested resource does not exist"}})
		case "/services/data/v65.0/tooling/sobjects/ScratchOrgInfo":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "2SR123", "success": true})
		case "/services/data/v65.0/tooling/sobjects/ScratchOrgInfo/2SR123":
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": "Active", "SignupUsername": "scratch@example.com", "ScratchOrg": "00Dscratch", "LoginUrl": server.URL, "AuthCode": "auth-code"})
		case "/services/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "scratch-token", "instance_url": server.URL})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	org, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).CreateScratch(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "hub-token"}, ScratchRequest{})
	if err != nil || org.AccessToken != "scratch-token" {
		t.Fatalf("org=%#v err=%v", org, err)
	}
}

func writeSalesforceUserInfo(w http.ResponseWriter, host string) {
	payload := map[string]any{"organization_id": "00Dhub", "preferred_username": "hub@example.com"}
	if strings.TrimSpace(host) != "" {
		payload["urls"] = map[string]string{
			"custom_domain": host,
			"rest":          strings.TrimRight(host, "/") + "/services/data/v65.0/",
			"tooling_rest":  strings.TrimRight(host, "/") + "/services/data/v65.0/tooling/",
		}
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func TestNewClientAllowsLongRunningMetadataInventory(t *testing.T) {
	if got := NewClient().HTTP.Timeout; got < 10*time.Minute {
		t.Fatalf("metadata inventory timeout = %s, want at least ten minutes", got)
	}
}
