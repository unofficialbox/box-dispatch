package main

import (
	"encoding/json"
	"errors"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

type dispatchTestTransport func(*http.Request) (*http.Response, error)

func (transport dispatchTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestWebApplicationEntryCoordinatesDirectConnectionWritesDuringValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	restoreGuard := shellstate.SetConnectionWriteGuard(nil)
	t.Cleanup(restoreGuard)
	packagePath := filepath.Join(t.TempDir(), "package")
	if err := os.MkdirAll(packagePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packagePath, "fixture.txt"), []byte("package"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := config.ConnectionSettings{}.UpsertBoxConnection(config.BoxAppConnection{ID: "fixture-box", ClientID: "fixture-client", ClientSecret: "fixture-secret", SubjectType: "user", SubjectID: "123"}, true)
	if err := shellstate.SaveConnectionSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := shellstate.SaveSolutionPlan(config.SolutionPlan{TemplateID: "clm", Name: "Entry guard", PackagePath: packagePath, Components: []string{"box"}}); err != nil {
		t.Fatal(err)
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	var release sync.Once
	priorTransport := http.DefaultTransport
	http.DefaultTransport = dispatchTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.box.com" || request.URL.Path != "/oauth2/token" {
			return nil, errors.New("unexpected provider request in hermetic entry test")
		}
		close(entered)
		<-finish
		return &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`)), Request: request}, nil
	})
	t.Cleanup(func() { release.Do(func() { close(finish) }); http.DefaultTransport = priorTransport })
	handler := newWebApplicationAPI("entry-fixture", func() bool { return true }, func() bool { return true })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/runs", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("validation start=%d: %s", response.Code, response.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("validation did not reach held local transport")
	}
	updateDone := make(chan error, 1)
	go func() {
		_, err := shellstate.UpdateConnectionSettings(func(current config.ConnectionSettings) (config.ConnectionSettings, error) {
			current.BoxConnections[0].AccessToken = "fixture-rotated-access"
			return current, nil
		})
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("same-target update=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("same-target update deadlocked behind run lease")
	}
	_, err := shellstate.UpdateConnectionSettings(func(current config.ConnectionSettings) (config.ConnectionSettings, error) {
		current.BoxConnections[0].SubjectID = "different"
		return current.HydrateBoxConnections(), nil
	})
	if !errors.Is(err, config.ErrConnectionConflict) {
		t.Fatalf("direct target update escaped entry lease: %v", err)
	}
	captured, err := shellstate.LoadConnectionSettings()
	if err != nil {
		t.Fatal(err)
	}
	captured.BoxConnections[0].SubjectID = "different"
	if err := shellstate.SaveConnectionSettings(captured); !errors.Is(err, config.ErrConnectionConflict) {
		t.Fatalf("direct whole-state save escaped entry lease: %v", err)
	}
	release.Do(func() { close(finish) })
	deadline := time.Now().Add(time.Second)
	terminal := false
	for time.Now().Before(deadline) {
		status := httptest.NewRecorder()
		handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/runs/"+run.ID, nil))
		if strings.Contains(status.Body.String(), `"status":"failed"`) {
			terminal = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !terminal {
		t.Fatal("held hermetic validation did not terminate")
	}
	if err := shellstate.SaveConnectionSettings(captured); err != nil {
		t.Fatalf("terminal run did not release entry lease: %v", err)
	}
}
