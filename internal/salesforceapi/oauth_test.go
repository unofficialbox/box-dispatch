package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizationURLUsesPKCEAndLoopbackRedirect(t *testing.T) {
	got, err := AuthorizationURL(AuthorizationRequest{
		LoginURL:      "production",
		ClientID:      "dispatch-client",
		RedirectURL:   "http://127.0.0.1:8787/api/connections/salesforce/oauth/callback",
		State:         "state-1",
		CodeChallenge: "challenge-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "login.salesforce.com" || parsed.Path != "/services/oauth2/authorize" {
		t.Fatalf("authorize URL = %s", got)
	}
	query := parsed.Query()
	if query.Get("response_type") != "code" || query.Get("client_id") != "dispatch-client" || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != "challenge-1" {
		t.Fatalf("query = %s", parsed.RawQuery)
	}
	if !strings.Contains(query.Get("scope"), "refresh_token") {
		t.Fatalf("scope = %q", query.Get("scope"))
	}
}

func TestNormalizeLoginURLRejectsNonSalesforceHosts(t *testing.T) {
	if _, err := NormalizeLoginURL("https://example.com"); err == nil {
		t.Fatal("expected non-Salesforce host to fail")
	}
	got, err := NormalizeLoginURL("sandbox")
	if err != nil || got != DefaultSandboxLogin {
		t.Fatalf("sandbox = %q err=%v", got, err)
	}
}

func TestExchangeAuthorizationCodeUsesPKCE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/oauth2/token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "auth-code" || r.Form.Get("code_verifier") != "verifier" || r.Form.Get("client_id") != "client-id" {
			t.Fatalf("form = %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access", "refresh_token": "refresh", "instance_url": "https://org.my.salesforce.com"})
	}))
	defer server.Close()

	token, err := (&Client{HTTP: server.Client()}).ExchangeAuthorizationCode(context.Background(), AuthorizationCodeRequest{
		LoginURL: server.URL, ClientID: "client-id", RedirectURL: "http://127.0.0.1:8787/callback", Code: "auth-code", CodeVerifier: "verifier",
	})
	if err != nil || token.AccessToken != "access" || token.RefreshToken != "refresh" || token.InstanceURL != "https://org.my.salesforce.com" {
		t.Fatalf("token=%#v err=%v", token, err)
	}
}

func TestExchangeAuthorizationCodeOmitsEmptyPKCEVerifier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Form["code_verifier"]; ok {
			t.Fatalf("empty PKCE verifier must be omitted: %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "access", "instance_url": "https://org.my.salesforce.com"})
	}))
	defer server.Close()

	token, err := (&Client{HTTP: server.Client()}).ExchangeAuthorizationCode(context.Background(), AuthorizationCodeRequest{
		LoginURL: server.URL, ClientID: ScratchSignupClientID, RedirectURL: ScratchSignupCallbackURL, Code: "auth-code",
	})
	if err != nil || token.AccessToken != "access" {
		t.Fatalf("token=%#v err=%v", token, err)
	}
}

func TestExchangeAuthorizationCodeSurfacesOAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "invalid code verifier"})
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client()}).ExchangeAuthorizationCode(context.Background(), AuthorizationCodeRequest{
		LoginURL: server.URL, ClientID: ScratchSignupClientID, RedirectURL: ScratchSignupCallbackURL, Code: "auth-code",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid code verifier") {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshAccessTokenKeepsExistingRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh" {
			t.Fatalf("form = %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "next-access", "instance_url": "https://org.my.salesforce.com"})
	}))
	defer server.Close()

	token, err := (&Client{HTTP: server.Client()}).RefreshAccessToken(context.Background(), RefreshRequest{
		LoginURL: server.URL, ClientID: "client-id", RefreshToken: "refresh",
	})
	if err != nil || token.AccessToken != "next-access" || token.RefreshToken != "refresh" {
		t.Fatalf("token=%#v err=%v", token, err)
	}
}
