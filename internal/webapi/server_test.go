package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/audit"
	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/lifecycle"
	"github.com/unofficialbox/box-dispatch/internal/salesforceapi"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
)

func TestDeploymentsExposeSafeRunSummaries(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		Profile: "demo",
		DeploymentStore: func() ([]audit.DeploymentRecord, error) {
			return []audit.DeploymentRecord{{
				DeploymentID: "run-42", Name: "Northstar CLM rollout", TemplateID: "clm", Strategy: "reuse-existing",
				SourcePath: "/private/audit.json", PackageRoot: "/private/package",
				CompletedAt: time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC),
				Providers:   []audit.ProviderRecord{{Provider: "salesforce", StatusAfter: lifecycle.StatusPresent, Detail: "raw CLI output"}},
			}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"/private", "raw CLI output", "SourcePath", "PackageRoot"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var summaries []deploymentSummary
	if err := json.Unmarshal(response.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "run-42" || summaries[0].Name != "Northstar CLM rollout" || summaries[0].Providers[0].Status != string(lifecycle.StatusPresent) {
		t.Fatalf("summaries = %#v", summaries)
	}
}

func TestSalesforceRESTConnectionCheckAndScratchCreation(t *testing.T) {
	settings := config.ConnectionSettings{}
	created := make(chan struct{}, 1)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.InstanceURL != "https://scratch.example.com" || credential.AccessToken != "scratch-token" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dscratch", Username: "scratch@example.com", Status: "Active"}, nil
		},
		SalesforceCreate: func(_ context.Context, credential salesforceapi.Credential, request salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
			if credential.InstanceURL != "https://devhub.example.com" || credential.ClientID != "client-id" || request.Alias != "replacement" || request.OrgName != "Northstar CLM rollout" {
				t.Fatalf("credential=%#v request=%#v", credential, request)
			}
			created <- struct{}{}
			return salesforceapi.ScratchOrg{Alias: "replacement", Username: "new@example.com", OrgID: "00Dnew", InstanceURL: "https://new.example.com", AccessToken: "new-token", Status: "Active", ExpirationDate: "2026-09-21"}, nil
		},
	})

	connect := httptest.NewRecorder()
	handler.ServeHTTP(connect, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce/rest", strings.NewReader(`{"instanceUrl":"https://scratch.example.com","accessToken":"scratch-token","devHubUrl":"https://devhub.example.com","devHubAccessToken":"hub-token","clientId":"client-id","clientSecret":"secret"}`)))
	if connect.Code != http.StatusOK || strings.Contains(connect.Body.String(), "scratch-token") || strings.Contains(connect.Body.String(), "secret") {
		t.Fatalf("connect = %d %s", connect.Code, connect.Body.String())
	}
	if !strings.Contains(connect.Body.String(), `"restConfigured":true`) || !strings.Contains(connect.Body.String(), `"devHubConfigured":true`) || !strings.Contains(connect.Body.String(), "Connected Salesforce org") {
		t.Fatalf("connect omitted REST connection state: %s", connect.Body.String())
	}
	if settings.SalesforceAlias != "Connected Salesforce org" {
		t.Fatalf("saved alias = %q", settings.SalesforceAlias)
	}

	check := httptest.NewRecorder()
	handler.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"available":true`) || settings.SalesforceOrgID != "00Dscratch" {
		t.Fatalf("check = %d %s settings=%#v", check.Code, check.Body.String(), settings)
	}

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/salesforce/scratch-orgs", strings.NewReader(`{"alias":"replacement","orgName":"Northstar CLM rollout","durationDays":30}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	var job scratchJob
	if err := json.Unmarshal(create.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-created:
	case <-time.After(time.Second):
		t.Fatal("scratch creation did not start")
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		status := httptest.NewRecorder()
		handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/salesforce/scratch-orgs/"+job.ID, nil))
		if strings.Contains(status.Body.String(), `"status":"active"`) {
			if settings.SalesforceAlias != "replacement" || settings.SalesforceAccessToken != "new-token" || settings.SalesforceDevHubToken != "hub-token" || len(settings.SalesforceOrgs) < 2 {
				t.Fatalf("settings = %#v", settings)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scratch creation did not complete")
}

func TestScratchCreateUsesConfirmedDevHubInsteadOfSelectedOrg(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "client-id"}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: "kyle+namespace@unofficialbox.dev", Username: "kyle+namespace@unofficialbox.dev", OrgID: "00Dns",
		InstanceURL: "https://namespace.example.com", AccessToken: "ns-token", OrgType: "persistent", DevHub: true,
	}, true)
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: "kadams385@agentforce.com", Username: "kadams385@agentforce.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.example.com", AccessToken: "hub-token", OrgType: "persistent",
	}, false)
	created := make(chan salesforceapi.Credential, 1)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceDevHubCheck: func(_ context.Context, credential salesforceapi.Credential) (bool, error) {
			return credential.AccessToken == "hub-token", nil
		},
		SalesforceCreate: func(_ context.Context, credential salesforceapi.Credential, _ salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
			created <- credential
			return salesforceapi.ScratchOrg{Alias: "test1", Username: "scratch@example.com", OrgID: "00Dscratch", InstanceURL: "https://scratch.example.com", AccessToken: "scratch-token", Status: "Active"}, nil
		},
	})
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/salesforce/scratch-orgs", strings.NewReader(`{"alias":"test1","durationDays":30}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	select {
	case credential := <-created:
		if credential.AccessToken != "hub-token" || credential.InstanceURL != "https://hub.example.com" {
			t.Fatalf("scratch create used %#v", credential)
		}
	case <-time.After(time.Second):
		t.Fatal("scratch creation did not start")
	}
}

func TestScratchCreatePreparesManagedPackageWhenRequested(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceDevHubURL: "https://devhub.example.com", SalesforceDevHubToken: "hub-token"}
	prepared := make(chan struct{}, 1)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{Name: "Northstar", PackagePath: "/tmp/package"}, nil
		},
		SalesforceCreate: func(_ context.Context, _ salesforceapi.Credential, _ salesforceapi.ScratchRequest) (salesforceapi.ScratchOrg, error) {
			return salesforceapi.ScratchOrg{Alias: "northstar", Username: "scratch@example.com", OrgID: "00Dscratch", InstanceURL: "https://scratch.example.com", AccessToken: "scratch-token", Status: "Active"}, nil
		},
		SalesforcePackagePrepare: func(_ context.Context, plan config.SolutionPlan, credential salesforceapi.Credential, report func(scratchPackageProgress)) (string, error) {
			if plan.Name != "Northstar" || credential.InstanceURL != "https://scratch.example.com" {
				t.Fatalf("plan=%#v credential=%#v", plan, credential)
			}
			report(scratchPackageProgress{Status: "installing", Message: "Salesforce reports in progress", RequestID: "0Hf-test"})
			prepared <- struct{}{}
			return "Box for Salesforce is installed", nil
		},
	})

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/salesforce/scratch-orgs", strings.NewReader(`{"alias":"northstar","installManagedPackage":true}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", create.Code, create.Body.String())
	}
	select {
	case <-prepared:
	case <-time.After(time.Second):
		t.Fatal("managed-package preparation did not start")
	}

	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		latest := httptest.NewRecorder()
		handler.ServeHTTP(latest, httptest.NewRequest(http.MethodGet, "/api/salesforce/scratch-orgs/latest", nil))
		if strings.Contains(latest.Body.String(), `"packageStatus":"complete"`) {
			if !strings.Contains(latest.Body.String(), "Box for Salesforce is installed") {
				t.Fatalf("latest = %d %s", latest.Code, latest.Body.String())
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("managed-package preparation did not complete")
}

func TestSalesforceRESTCheckRejectsExpiredScratchOrgBeforeCallingProvider(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceInstanceURL:    "https://expired.example.com",
		SalesforceAccessToken:    "expired-token",
		SalesforceOrgType:        "scratch",
		SalesforceExpirationDate: "2026-08-21",
	}
	called := false
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			called = true
			return salesforceapi.OrgStatus{}, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusGone || called || settings.SalesforceOrgStatus != "Expired" {
		t.Fatalf("status=%d called=%t settings=%#v body=%s", response.Code, called, settings, response.Body.String())
	}
}

func TestSalesforceCheckRequiresRESTCredentialsEvenWhenCLIAliasIsSelected(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceAlias: "devhub"}
	called := false
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		SalesforceCheck: func(context.Context, salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			called = true
			return salesforceapi.OrgStatus{}, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusBadRequest || called || !strings.Contains(response.Body.String(), "connect a Salesforce org before checking availability") {
		t.Fatalf("status=%d called=%t body=%s", response.Code, called, response.Body.String())
	}
}

func TestSalesforceRESTSaveWithoutAliasAllowsAvailabilityCheck(t *testing.T) {
	settings := config.ConnectionSettings{}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.InstanceURL != "https://org.example.com" || credential.AccessToken != "org-token" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dorg", Username: "user@example.com", Status: "Active"}, nil
		},
	})

	connect := httptest.NewRecorder()
	handler.ServeHTTP(connect, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce/rest", strings.NewReader(`{"instanceUrl":"https://org.example.com","accessToken":"org-token"}`)))
	if connect.Code != http.StatusOK {
		t.Fatalf("connect = %d %s", connect.Code, connect.Body.String())
	}

	check := httptest.NewRecorder()
	handler.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if check.Code != http.StatusOK || settings.SalesforceOrgID != "00Dorg" {
		t.Fatalf("check = %d %s settings=%#v", check.Code, check.Body.String(), settings)
	}

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/options", nil))
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), "Connected Salesforce org") {
		t.Fatalf("options = %d %s", options.Code, options.Body.String())
	}
}

func TestSalesforceRESTCheckMarksSelectedOrgAsDevHub(t *testing.T) {
	settings := config.ConnectionSettings{}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, _ salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dhub", Username: "hub@example.com", Status: "Ready"}, nil
		},
		SalesforceDevHubCheck: func(_ context.Context, _ salesforceapi.Credential) (bool, error) {
			return true, nil
		},
	})
	connect := httptest.NewRecorder()
	handler.ServeHTTP(connect, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce/rest", strings.NewReader(`{"instanceUrl":"https://hub.example.com","accessToken":"hub-token"}`)))
	if connect.Code != http.StatusOK || settings.HasSalesforceDevHub() {
		t.Fatalf("connect = %d hub=%t %s", connect.Code, settings.HasSalesforceDevHub(), connect.Body.String())
	}
	check := httptest.NewRecorder()
	handler.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if check.Code != http.StatusOK || !settings.HasSalesforceDevHub() {
		t.Fatalf("check = %d hub=%t %s settings=%#v", check.Code, settings.HasSalesforceDevHub(), check.Body.String(), settings)
	}
}

func TestSalesforceOAuthStartUsesPublicLoginClient(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"org"}`))
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("start = %d %s", response.Code, response.Body.String())
	}
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorize.Query()
	if query.Get("client_id") != salesforceapi.LoginClientID || query.Get("redirect_uri") != salesforceapi.LoginCallbackURL || query.Get("code_challenge") == "" {
		t.Fatalf("authorize URL = %s", started.AuthorizeURL)
	}
}

func TestSalesforceSelectsConnectedOrgFromStoredList(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "dispatch-app"}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "hub", Alias: "Dev Hub", Username: "hub@example.com", InstanceURL: "https://hub.example.com",
		AccessToken: "hub-token", OrgType: "persistent", DevHub: true,
	}, true)
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "scratch", Alias: "Scratch", Username: "scratch@example.com", InstanceURL: "https://scratch.example.com",
		AccessToken: "scratch-token", OrgType: "scratch",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce", strings.NewReader(`{"id":"hub"}`)))
	if response.Code != http.StatusOK || settings.SalesforceAccessToken != "hub-token" || settings.SalesforceDevHubToken != "hub-token" {
		t.Fatalf("select = %d %s settings=%#v", response.Code, response.Body.String(), settings)
	}
	if !strings.Contains(response.Body.String(), `"id":"hub"`) || strings.Contains(response.Body.String(), "hub-token") {
		t.Fatalf("summary = %s", response.Body.String())
	}
}

func TestSalesforceOrgCanBeRemovedByID(t *testing.T) {
	settings := config.ConnectionSettings{}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "hub", Alias: "Dev Hub", Username: "hub@example.com", InstanceURL: "https://hub.example.com",
		AccessToken: "hub-token", OrgType: "persistent", DevHub: true,
	}, true)
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "scratch", Alias: "Scratch", Username: "scratch@example.com", InstanceURL: "https://scratch.example.com",
		AccessToken: "scratch-token", OrgType: "scratch",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/connections/salesforce/scratch", nil))
	if response.Code != http.StatusOK || len(settings.SalesforceOrgs) != 1 || settings.SalesforceAccessToken != "hub-token" || strings.Contains(response.Body.String(), "scratch-token") {
		t.Fatalf("remove = %d settings=%#v body=%s", response.Code, settings, response.Body.String())
	}
}

func TestSalesforceOAuthWebLoginSavesTokensWithoutReturningSecrets(t *testing.T) {
	settings := config.ConnectionSettings{}
	var exchanged salesforceapi.AuthorizationCodeRequest
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 21, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceOAuth: func(_ context.Context, request salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
			exchanged = request
			return salesforceapi.TokenResponse{AccessToken: "oauth-access", RefreshToken: "oauth-refresh", InstanceURL: "https://org.my.salesforce.com"}, nil
		},
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.AccessToken != "oauth-access" || credential.InstanceURL != "https://org.my.salesforce.com" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Doauth", Username: "admin@example.com", Status: "Active"}, nil
		},
	})

	start := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"org"}`))
	handler.ServeHTTP(start, startReq)
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(started.AuthorizeURL, "login.salesforce.com") || !strings.Contains(started.AuthorizeURL, "code_challenge") || !strings.Contains(started.AuthorizeURL, "PlatformCLI") || strings.Contains(start.Body.String(), "oauth-access") {
		t.Fatalf("authorize URL = %s", start.Body.String())
	}
	if settings.SalesforceClientID != "" {
		t.Fatalf("login should not persist a consumer key: %#v", settings)
	}

	callback := httptest.NewRecorder()
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "connected") || strings.Contains(callback.Body.String(), "oauth-access") || strings.Contains(callback.Body.String(), "oauth-refresh") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	if exchanged.Code != "auth-code" || exchanged.ClientID != salesforceapi.LoginClientID || exchanged.RedirectURL != salesforceapi.LoginCallbackURL || exchanged.CodeVerifier == "" {
		t.Fatalf("exchange = %#v", exchanged)
	}
	if settings.SalesforceAccessToken != "oauth-access" || settings.SalesforceRefreshToken != "oauth-refresh" || settings.SalesforceOrgID != "00Doauth" || settings.VerifiedConnections["salesforce"].Identity != "admin@example.com" {
		t.Fatalf("settings = %#v", settings)
	}
	if len(settings.SalesforceOrgs) != 1 || settings.HasSalesforceDevHub() {
		t.Fatalf("regular org login should not become the Dev Hub: %#v", settings)
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/oauth/"+started.ID, nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"active"`) || strings.Contains(status.Body.String(), "oauth-access") {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}

	summary := httptest.NewRecorder()
	handler.ServeHTTP(summary, httptest.NewRequest(http.MethodGet, "/api/connections", nil))
	if !strings.Contains(summary.Body.String(), `"oauthConfigured":true`) || !strings.Contains(summary.Body.String(), "Salesforce OAuth") || strings.Contains(summary.Body.String(), `"devHubConfigured":true`) || !strings.Contains(summary.Body.String(), "admin@example.com") || strings.Contains(summary.Body.String(), "oauth-refresh") {
		t.Fatalf("summary = %s", summary.Body.String())
	}
}

func TestSalesforceOAuthDevHubRoleStoresOnlyAConfirmedDevHub(t *testing.T) {
	settings := config.ConnectionSettings{}
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceOAuth: func(_ context.Context, _ salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
			return salesforceapi.TokenResponse{AccessToken: "hub-access", RefreshToken: "hub-refresh", InstanceURL: "https://hub.my.salesforce.com"}, nil
		},
		SalesforceCheck: func(_ context.Context, _ salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dhub", Username: "hub@example.com", Status: "Ready"}, nil
		},
		SalesforceDevHubCheck: func(_ context.Context, credential salesforceapi.Credential) (bool, error) {
			return credential.AccessToken == "hub-access", nil
		},
	})
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"devhub"}`)))
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "Dev Hub connected") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	if !settings.HasSalesforceDevHub() || settings.SalesforceDevHubToken != "hub-access" {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestSalesforceOAuthDevHubRoleRejectsOrgsWithoutDevHub(t *testing.T) {
	settings := config.ConnectionSettings{}
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceOAuth: func(_ context.Context, _ salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
			return salesforceapi.TokenResponse{AccessToken: "org-access", InstanceURL: "https://org.my.salesforce.com"}, nil
		},
		SalesforceCheck: func(_ context.Context, _ salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dorg", Username: "admin@example.com", Status: "Ready"}, nil
		},
		SalesforceDevHubCheck: func(_ context.Context, _ salesforceapi.Credential) (bool, error) {
			return false, nil
		},
	})
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"devhub"}`)))
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "not a Dev Hub") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	if settings.HasSalesforceDevHub() || settings.SalesforceAccessToken != "org-access" {
		t.Fatalf("rejected Dev Hub login should still save the org: %#v", settings)
	}
}

func TestSalesforceOAuthDevHubRoleKeepsHubWhenStatusCheckFails(t *testing.T) {
	settings := config.ConnectionSettings{}
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceOAuth: func(_ context.Context, _ salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
			return salesforceapi.TokenResponse{AccessToken: "hub-access", RefreshToken: "hub-refresh", InstanceURL: "https://hub.my.salesforce.com"}, nil
		},
		SalesforceCheck: func(_ context.Context, _ salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dhub", Username: "hub@example.com", Status: "Ready"}, nil
		},
		SalesforceDevHubCheck: func(_ context.Context, _ salesforceapi.Credential) (bool, error) {
			return false, errors.New("could not confirm Salesforce Dev Hub status")
		},
	})
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"devhub"}`)))
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "Dev Hub connected") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	if !settings.HasSalesforceDevHub() || settings.SalesforceDevHubToken != "hub-access" {
		t.Fatalf("explicit Dev Hub login should be kept when the live check errors: %#v", settings)
	}
}

func TestSalesforceOAuthSecondOrgKeepsTheFirst(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "dispatch-app"}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: "devhub", Username: "hub@example.com", OrgID: "00Dhub",
		InstanceURL: "https://hub.my.salesforce.com", AccessToken: "hub-token", RefreshToken: "hub-refresh",
		OrgType: "persistent", DevHub: true, ClientID: "dispatch-app",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceOAuth: func(_ context.Context, _ salesforceapi.AuthorizationCodeRequest) (salesforceapi.TokenResponse, error) {
			return salesforceapi.TokenResponse{AccessToken: "prod-access", RefreshToken: "prod-refresh", InstanceURL: "https://prod.my.salesforce.com"}, nil
		},
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.AccessToken != "prod-access" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dprod", Username: "admin@prod.example", Status: "Ready"}, nil
		},
	})
	start := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"org"}`))
	handler.ServeHTTP(start, startReq)
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || len(settings.SalesforceOrgs) != 2 {
		t.Fatalf("callback = %d orgs=%#v body=%s", callback.Code, settings.SalesforceOrgs, callback.Body.String())
	}
	if settings.SalesforceOrgID != "00Dprod" || settings.SalesforceDevHubToken != "hub-token" || settings.SalesforceOrgs[0].OrgID != "00Dhub" {
		t.Fatalf("second org replaced the first: %#v", settings)
	}
}

func TestSalesforceOAuthCallbackExplainsCrossOrgExternalClientApp(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "dispatch-app"}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})
	start := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/oauth/start", strings.NewReader(`{"loginHost":"production","role":"org"}`))
	startReq.Host = "localhost:8792"
	handler.ServeHTTP(start, startReq)
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	var started salesforceOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}

	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/oauth/callback?error=OAUTH_APPROVAL_ERROR&error_description="+url.QueryEscape("Cross-org OAuth flows are not supported for this external client app")+"&state="+authorize.Query().Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "Start login again from Dispatch") || strings.Contains(callback.Body.String(), "Cross-org OAuth flows") || strings.Contains(callback.Body.String(), "consumer key") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/oauth/"+started.ID, nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"failed"`) || !strings.Contains(status.Body.String(), "Start login again from Dispatch") {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}
}

func TestSalesforceOAuthCallbackRejectsUnknownState(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/oauth/callback?code=auth-code&state=missing", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "was not found") {
		t.Fatalf("callback = %d %s", response.Code, response.Body.String())
	}
}

func TestSalesforceAvailabilityCheckRefreshesExpiredAccessToken(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceInstanceURL:  "https://org.example.com",
		SalesforceAccessToken:  "stale-token",
		SalesforceRefreshToken: "refresh-token",
		SalesforceClientID:     "dispatch-app",
	}
	checks := 0
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 21, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			checks++
			if credential.AccessToken == "stale-token" {
				return salesforceapi.OrgStatus{}, errors.New("expired access token")
			}
			if credential.AccessToken != "fresh-token" {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, OrgID: "00Dfresh", Username: "admin@example.com", Status: "Active"}, nil
		},
		SalesforceRefresh: func(_ context.Context, request salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error) {
			if request.RefreshToken != "refresh-token" || request.ClientID != "dispatch-app" {
				t.Fatalf("refresh = %#v", request)
			}
			return salesforceapi.TokenResponse{AccessToken: "fresh-token", InstanceURL: "https://org.example.com"}, nil
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusOK || settings.SalesforceAccessToken != "fresh-token" || settings.VerifiedConnections["salesforce"].AuthType != "Salesforce OAuth" || checks != 2 {
		t.Fatalf("check = %d %s settings=%#v checks=%d", response.Code, response.Body.String(), settings, checks)
	}
}

func TestSalesforceAvailabilityCheckRefreshesScratchOrgWithoutStoredClientID(t *testing.T) {
	settings := config.ConnectionSettings{}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: "Test1", Username: "test-1haddldbfzqy@example.com", OrgID: "00Dscratch",
		InstanceURL: "https://scratch.example.com", AccessToken: "stale-token", RefreshToken: "refresh-token",
		OrgType: "scratch", Status: "Active",
	}, true)
	checks := 0
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			checks++
			if credential.AccessToken == "stale-token" {
				return salesforceapi.OrgStatus{}, errors.New("HTTP 403 from Salesforce")
			}
			if credential.AccessToken != "fresh-token" || credential.ClientID != salesforceapi.LoginClientID {
				t.Fatalf("credential = %#v", credential)
			}
			return salesforceapi.OrgStatus{Available: true, Status: "Ready"}, nil
		},
		SalesforceRefresh: func(_ context.Context, request salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error) {
			if request.RefreshToken != "refresh-token" || request.ClientID != salesforceapi.LoginClientID || request.ClientSecret != "" {
				t.Fatalf("refresh = %#v", request)
			}
			return salesforceapi.TokenResponse{AccessToken: "fresh-token", InstanceURL: "https://scratch.example.com"}, nil
		},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusOK || settings.SalesforceAccessToken != "fresh-token" || settings.VerifiedConnections["salesforce"].Identity != "test-1haddldbfzqy@example.com" || settings.VerifiedConnections["salesforce"].OrgID != "00Dscratch" || checks != 2 || !strings.Contains(response.Body.String(), `"username":"test-1haddldbfzqy@example.com"`) {
		t.Fatalf("check = %d %s settings=%#v checks=%d", response.Code, response.Body.String(), settings, checks)
	}
}

func TestSalesforceAvailabilityCheckClearsStaleVerificationAfterRefreshFailure(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceClientID: "custom-client", SalesforceClientSecret: "custom-secret",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"salesforce": {VerifiedAt: "2026-08-25T01:20:09Z", Identity: "admin@example.com"},
		},
	}.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		Alias: "Production", Username: "admin@example.com", OrgID: "00Dprod",
		InstanceURL: "https://prod.example.com", AccessToken: "stale-token", RefreshToken: "refresh-token",
		ClientID: salesforceapi.LoginClientID, Status: "Active",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceCheck: func(_ context.Context, credential salesforceapi.Credential) (salesforceapi.OrgStatus, error) {
			if credential.ClientID != salesforceapi.LoginClientID || credential.ClientSecret != "" {
				t.Fatalf("credential mixed OAuth client records: %#v", credential)
			}
			return salesforceapi.OrgStatus{}, errors.New("INVALID_SESSION_ID: Session expired or invalid")
		},
		SalesforceRefresh: func(_ context.Context, request salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error) {
			if request.ClientID != salesforceapi.LoginClientID || request.ClientSecret != "" {
				t.Fatalf("refresh mixed OAuth client records: %#v", request)
			}
			return salesforceapi.TokenResponse{}, errors.New("invalid_grant: refresh token expired")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/salesforce/check", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("check = %d %s", response.Code, response.Body.String())
	}
	if _, verified := settings.VerifiedConnections["salesforce"]; verified {
		t.Fatalf("stale readiness was retained: %#v", settings.VerifiedConnections)
	}
	selected, ok := settings.SelectedSalesforceOrg()
	if !ok || selected.Status != "Unavailable" {
		t.Fatalf("selected org = %#v", selected)
	}
}

func TestDeploymentDetailCountsWithoutDiagnostics(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		DeploymentStore: func() ([]audit.DeploymentRecord, error) {
			return []audit.DeploymentRecord{{
				DeploymentID: "run-42", SourcePath: "/private/audit.json", PackageRoot: "/private/package",
				Providers: []audit.ProviderRecord{{
					Provider: "box", StatusAfter: lifecycle.StatusMissing, Detail: "secret-adjacent diagnostic",
					Deployed: []string{"one"}, PresentAfter: []string{"one", "two"}, Remaining: []string{"three"},
					AdapterPending: []string{"four"}, Experimental: []string{"five"},
				}},
			}}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/deployments/run-42", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"/private", "secret-adjacent", "AdapterPending"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var detail deploymentDetail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	provider := detail.Providers[0]
	if provider.DeployedCount != 1 || provider.PresentCount != 2 || provider.RemainingCount != 1 || provider.ManualItemCount != 2 {
		t.Fatalf("provider detail = %#v", provider)
	}
}

func TestConnectionsRedactCredentials(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) {
			return config.ConnectionSettings{
				SalesforceAlias: "scratch-org", SalesforceOrgStatus: "Active", SalesforceExpirationDate: "2026-09-20",
				BoxCCGAlias: "Legal Box", BoxCCGClientID: "client-id", BoxCCGClientSecret: "private-secret", BoxCCGSubjectType: "enterprise", BoxCCGSubjectID: "123",
				VerifiedConnections: map[string]config.VerifiedConnection{"box": {VerifiedAt: "2026-08-21", Selection: "ccg", Identity: "operator@example.com"}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"private-secret", "client-id", "123"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "client credentials") || !strings.Contains(body, "scratch-org") || !strings.Contains(body, "Legal Box") || !strings.Contains(body, "Ending in t-id") || !strings.Contains(body, "operator@example.com") {
		t.Fatalf("response omitted safe connection state: %s", body)
	}
}

func TestBoxOAuthStartRequiresCallbackPort(t *testing.T) {
	t.Setenv("BOX_CLIENT_ID", "box-client")
	t.Setenv("BOX_CLIENT_SECRET", "box-secret")
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore:  func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
		BoxCallbackReady: func() bool { return false },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/box/oauth/start", strings.NewReader(`{}`)))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "4400") {
		t.Fatalf("start = %d %s", response.Code, response.Body.String())
	}
}

func TestBoxOAuthCallbackPageUsesBoxCopy(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/oauth/callback?state=missing", nil))
	body := response.Body.String()
	if !strings.Contains(body, "Box login did not finish") || !strings.Contains(body, "Box login session was not found") || !strings.Contains(body, "Box Dispatch") || !strings.Contains(body, "B/") || !strings.Contains(body, "border-radius: 10px") || strings.Contains(body, "Salesforce login") {
		t.Fatalf("callback page = %s", body)
	}
}

func TestBoxOAuthStartRequiresEnvironmentClient(t *testing.T) {
	t.Setenv("BOX_CLIENT_ID", "")
	t.Setenv("BOX_CLIENT_SECRET", "")
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/box/oauth/start", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "BOX_CLIENT_ID") {
		t.Fatalf("start = %d %s", response.Code, response.Body.String())
	}
}

func TestBoxOAuthWebLoginSavesTokensWithoutReturningSecrets(t *testing.T) {
	t.Setenv("BOX_CLIENT_ID", "box-client")
	t.Setenv("BOX_CLIENT_SECRET", "box-secret")
	settings := config.ConnectionSettings{}
	var exchanged boxconn.AuthorizationCodeRequest
	handler := NewHandlerWithOptions(ServerOptions{
		Now:             func() time.Time { return time.Date(2026, 8, 23, 23, 0, 0, 0, time.UTC) },
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		BoxOAuth: func(_ context.Context, request boxconn.AuthorizationCodeRequest) (boxconn.TokenResponse, error) {
			exchanged = request
			return boxconn.TokenResponse{AccessToken: "box-access", RefreshToken: "box-refresh"}, nil
		},
		BoxCheck: func(_ context.Context, candidate config.ConnectionSettings) (config.VerifiedConnection, error) {
			if !candidate.HasBoxOAuth() {
				t.Fatalf("candidate = %#v", candidate)
			}
			return config.VerifiedConnection{Identity: "box-user@example.com", Account: "12345", Enterprise: "98765", AuthType: "OAuth2"}, nil
		},
	})

	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/connections/box/oauth/start", strings.NewReader(`{}`)))
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	var started boxOAuthStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(started.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authorize.Query()
	if query.Get("client_id") != "box-client" || query.Get("redirect_uri") != boxconn.LoginCallbackURL || query.Get("code_challenge") == "" {
		t.Fatalf("authorize URL = %s", started.AuthorizeURL)
	}

	callback := httptest.NewRecorder()
	handler.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/oauth/callback?code=auth-code&state="+query.Get("state"), nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "Box connected") || !strings.Contains(callback.Body.String(), "Box Dispatch") || !strings.Contains(callback.Body.String(), "B/") || strings.Contains(callback.Body.String(), "Salesforce login") || strings.Contains(callback.Body.String(), "box-access") || strings.Contains(callback.Body.String(), "box-refresh") {
		t.Fatalf("callback = %d %s", callback.Code, callback.Body.String())
	}
	if exchanged.Code != "auth-code" || exchanged.ClientID != "box-client" || exchanged.RedirectURL != boxconn.LoginCallbackURL || exchanged.CodeVerifier == "" {
		t.Fatalf("exchange = %#v", exchanged)
	}
	if len(settings.BoxConnections) != 1 || settings.BoxConnections[0].RefreshToken != "box-refresh" || settings.VerifiedConnections["box"].Identity != "box-user@example.com" {
		t.Fatalf("settings = %#v", settings)
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/connections/box/oauth/"+started.ID, nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"active"`) || strings.Contains(status.Body.String(), "box-refresh") {
		t.Fatalf("status = %d %s", status.Code, status.Body.String())
	}

	summary := httptest.NewRecorder()
	handler.ServeHTTP(summary, httptest.NewRequest(http.MethodGet, "/api/connections", nil))
	if !strings.Contains(summary.Body.String(), `"oauthConfigured":true`) || !strings.Contains(summary.Body.String(), "Box OAuth") || !strings.Contains(summary.Body.String(), "box-user@example.com") || strings.Contains(summary.Body.String(), "box-refresh") || strings.Contains(summary.Body.String(), "box-secret") {
		t.Fatalf("summary = %s", summary.Body.String())
	}
}

func TestBoxConnectionStoresCCGWithoutReturningSecrets(t *testing.T) {
	var settings config.ConnectionSettings
	checked := false
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		BoxCheck: func(_ context.Context, candidate config.ConnectionSettings) (config.VerifiedConnection, error) {
			checked = true
			if candidate.BoxCCGClientID != "client-id" || candidate.BoxCCGSubjectID != "12345" {
				t.Fatalf("candidate = %#v", candidate)
			}
			return config.VerifiedConnection{Selection: boxconn.DispatchCCGName, Identity: "box-user@example.com", Account: "12345", Enterprise: "98765", AuthType: "CCG"}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 22, 20, 0, 0, 0, time.UTC) },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/box", strings.NewReader(`{"alias":"Legal Box","clientId":"client-id","clientSecret":"very-secret","subjectType":"enterprise","subjectId":"12345"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if !checked || settings.BoxCCGAlias != "Legal Box" || settings.BoxCCGClientID != "client-id" || settings.BoxCCGClientSecret != "very-secret" || settings.BoxCCGSubjectType != "enterprise" || settings.BoxCCGSubjectID != "12345" {
		t.Fatalf("settings = %#v", settings)
	}
	if settings.VerifiedConnections["box"].VerifiedAt == "" || settings.VerifiedConnections["box"].Selection != "Legal Box" {
		t.Fatalf("verification was not saved: %#v", settings.VerifiedConnections)
	}
	if len(settings.BoxConnections) != 1 || settings.BoxConnections[0].ClientSecret != "very-secret" {
		t.Fatalf("box connections = %#v", settings.BoxConnections)
	}
	if strings.Contains(response.Body.String(), "very-secret") || strings.Contains(response.Body.String(), "client-id") || strings.Contains(response.Body.String(), "12345") {
		t.Fatalf("secret material leaked: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Legal Box") || !strings.Contains(response.Body.String(), "Ending in t-id") || !strings.Contains(response.Body.String(), "Ending in 2345") || !strings.Contains(response.Body.String(), `"verified":true`) {
		t.Fatalf("safe connection details missing: %s", response.Body.String())
	}
}

func TestBoxConnectionSecondAppKeepsTheFirst(t *testing.T) {
	settings := config.ConnectionSettings{}.UpsertBoxConnection(config.BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a", SubjectType: "enterprise", SubjectID: "111",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		BoxCheck: func(_ context.Context, candidate config.ConnectionSettings) (config.VerifiedConnection, error) {
			if candidate.BoxCCGClientID != "client-b" {
				t.Fatalf("candidate = %#v", candidate)
			}
			return config.VerifiedConnection{Selection: "Finance Box", AuthType: "CCG"}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC) },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/box", strings.NewReader(`{"alias":"Finance Box","clientId":"client-b","clientSecret":"secret-b","subjectType":"user","subjectId":"222"}`)))
	if response.Code != http.StatusOK || len(settings.BoxConnections) != 2 {
		t.Fatalf("status = %d apps=%#v body=%s", response.Code, settings.BoxConnections, response.Body.String())
	}
	if settings.BoxCCGAlias != "Finance Box" || settings.BoxConnections[0].Alias != "Legal Box" || settings.BoxConnections[0].ClientSecret != "secret-a" {
		t.Fatalf("second Box app replaced the first: %#v", settings)
	}
}

func TestBoxConnectionCanBeRemovedByID(t *testing.T) {
	settings := config.ConnectionSettings{}.UpsertBoxConnection(config.BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a", SubjectType: "enterprise", SubjectID: "111",
	}, true)
	settings = settings.UpsertBoxConnection(config.BoxAppConnection{
		Alias: "Finance Box", RefreshToken: "refresh-b", Identity: "finance@example.com", Account: "222",
	}, true)
	removeID := settings.BoxSelectedConnectionID
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/connections/box/"+removeID, nil))
	if response.Code != http.StatusOK || len(settings.BoxConnections) != 1 || settings.BoxCCGAlias != "Legal Box" || strings.Contains(response.Body.String(), "refresh-b") {
		t.Fatalf("remove = %d settings=%#v body=%s", response.Code, settings, response.Body.String())
	}
}

func TestSavedBoxConnectionCanBeVerified(t *testing.T) {
	settings := config.ConnectionSettings{BoxCCGAlias: "Legal Box", BoxCCGClientID: "client-id", BoxCCGClientSecret: "secret", BoxCCGSubjectType: "user", BoxCCGSubjectID: "12345"}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		BoxCheck: func(_ context.Context, _ config.ConnectionSettings) (config.VerifiedConnection, error) {
			return config.VerifiedConnection{Selection: boxconn.DispatchCCGName, Identity: "box-user@example.com", Account: "12345", Enterprise: "98765", AuthType: "CCG"}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 22, 20, 5, 0, 0, time.UTC) },
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/connections/box/check", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"verified":true`) {
		t.Fatalf("response = %d: %s", response.Code, response.Body.String())
	}
	if settings.VerifiedConnections["box"].VerifiedAt == "" {
		t.Fatalf("verification was not persisted: %#v", settings.VerifiedConnections)
	}
}

func TestBoxConnectionIsNotSavedWhenVerificationFails(t *testing.T) {
	settings := config.ConnectionSettings{}
	saved := false
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; saved = true; return nil },
		BoxCheck: func(_ context.Context, _ config.ConnectionSettings) (config.VerifiedConnection, error) {
			return config.VerifiedConnection{}, errors.New("Box rejected these credentials")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/box", strings.NewReader(`{"alias":"Legal Box","clientId":"client-id","clientSecret":"wrong-secret","subjectType":"user","subjectId":"12345"}`)))
	if response.Code != http.StatusUnprocessableEntity || saved {
		t.Fatalf("response = %d, saved = %t: %s", response.Code, saved, response.Body.String())
	}
}

func TestBoxSelectsConnectedAppFromStoredList(t *testing.T) {
	settings := config.ConnectionSettings{}.UpsertBoxConnection(config.BoxAppConnection{
		Alias: "Legal Box", ClientID: "client-a", ClientSecret: "secret-a", SubjectType: "enterprise", SubjectID: "111",
	}, true).UpsertBoxConnection(config.BoxAppConnection{
		Alias: "Finance Box", ClientID: "client-b", ClientSecret: "secret-b", SubjectType: "user", SubjectID: "222",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/connections/box/selection", strings.NewReader(`{"id":"`+settings.BoxConnections[0].ID+`"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if settings.BoxCCGAlias != "Legal Box" || settings.BoxCCGClientID != "client-a" {
		t.Fatalf("selected = %#v", settings)
	}
	if strings.Contains(response.Body.String(), "secret-a") || !strings.Contains(response.Body.String(), "Finance Box") {
		t.Fatalf("response = %s", response.Body.String())
	}
}

func TestSalesforceConnectionSelectionOnlyAcceptsAuthenticatedAlias(t *testing.T) {
	settings := config.ConnectionSettings{
		SalesforceAlias: "old-org", SalesforceOrgID: "00Dold",
		VerifiedConnections: map[string]config.VerifiedConnection{"salesforce": {VerifiedAt: "2026-08-21", Selection: "old-org"}},
	}
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceTargets: func() ([]salesforceorg.Target, error) {
			return []salesforceorg.Target{
				{Alias: "dispatch-scratch", ConnectedStatus: "Connected", Status: "Active", ExpirationDate: "2026-09-15", DevHubID: "00Dhub"},
				{Alias: "disconnected", ConnectedStatus: "Disconnected"},
			}, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/options", nil))
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), "dispatch-scratch") || strings.Contains(options.Body.String(), "disconnected") {
		t.Fatalf("options = %d: %s", options.Code, options.Body.String())
	}

	selectRequest := httptest.NewRequest(http.MethodPut, "/api/connections/salesforce", strings.NewReader(`{"alias":"dispatch-scratch"}`))
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, selectRequest)
	if selected.Code != http.StatusOK {
		t.Fatalf("selection = %d: %s", selected.Code, selected.Body.String())
	}
	if settings.SalesforceAlias != "dispatch-scratch" || settings.SalesforceOrgID != "" || settings.SalesforceOrgType != "scratch" {
		t.Fatalf("saved settings = %#v", settings)
	}
	if _, found := settings.VerifiedConnections["salesforce"]; found {
		t.Fatalf("selection retained stale verification: %#v", settings.VerifiedConnections)
	}

	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodPut, "/api/connections/salesforce", strings.NewReader(`{"alias":"unknown"}`)))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("rejected selection = %d: %s", rejected.Code, rejected.Body.String())
	}
}

func TestHealthReturnsProfile(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{Profile: "demo", Now: func() time.Time { return time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC) }})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"profile":"demo"`) {
		t.Fatalf("health = %d %s", response.Code, response.Body.String())
	}
}

func TestPlanExposesProviderReadinessWithoutPackagePath(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{Name: "Northstar CLM rollout", TemplateID: "clm", Template: "Contract Lifecycle Management", Repository: "https://example.test/clm", PackagePath: "/private/package", Components: []string{"box", "salesforce"}}, nil
		},
		ConnectionStore: func() (config.ConnectionSettings, error) {
			return config.ConnectionSettings{
				SalesforceAlias: "scratch-org",
				BoxCCGClientID:  "id", BoxCCGClientSecret: "secret", BoxCCGSubjectType: "enterprise", BoxCCGSubjectID: "123",
				VerifiedConnections: map[string]config.VerifiedConnection{"box": {VerifiedAt: "2026-08-21"}, "salesforce": {VerifiedAt: "2026-08-21"}},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/plan", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if strings.Contains(response.Body.String(), "/private/package") {
		t.Fatalf("plan response exposed package path: %s", response.Body.String())
	}
	var plan planResponse
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Exists || plan.Name != "Northstar CLM rollout" || len(plan.Components) != 2 || !plan.Components[0].Ready || !plan.Components[1].Ready {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestConnectionSummariesExposeSafeProviderDestinations(t *testing.T) {
	settings := config.ConnectionSettings{
		BoxCCGClientID: "id", BoxCCGClientSecret: "secret", BoxCCGSubjectType: "enterprise", BoxCCGSubjectID: "123",
		SalesforceAlias: "northstar", SalesforceInstanceURL: "https://northstar.my.salesforce.com/services/data/?token=hidden", SalesforceAccessToken: "token",
		VerifiedConnections: map[string]config.VerifiedConnection{
			"box":        {VerifiedAt: "2026-08-25"},
			"salesforce": {VerifiedAt: "2026-08-25"},
		},
	}
	summaries := connectionSummaries(settings)
	if summaries[0].LaunchURL != "https://app.box.com/" {
		t.Fatalf("Box launch URL = %q", summaries[0].LaunchURL)
	}
	if summaries[1].LaunchURL != "/api/connections/salesforce/open" {
		t.Fatalf("Salesforce launch URL = %q", summaries[1].LaunchURL)
	}
	if got := safeHTTPSLaunchURL("javascript:alert(1)"); got != "" {
		t.Fatalf("unsafe launch URL = %q", got)
	}
}

func TestSalesforceOpenRefreshesSelectedOrgAndRedirectsThroughFrontDoor(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "platform-client"}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "northstar", Alias: "Northstar", InstanceURL: "https://old.my.salesforce.com",
		AccessToken: "stale-token", RefreshToken: "refresh-token", OrgType: "persistent",
	}, true)
	refreshed := false
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceRefresh: func(_ context.Context, request salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error) {
			refreshed = true
			if request.ClientID != "platform-client" || request.RefreshToken != "refresh-token" {
				t.Fatalf("refresh request = %#v", request)
			}
			return salesforceapi.TokenResponse{InstanceURL: "https://northstar.my.salesforce.com", AccessToken: "fresh-token"}, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/open", nil))
	if response.Code != http.StatusSeeOther || !refreshed {
		t.Fatalf("open = %d refreshed=%t body=%s", response.Code, refreshed, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Scheme != "https" || location.Host != "northstar.my.salesforce.com" || location.Path != "/secur/frontdoor.jsp" || location.Query().Get("sid") != "fresh-token" || location.Query().Get("retURL") != "/lightning/page/home" {
		t.Fatalf("front-door location = %q", location.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" || response.Body.Len() != 0 {
		t.Fatalf("unsafe redirect response headers=%v body=%q", response.Header(), response.Body.String())
	}
	if settings.SalesforceAccessToken != "fresh-token" || settings.SalesforceInstanceURL != "https://northstar.my.salesforce.com" {
		t.Fatalf("saved settings = %#v", settings)
	}
}

func TestSalesforceOpenUsesKnownDestinationPaths(t *testing.T) {
	settings := config.ConnectionSettings{}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "northstar", Alias: "Northstar", InstanceURL: "https://northstar.my.salesforce.com",
		AccessToken: "access-token", OrgType: "persistent",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
	})

	tests := []struct {
		name        string
		destination string
		wantPath    string
	}{
		{name: "Box settings", destination: "box-settings", wantPath: "/lightning/n/box__Box_Settings"},
		{name: "CLM app", destination: "clm-app", wantPath: "/lightning/app/c__CLM_Demo"},
		{name: "unknown destination", destination: "unsupported", wantPath: "/lightning/page/home"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/open?destination="+url.QueryEscape(test.destination), nil)
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("open = %d, want %d; body=%s", response.Code, http.StatusSeeOther, response.Body.String())
			}
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if location.Query().Get("sid") != "access-token" || location.Query().Get("retURL") != test.wantPath {
				t.Fatalf("front-door location = %q", location.String())
			}
		})
	}
}

func TestSalesforceOpenUsesAuthenticatedExperienceEmployeePath(t *testing.T) {
	settings := config.ConnectionSettings{}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "northstar", Alias: "Northstar", InstanceURL: "https://northstar.my.salesforce.com",
		AccessToken: "access-token", OrgType: "persistent",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		SalesforceExperiencePath: func(_ context.Context, _ salesforceapi.Credential, version, networkID string) (string, error) {
			if version != "" || networkID != "0DB1" {
				t.Fatalf("version=%q networkID=%q", version, networkID)
			}
			return "/servlet/networks/session/create?site=0DM1", nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/open?destination=experience-site&site=0DB1", nil))
	if response.Code != http.StatusSeeOther {
		t.Fatalf("open = %d, want %d; body=%s", response.Code, http.StatusSeeOther, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("sid") != "access-token" || location.Query().Get("retURL") != "/servlet/networks/session/create?site=0DM1" {
		t.Fatalf("front-door location = %q", location.String())
	}
}

func TestSalesforceOpenRejectsExpiredRefreshSession(t *testing.T) {
	settings := config.ConnectionSettings{SalesforceClientID: "platform-client"}
	settings = settings.UpsertSalesforceOrg(config.SalesforceOrgConnection{
		ID: "northstar", Alias: "Northstar", InstanceURL: "https://northstar.my.salesforce.com",
		AccessToken: "stale-token", RefreshToken: "expired-refresh-token", OrgType: "persistent",
	}, true)
	handler := NewHandlerWithOptions(ServerOptions{
		ConnectionStore: func() (config.ConnectionSettings, error) { return settings, nil },
		ConnectionSaver: func(next config.ConnectionSettings) error { settings = next; return nil },
		SalesforceRefresh: func(context.Context, salesforceapi.RefreshRequest) (salesforceapi.TokenResponse, error) {
			return salesforceapi.TokenResponse{}, errors.New("invalid_grant")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/connections/salesforce/open", nil))
	if response.Code != http.StatusUnauthorized || response.Header().Get("Location") != "" || !strings.Contains(response.Body.String(), "Reconnect the org") {
		t.Fatalf("open = %d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if settings.VerifiedConnections != nil && settings.VerifiedConnections["salesforce"].VerifiedAt != "" {
		t.Fatalf("stale verification was retained: %#v", settings.VerifiedConnections)
	}
}

func TestPlanUpdatePersistsOnlySupportedDraftFields(t *testing.T) {
	var saved config.SolutionPlan
	handler := NewHandlerWithOptions(ServerOptions{
		PlanStore: func() (config.SolutionPlan, error) {
			return config.SolutionPlan{PackagePath: "/kept-on-server"}, nil
		},
		PlanSaver:       func(plan config.SolutionPlan) error { saved = plan; return nil },
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})

	req := httptest.NewRequest(http.MethodPut, "/api/plan", strings.NewReader(`{"name":" Northstar CLM rollout ","templateId":" clm ","template":" Contract Lifecycle Management ","repository":" https://example.test/clm ","components":[" BOX ","salesforce"]}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if saved.PackagePath != "/kept-on-server" || saved.Name != "Northstar CLM rollout" || saved.TemplateID != "clm" || saved.Template != "Contract Lifecycle Management" || saved.Repository != "https://example.test/clm" || strings.Join(saved.Components, ",") != "box,salesforce" {
		t.Fatalf("saved = %#v", saved)
	}
}

func TestPlanUpdateRejectsUnsupportedProvider(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{PlanStore: func() (config.SolutionPlan, error) { return config.SolutionPlan{}, nil }})
	req := httptest.NewRequest(http.MethodPut, "/api/plan", strings.NewReader(`{"name":"Test deployment","templateId":"clm","template":"CLM","repository":"https://example.test/clm","components":["box","databricks"]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "not available") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestPackageAssemblyUsesConfiguredTemplateAndKeepsWorkspacePrivate(t *testing.T) {
	var saved config.SolutionPlan
	var assembledTemplate packageTemplate
	var assembledComponents []string
	handler := NewHandlerWithOptions(ServerOptions{
		Templates: func() ([]packageTemplate, error) {
			return []packageTemplate{{ID: "clm", Name: "Contract Lifecycle Management", Description: "Contracts", repository: "https://example.test/clm"}}, nil
		},
		PackageAssembler: func(template packageTemplate, components []string, strategy string) (config.SolutionPlan, error) {
			assembledTemplate = template
			assembledComponents = append([]string(nil), components...)
			return config.SolutionPlan{TemplateID: template.ID, Template: template.Name, Repository: template.repository, Components: components, Strategy: strategy, PackagePath: "/private/web-package"}, nil
		},
		PlanSaver:       func(plan config.SolutionPlan) error { saved = plan; return nil },
		ConnectionStore: func() (config.ConnectionSettings, error) { return config.ConnectionSettings{}, nil },
	})

	request := httptest.NewRequest(http.MethodPost, "/api/packages", strings.NewReader(`{"name":"Northstar CLM rollout","templateId":"clm","components":["box","salesforce"]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if assembledTemplate.ID != "clm" || assembledTemplate.repository != "https://example.test/clm" || strings.Join(assembledComponents, ",") != "box,salesforce" {
		t.Fatalf("assembly = %#v %#v", assembledTemplate, assembledComponents)
	}
	if saved.PackagePath != "/private/web-package" || saved.Name != "Northstar CLM rollout" {
		t.Fatalf("saved plan = %#v", saved)
	}
	for _, forbidden := range []string{"/private/web-package"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestPackageAssemblyRejectsUnknownTemplateAndUnsupportedProviders(t *testing.T) {
	handler := NewHandlerWithOptions(ServerOptions{
		Templates: func() ([]packageTemplate, error) {
			return []packageTemplate{{ID: "clm", Name: "CLM", repository: "https://example.test/clm"}}, nil
		},
	})
	for _, body := range []string{
		`{"name":"Test deployment","templateId":"unknown","components":["box"]}`,
		`{"name":"Test deployment","templateId":"clm","components":["box","databricks"]}`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/packages", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400: %s", body, response.Code, response.Body.String())
		}
	}
}
