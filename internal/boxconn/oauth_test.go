package boxconn

import (
	"net/url"
	"testing"
)

func TestAuthorizationURLUsesPKCEAndFixedRedirect(t *testing.T) {
	authorize, err := AuthorizationURL(AuthorizationRequest{
		ClientID: "box-client", RedirectURL: LoginCallbackURL, State: "state-1", CodeChallenge: "challenge-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorize)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != AuthorizeURL || query.Get("client_id") != "box-client" || query.Get("redirect_uri") != LoginCallbackURL || query.Get("code_challenge") != "challenge-1" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorize URL = %s", authorize)
	}
}

func TestAuthorizationURLRequiresClientID(t *testing.T) {
	if _, err := AuthorizationURL(AuthorizationRequest{RedirectURL: LoginCallbackURL, State: "state", CodeChallenge: "challenge"}); err == nil {
		t.Fatal("expected a missing client ID error")
	}
}

func TestHasBoxOAuthAppReadsEnvironment(t *testing.T) {
	t.Setenv("BOX_CLIENT_ID", "box-client")
	t.Setenv("BOX_CLIENT_SECRET", "box-secret")
	if !HasBoxOAuthApp() {
		t.Fatal("expected Box OAuth app from environment")
	}
	t.Setenv("BOX_CLIENT_SECRET", "")
	if HasBoxOAuthApp() {
		t.Fatal("secret is required")
	}
}
