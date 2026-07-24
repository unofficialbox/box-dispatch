package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/config"
)

func TestBoxCCGUserTokenSendsUserSubjectGrant(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"ccg-abc","expires_in":3600}`))
	}))
	defer server.Close()

	orig := boxTokenURL
	boxTokenURL = server.URL // overridden below via a small indirection
	defer func() { boxTokenURL = orig }()

	token, err := boxCCGToken(context.Background(), "cid", "csecret", "user", "385982796")
	if err != nil {
		t.Fatal(err)
	}
	if token != "ccg-abc" {
		t.Fatalf("token = %q", token)
	}
	if gotForm.Get("grant_type") != "client_credentials" {
		t.Fatalf("grant_type = %q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("box_subject_type") != "user" || gotForm.Get("box_subject_id") != "385982796" {
		t.Fatalf("subject = %q/%q, want user/385982796", gotForm.Get("box_subject_type"), gotForm.Get("box_subject_id"))
	}
	if gotForm.Get("client_id") != "cid" || gotForm.Get("client_secret") != "csecret" {
		t.Fatal("client credentials not sent")
	}
}

func TestBoxCCGUserTokenSurfacesOAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"client secret is invalid"}`))
	}))
	defer server.Close()
	orig := boxTokenURL
	boxTokenURL = server.URL
	defer func() { boxTokenURL = orig }()

	if _, err := boxCCGToken(context.Background(), "cid", "bad", "user", "1"); err == nil {
		t.Fatal("expected an error for a rejected client")
	} else if !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("error should carry the OAuth reason: %v", err)
	}
}

func TestHasBoxCCGRequiresAllThreeFields(t *testing.T) {
	full := config.ConnectionSettings{BoxCCGClientID: "a", BoxCCGClientSecret: "b", BoxCCGSubjectType: "user", BoxCCGSubjectID: "c"}
	if !full.HasBoxCCG() {
		t.Fatal("a complete CCG set should be recognised")
	}
	for _, partial := range []config.ConnectionSettings{
		{BoxCCGClientID: "a", BoxCCGClientSecret: "b"},
		{BoxCCGClientID: "a", BoxCCGSubjectType: "user", BoxCCGSubjectID: "c"},
		{},
	} {
		if partial.HasBoxCCG() {
			t.Fatalf("incomplete CCG set should not qualify: %+v", partial)
		}
	}
}
