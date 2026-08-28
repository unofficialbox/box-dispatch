package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

func TestVerifyBoxConnectionChecksActingUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_id") != "client-id" || r.Form.Get("box_subject_id") != "12345" {
				t.Fatalf("token form = %#v", r.Form)
			}
			_, _ = w.Write([]byte(`{"access_token":"token-value"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer token-value" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"id":"12345","login":"box-user@example.com","hostname":"https://acme.app.box.com/","enterprise":{"id":"98765"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	originalTokenURL := boxconn.TokenURL
	originalUserURL := boxCurrentUserURL
	boxconn.TokenURL = server.URL + "/token"
	boxCurrentUserURL = server.URL + "/user"
	t.Cleanup(func() {
		boxconn.TokenURL = originalTokenURL
		boxCurrentUserURL = originalUserURL
	})

	verification, err := verifyBoxConnection(context.Background(), config.ConnectionSettings{BoxCCGClientID: "client-id", BoxCCGClientSecret: "secret", BoxCCGSubjectType: "user", BoxCCGSubjectID: "12345"})
	if err != nil {
		t.Fatal(err)
	}
	if verification.Identity != "box-user@example.com" || verification.Account != "12345" || verification.Enterprise != "98765" || verification.Host != "https://acme.app.box.com/" || verification.AuthType != "CCG" {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestSafeConnectionHostnameOnlyReturnsHTTPSHost(t *testing.T) {
	for input, want := range map[string]string{
		"https://Acme.App.Box.com/folder/123?token=secret": "acme.app.box.com",
		"https://example.my.salesforce.com/services/data":  "example.my.salesforce.com",
		"http://example.my.salesforce.com/":                "",
		"https://user:secret@example.com/":                 "",
		"not a URL":                                        "",
	} {
		if got := safeConnectionHostname(input); got != want {
			t.Fatalf("safeConnectionHostname(%q) = %q, want %q", input, got, want)
		}
	}
}
