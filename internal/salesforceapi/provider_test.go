package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestListInstalledPackagesUsesToolingAPI(t *testing.T) {
	server := newSalesforceRESTTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/data/v67.0/tooling/query" || !strings.Contains(r.URL.Query().Get("q"), "InstalledSubscriberPackage") {
			t.Fatalf("request = %s q=%q", r.URL.Path, r.URL.Query().Get("q"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
			"SubscriberPackage":        map[string]any{"Id": "033box", "Name": "Box for Salesforce", "NamespacePrefix": "box"},
			"SubscriberPackageVersion": map[string]any{"Id": "04tbox", "Name": "5.43", "MajorVersion": 5, "MinorVersion": 43, "PatchVersion": 0, "BuildNumber": 1},
		}}})
	})
	defer server.Close()

	packages, err := (&Client{HTTP: server.Client()}).ListInstalledPackages(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil || len(packages) != 1 || packages[0].VersionNumber != "5.43.0.1" || packages[0].Namespace != "box" {
		t.Fatalf("packages=%#v err=%v", packages, err)
	}
}

func TestInstallPackageUsesToolingRequest(t *testing.T) {
	polls := 0
	updates := []PackageInstallProgress{}
	server := newSalesforceRESTTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		path := "/services/data/v67.0/tooling/sobjects/PackageInstallRequest"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == path:
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["SubscriberPackageVersionKey"] != "04tbox" || input["SecurityType"] != "None" {
				t.Fatalf("input = %#v", input)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "0Hf123", "success": true})
		case r.Method == http.MethodGet && r.URL.Path == path+"/0Hf123":
			polls++
			status := "IN_PROGRESS"
			if polls > 1 {
				status = "SUCCESS"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Status": status})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()

	err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).InstallPackageWithProgress(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "04tbox", "AdminsOnly", func(update PackageInstallProgress) {
		updates = append(updates, update)
	})
	if err != nil || polls != 2 {
		t.Fatalf("polls=%d err=%v", polls, err)
	}
	if len(updates) != 3 || updates[0].RequestID != "0Hf123" || updates[0].Status != "QUEUED" || updates[1].Status != "IN_PROGRESS" || updates[2].Status != "SUCCESS" {
		t.Fatalf("updates = %#v", updates)
	}
}

func TestToolingPackageSecurityTypeTranslatesManifestValues(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{name: "default administrators only", expected: "None"},
		{name: "CLI administrators only", input: "AdminsOnly", expected: "None"},
		{name: "CLI all users", input: "AllUsers", expected: "Full"},
		{name: "Tooling API none", input: "None", expected: "None"},
		{name: "Tooling API full", input: "Full", expected: "Full"},
		{name: "Tooling API custom", input: "Custom", expected: "Custom"},
		{name: "Tooling API push", input: "Push", expected: "Push"},
		{name: "unknown value", input: "Admins", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := toolingPackageSecurityType(test.input)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected an error, got %q", actual)
				}
				return
			}
			if err != nil || actual != test.expected {
				t.Fatalf("security type = %q, err=%v; want %q", actual, err, test.expected)
			}
		})
	}
}

func TestPermissionInventoryAndAssignmentUseRESTAPI(t *testing.T) {
	assignmentsCreated := 0
	server := newSalesforceRESTTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		versionRoot := "/services/data/v67.0"
		switch {
		case r.Method == http.MethodGet && r.URL.Path == versionRoot+"/query":
			query, _ := url.QueryUnescape(r.URL.Query().Get("q"))
			switch {
			case strings.Contains(query, "FROM User"):
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{"Id": "005admin", "Username": "admin@example.com", "Profile": map[string]string{"Name": "System Administrator"}}}})
			case strings.Contains(query, "FROM PermissionSetAssignment"):
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{"PermissionSet": map[string]string{"Name": "Box_Admin_All_Licenses", "NamespacePrefix": "box"}}}})
			case strings.Contains(query, "FROM PermissionSet"):
				_ = json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]string{"Id": "0PS123"}}})
			default:
				t.Fatalf("query = %s", query)
			}
		case r.Method == http.MethodPost && r.URL.Path == versionRoot+"/sobjects/PermissionSetAssignment":
			assignmentsCreated++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()
	client := &Client{HTTP: server.Client()}
	credential := Credential{InstanceURL: server.URL, AccessToken: "token"}

	inventory, err := client.ReadPermissionInventory(context.Background(), credential, "admin@example.com")
	if err != nil || inventory.Profile != "System Administrator" || !inventory.Assigned["box__box_admin_all_licenses"] {
		t.Fatalf("inventory=%#v err=%v", inventory, err)
	}
	if err := client.AssignPermissionSets(context.Background(), credential, inventory.UserID, []string{"CLM_Demo_Operator"}); err != nil || assignmentsCreated != 1 {
		t.Fatalf("assignments=%d err=%v", assignmentsCreated, err)
	}
}

func TestPermissionInventoryRejectsMissingDeploymentUsernameBeforeCallingSalesforce(t *testing.T) {
	client := &Client{}
	_, err := client.ReadPermissionInventory(context.Background(), Credential{InstanceURL: "https://example.com", AccessToken: "token"}, "  ")
	if err == nil || !strings.Contains(err.Error(), "authenticated deployment username") {
		t.Fatalf("error = %v", err)
	}
}

func newSalesforceRESTTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/services/data/" {
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0"}})
			return
		}
		handler(w, r)
	}))
}
