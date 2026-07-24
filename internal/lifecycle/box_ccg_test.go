package lifecycle

import (
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
)

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

func TestPrefersBoxCCGHonoursPinnedDefault(t *testing.T) {
	full := config.ConnectionSettings{
		BoxCCGClientID: "id", BoxCCGClientSecret: "secret",
		BoxCCGSubjectType: "user", BoxCCGSubjectID: "123",
	}
	cases := []struct {
		name   string
		pin    string
		hasCCG bool
		want   bool
	}{
		{"no pin, CCG configured", "", true, true},
		{"pinned to dispatch CCG", boxconn.DispatchCCGName, true, true},
		{"pinned to a CLI env wins over CCG", "oauth", true, false},
		{"no CCG configured", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := full
			s.BoxDefaultConnection = tc.pin
			if !tc.hasCCG {
				s = config.ConnectionSettings{BoxDefaultConnection: tc.pin}
			}
			if got := prefersBoxCCG(s); got != tc.want {
				t.Fatalf("prefersBoxCCG = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostFromAvatarURLExtractsTenantOrigin(t *testing.T) {
	cases := map[string]string{
		"https://kadams.ent.box.com/api/avatar/large/385982796": "https://kadams.ent.box.com",
		"https://acme.ent.box.com/x":                            "https://acme.ent.box.com",
		"https://app.box.com/api/avatar/large/1":                "", // generic host, not a tenant
		"https://example.com/pic":                               "",
		"":                                                      "",
		"   ":                                                   "",
	}
	for in, want := range cases {
		if got := hostFromAvatarURL(in); got != want {
			t.Fatalf("hostFromAvatarURL(%q) = %q, want %q", in, got, want)
		}
	}
}
