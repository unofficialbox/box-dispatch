package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPickBoxTargetSelectsAuthenticatedEnterpriseTab(t *testing.T) {
	targets := []*chromedpTarget{
		{ID: "a", Type: "page", URL: "https://news.example.com"},
		{ID: "b", Type: "background_page", URL: "https://kadams.ent.box.com/extension"},
		{ID: "c", Type: "page", URL: "https://kadams.ent.box.com/folder/0"},
	}
	got, err := pickBoxTarget(targets)
	if err != nil {
		t.Fatal(err)
	}
	// Must pick the page, not the extension background page on the same host.
	if got != "c" {
		t.Fatalf("picked %q, want the enterprise Box page", got)
	}
}

func TestPickBoxTargetRequiresABoxTab(t *testing.T) {
	targets := []*chromedpTarget{{ID: "a", Type: "page", URL: "https://app.box.com/folder/0"}}
	// app.box.com is not the authenticated enterprise host the script guards on.
	if _, err := pickBoxTarget(targets); err == nil {
		t.Fatal("expected an error when no enterprise Box tab is open")
	}
}

func TestCDPWebSocketURLReadsDevtoolsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
	}))
	defer server.Close()

	got, err := cdpWebSocketURL(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("websocket url = %q", got)
	}
}

func TestCDPWebSocketURLFailsWithoutAnEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if _, err := cdpWebSocketURL(context.Background(), server.URL); err == nil {
		t.Fatal("expected an error when no websocket url is advertised")
	}
}

func TestCDPEndpointHonoursOverride(t *testing.T) {
	t.Setenv(cdpEndpointEnv, "http://localhost:9333/")
	if got := cdpEndpoint(); got != "http://localhost:9333" {
		t.Fatalf("endpoint = %q, want the override without a trailing slash", got)
	}
	t.Setenv(cdpEndpointEnv, "")
	if got := cdpEndpoint(); got != defaultCDPEndpoint {
		t.Fatalf("endpoint = %q, want the default", got)
	}
}
