package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSalesforceLoginRedirectHandlerServesOauthRedirect(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/OauthRedirect" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("login-ok"))
	})
	handler := salesforceLoginRedirectHandler(api)

	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/OauthRedirect?code=auth-code", nil))
	if ok.Code != http.StatusOK || ok.Body.String() != "login-ok" {
		t.Fatalf("oauth redirect = %d %q", ok.Code, ok.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("other path = %d", missing.Code)
	}
}

func TestBoxLoginRedirectHandlerServesOAuthCallback(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/callback" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("box-login-ok"))
	})
	handler := boxLoginRedirectHandler(api)

	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/oauth/callback?code=auth-code", nil))
	if ok.Code != http.StatusOK || ok.Body.String() != "box-login-ok" {
		t.Fatalf("oauth redirect = %d %q", ok.Code, ok.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("other path = %d", missing.Code)
	}
}

func TestDispatchWebHandlerServesUIAndAPI(t *testing.T) {
	handler := dispatchWebHandler("demo")

	ui := httptest.NewRecorder()
	handler.ServeHTTP(ui, httptest.NewRequest(http.MethodGet, "/", nil))
	if ui.Code != http.StatusOK || !strings.Contains(ui.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("UI response = %d %q", ui.Code, ui.Body.String())
	}

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if api.Code != http.StatusOK || !strings.Contains(api.Body.String(), `"profile":"demo"`) {
		t.Fatalf("API response = %d %q", api.Code, api.Body.String())
	}
}

func TestRootExposesWebApplicationFlags(t *testing.T) {
	root := newRootCommand()
	if flag := root.Flags().Lookup("port"); flag == nil {
		t.Fatal("root command omitted --port")
	}
	if flag := root.Flags().Lookup("no-open"); flag == nil {
		t.Fatal("root command omitted --no-open")
	}
}
