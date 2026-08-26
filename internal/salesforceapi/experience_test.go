package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPublishExperienceSiteWaitsForSalesforce(t *testing.T) {
	statuses := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/services/data/":
			json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0", "url": "/services/data/v67.0"}})
		case "/services/data/v67.0/connect/communities":
			json.NewEncoder(w).Encode(map[string]any{"communities": []map[string]string{{"id": "0DB1", "name": "CLM Experience", "status": "UnderConstruction", "siteUrl": "https://example.my.site.com/clm"}}})
		case "/services/data/v67.0/connect/communities/0DB1/publish":
			json.NewEncoder(w).Encode(map[string]string{"id": "0DB1", "jobId": "08P1", "name": "CLM Experience", "url": "https://example.my.site.com/clm"})
		case "/services/data/v67.0/query":
			if strings.Contains(r.URL.Query().Get("q"), "Message") {
				t.Fatalf("publish status query requested unsupported Message field: %s", r.URL.Query().Get("q"))
			}
			json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Status": "Complete"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{HTTP: server.Client(), PollInterval: time.Millisecond}
	site, err := client.PublishExperienceSite(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", "CLM Experience", func(status string) {
		statuses = append(statuses, status)
	})
	if err != nil {
		t.Fatal(err)
	}
	if site.ID != "0DB1" || site.Status != "Live" || site.URL != "https://example.my.site.com/clm" {
		t.Fatalf("unexpected site: %#v", site)
	}
	if !slices.Equal(statuses, []string{"queued", "Complete"}) {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestPublishExperienceSiteRequiresMetadataCreatedSite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/services/data/" {
			json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0", "url": "/services/data/v67.0"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"communities": []any{}})
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client()}).PublishExperienceSite(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", "CLM Experience", nil)
	if err == nil {
		t.Fatal("expected missing site error")
	}
}

func TestExperienceEmployeePathResolvesActiveSiteContainer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/services/data/":
			json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0", "url": "/services/data/v67.0"}})
		case "/services/data/v67.0/connect/communities":
			json.NewEncoder(w).Encode(map[string]any{"communities": []map[string]string{{"id": "0DB1", "name": "CLM Experience", "siteUrl": "https://example.my.site.com/clm"}}})
		case "/services/data/v67.0/query":
			query := r.URL.Query().Get("q")
			if !strings.Contains(query, "FROM Site") || !strings.Contains(query, "UrlPathPrefix='clm'") || !strings.Contains(query, "Status='Active'") {
				t.Fatalf("query = %q", query)
			}
			json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"Id": "0DM1"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path, err := (&Client{HTTP: server.Client()}).ExperienceEmployeePath(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", "0DB1")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/servlet/networks/session/create?site=0DM1" {
		t.Fatalf("path = %q", path)
	}
}
