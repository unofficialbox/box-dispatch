package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestCDPPortFallsBackToDefault(t *testing.T) {
	if got := cdpPort("http://127.0.0.1:9333"); got != "9333" {
		t.Fatalf("port = %q, want 9333", got)
	}
	if got := cdpPort("not a url"); got != "9222" {
		t.Fatalf("port = %q, want the 9222 default", got)
	}
}

func TestChromeAutoLaunchCanBeDisabled(t *testing.T) {
	t.Setenv(chromeAutoLaunchEnv, "false")
	if chromeAutoLaunchEnabled() {
		t.Fatal("auto launch should be disabled")
	}
	if err := launchChrome(context.Background(), "http://127.0.0.1:9222"); err == nil {
		t.Fatal("launch must refuse when auto launch is disabled")
	}
	t.Setenv(chromeAutoLaunchEnv, "")
	if !chromeAutoLaunchEnabled() {
		t.Fatal("auto launch should default to enabled")
	}
}

func TestChromeExecutableHonoursOverride(t *testing.T) {
	t.Setenv(chromeExecutableEnv, filepath.Join(t.TempDir(), "does-not-exist"))
	if _, err := chromeExecutable(); err == nil {
		t.Fatal("a missing override path should error rather than silently fall through")
	}
	// A real file satisfies the override.
	stub := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(chromeExecutableEnv, stub)
	got, err := chromeExecutable()
	if err != nil || got != stub {
		t.Fatalf("executable = %q, err = %v; want the override", got, err)
	}
}

func TestChromeCandidatesAreProvidedForThisPlatform(t *testing.T) {
	if len(chromeCandidates()) == 0 {
		t.Fatal("no browser candidates for this platform")
	}
}

func TestPickBoxTargetIgnoresLoginPageWithTenantRedirect(t *testing.T) {
	// The Box login URL carries the tenant address in its redirect query, so a
	// substring match would select the login tab and script an unauthenticated page.
	login := "https://kadams.account.box.com/login?redirect_url=https%3A%2F%2Fkadams.ent.box.com%2F"
	if _, err := pickBoxTarget([]*chromedpTarget{{ID: "login", Type: "page", URL: login}}); err == nil {
		t.Fatal("a login page must never be selected as the authenticated Box tab")
	}
	// The real tenant tab is still selected when both are open.
	targets := []*chromedpTarget{
		{ID: "login", Type: "page", URL: login},
		{ID: "tenant", Type: "page", URL: "https://kadams.ent.box.com/folder/0"},
	}
	got, err := pickBoxTarget(targets)
	if err != nil || got != "tenant" {
		t.Fatalf("picked %q (err %v), want the tenant tab", got, err)
	}
}

func TestFirstPageTargetIDReusesAnExistingTab(t *testing.T) {
	// Only a "page" can be navigated; workers and background pages are skipped so a
	// half-open browser is driven through a tab it already has rather than a fresh
	// target it cannot create (the CDP -32000 case).
	targets := []*chromedpTarget{
		{Type: "service_worker", ID: "sw"},
		{Type: "background_page", ID: "bg"},
		{Type: "page", ID: "app-box-tab", URL: "https://app.box.com"},
		{Type: "page", ID: "second"},
	}
	if got := firstPageTargetID(targets); got != "app-box-tab" {
		t.Fatalf("firstPageTargetID = %q, want the first page tab", got)
	}
	if got := firstPageTargetID([]*chromedpTarget{{Type: "service_worker", ID: "x"}}); got != "" {
		t.Fatalf("no page tab should yield empty, got %q", got)
	}
	if got := firstPageTargetID(nil); got != "" {
		t.Fatalf("empty target list should yield empty, got %q", got)
	}
}
