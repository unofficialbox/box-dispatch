package boxconn

import "testing"

func TestParseCLIEnvironmentsExtractsNameAndAuthType(t *testing.T) {
	// Shape of `box configure:environments:get`, with ANSI colour codes.
	out := "\x1b[36mKadams1:\x1b[39m\n    Name: kadams1\n    Auth Method: oauth20\n" +
		"Oauth:\n    Name: oauth\n    Auth Method: oauth20\n" +
		"Box-cmis-increo-5105484:\n    Name: box-cmis-increo-5105484\n    Auth Method: ccg\n"
	got := ParseCLIEnvironments(out)
	if len(got) != 3 {
		t.Fatalf("got %d connections, want 3: %+v", len(got), got)
	}
	want := map[string]string{"kadams1": "OAuth2", "oauth": "OAuth2", "box-cmis-increo-5105484": "CCG"}
	for _, c := range got {
		if c.AuthType != want[c.Name] {
			t.Fatalf("%s auth = %q, want %q", c.Name, c.AuthType, want[c.Name])
		}
		if c.Source != SourceCLI {
			t.Fatalf("%s source = %q, want cli", c.Name, c.Source)
		}
	}
}

func TestAuthLabelNormalisesMethods(t *testing.T) {
	for method, want := range map[string]string{
		"oauth20": "OAuth2", "ccg": "CCG", "jwt": "JWT", "weird": "weird",
	} {
		if got := authLabel(method); got != want {
			t.Fatalf("authLabel(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestMarkStateFlagsCurrentAndDefault(t *testing.T) {
	conns := []Connection{
		{Name: "oauth", Source: SourceCLI},
		{Name: "box-cmis-increo-5105484", Source: SourceCLI},
		{Name: DispatchCCGName, Source: SourceDispatch},
	}
	conns = markState(conns, "oauth", DispatchCCGName)
	if !conns[0].Current || conns[1].Current || conns[2].Current {
		t.Fatalf("Current flags wrong: %+v", conns)
	}
	if conns[0].Default || conns[1].Default || !conns[2].Default {
		t.Fatalf("Default flags wrong: %+v", conns)
	}
	// A box-dispatch connection is never the CLI current, even if names collided.
	if conns[2].Current {
		t.Fatal("a dispatch connection must not be marked CLI-current")
	}
}
