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

func TestFindRecordsByStringFieldReturnsImmutableIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "67.0", "url": "/services/data/v67.0"}})
		case "/services/data/v67.0/query":
			query, _ := url.QueryUnescape(r.URL.Query().Get("q"))
			if !strings.Contains(query, "FROM CLM_Contract__c") || !strings.Contains(query, "Contract_ID__c IN") {
				t.Fatalf("query = %q", query)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"Id": "a01-record", "Name": "Northstar", "Contract_ID__c": "CLM-1"}}})
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	records, err := (&Client{HTTP: server.Client()}).FindRecordsByStringField(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "CLM_Contract__c", "Contract_ID__c", []string{"CLM-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "a01-record" || records[0].Key != "CLM-1" {
		t.Fatalf("records = %#v", records)
	}
}

func TestFindRecordsByStringFieldRejectsUnsafeIdentifiers(t *testing.T) {
	_, err := (&Client{}).FindRecordsByStringField(context.Background(), Credential{InstanceURL: "https://example.com", AccessToken: "token"}, "Contract__c WHERE", "Name", []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "invalid object or field") {
		t.Fatalf("err = %v", err)
	}
}
