package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
