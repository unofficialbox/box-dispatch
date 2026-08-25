package webapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPRecorderCapturesAndRedactsJSONExchange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.jsonl")
	recorder, err := NewHTTPRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := recorder.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"accessToken":"response-secret","ok":true}`)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/example?code=query-secret", strings.NewReader(`{"clientSecret":"request-secret","name":"CLM"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("recording is empty")
	}
	var entry httpTraceEntry
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Request.Path != "/api/example" || entry.Response.Status != http.StatusCreated {
		t.Fatalf("entry = %#v", entry)
	}
	encoded := string(scanner.Bytes())
	for _, secret := range []string{"request-secret", "response-secret", "query-secret"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("recording leaked %q: %s", secret, encoded)
		}
	}
	if strings.Count(encoded, "[REDACTED]") != 2 {
		t.Fatalf("recording did not redact both JSON secrets: %s", encoded)
	}
}

func TestHTTPRecorderPreservesSSEFlushing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	recorder, err := NewHTTPRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := recorder.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: dispatch\ndata: {\"status\":\"completed\"}\n\n")
		w.(http.Flusher).Flush()
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/runs/mock/events", nil))
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `event: dispatch`) || !strings.Contains(response.Body.String(), `"completed"`) {
		t.Fatalf("SSE was not preserved: response=%q recording=%q", response.Body.String(), data)
	}
}
