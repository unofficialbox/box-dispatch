package lifecycle

import (
	"testing"

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
