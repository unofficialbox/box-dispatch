package salesforceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadMultiFrameworkEligibilityIdentifiesHyperforceOrg(t *testing.T) {
	server := multiFrameworkTestServer(t, "USA470", "en_US")
	defer server.Close()

	eligibility, err := (&Client{HTTP: server.Client()}).ReadMultiFrameworkEligibility(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if !eligibility.Hyperforce || !eligibility.EnglishDefault || !eligibility.SupportedRelease || eligibility.InstanceName != "USA470" || eligibility.LanguageLocale != "en_US" || eligibility.APIVersion != "67.0" {
		t.Fatalf("eligibility = %#v", eligibility)
	}
}

func TestEnglishLanguageLocaleAcceptsSalesforceEnglishFormats(t *testing.T) {
	for _, locale := range []string{"en", "en_US", "en-GB"} {
		if !isEnglishLanguageLocale(locale) {
			t.Fatalf("expected %q to be English", locale)
		}
	}
	if isEnglishLanguageLocale("de_DE") {
		t.Fatal("German must not be accepted as the default English language")
	}
}

func TestReadMultiFrameworkEligibilityIdentifiesFirstPartyOrg(t *testing.T) {
	server := multiFrameworkTestServer(t, "CS248", "en_US")
	defer server.Close()

	eligibility, err := (&Client{HTTP: server.Client()}).ReadMultiFrameworkEligibility(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if eligibility.Hyperforce || !eligibility.EnglishDefault || eligibility.InstanceName != "CS248" {
		t.Fatalf("eligibility = %#v", eligibility)
	}
}

func TestReadMultiFrameworkEligibilityRejectsMissingOrgProperties(t *testing.T) {
	server := multiFrameworkTestServer(t, "", "en_US")
	defer server.Close()

	_, err := (&Client{HTTP: server.Client()}).ReadMultiFrameworkEligibility(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"})
	if err == nil || !strings.Contains(err.Error(), "no instance name") {
		t.Fatalf("error = %v", err)
	}
}

func TestHyperforceInstanceNameRequiresThreeLetterPrefix(t *testing.T) {
	for _, instance := range []string{"USA470", "GBR10", "aus12"} {
		if !isHyperforceInstanceName(instance) {
			t.Fatalf("expected %q to be Hyperforce", instance)
		}
	}
	for _, instance := range []string{"CS248", "NA1", "USA", "USA-470", ""} {
		if isHyperforceInstanceName(instance) {
			t.Fatalf("expected %q to be first-party or unknown", instance)
		}
	}
}

func multiFrameworkTestServer(t *testing.T, instanceName, language string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/data/":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"version": "66.0"}, {"version": "67.0"}})
		case r.URL.Path == "/services/data/v67.0/query":
			if query := r.URL.Query().Get("q"); query != "SELECT InstanceName, LanguageLocaleKey FROM Organization" {
				t.Fatalf("query = %q", query)
			}
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"records": []map[string]string{{"InstanceName": instanceName, "LanguageLocaleKey": language}}})
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
}
