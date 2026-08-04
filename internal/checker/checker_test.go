package checker

import (
	"encoding/json"
	"testing"
)

func TestMergeDiscoveryRefinesWithoutErasing(t *testing.T) {
	dst := ProviderDiscovery{Profile: "agentforce", Options: []string{"agentforce", "sandbox"}}
	mergeDiscovery(&dst, ProviderDiscovery{Identity: "kadams@boxdemo.com", Host: "https://example.my.salesforce.com"})

	if dst.Identity != "kadams@boxdemo.com" || dst.Host != "https://example.my.salesforce.com" {
		t.Fatalf("connectivity fields were not applied: %+v", dst)
	}
	if dst.Profile != "agentforce" {
		t.Fatalf("locally discovered profile was erased: %q", dst.Profile)
	}
	if len(dst.Options) != 2 {
		t.Fatalf("locally discovered options were erased: %v", dst.Options)
	}
}

func TestMergeDiscoveryIgnoresBlankFields(t *testing.T) {
	dst := ProviderDiscovery{Identity: "kadams@boxdemo.com", Region: "us-east-1"}
	mergeDiscovery(&dst, ProviderDiscovery{Identity: "", Region: "   "})

	if dst.Identity != "kadams@boxdemo.com" || dst.Region != "us-east-1" {
		t.Fatalf("blank values overwrote discovered fields: %+v", dst)
	}
}

func TestAWSIdentityFromARN(t *testing.T) {
	for arn, want := range map[string]string{
		"arn:aws:iam::385982796:user/kadams":                     "kadams",
		"arn:aws:sts::385982796:assumed-role/Admin/session-name": "session-name",
		"":       "",
		"opaque": "opaque",
	} {
		if got := awsIdentityFromARN(arn); got != want {
			t.Fatalf("awsIdentityFromARN(%q) = %q, want %q", arn, got, want)
		}
	}
}

func TestBoxUserDiscoveryMapsFields(t *testing.T) {
	var user boxUser
	if err := json.Unmarshal([]byte(`{"id":"385982796","login":"kadams@boxdemo.com","enterprise":{"id":"5105484"}}`), &user); err != nil {
		t.Fatal(err)
	}
	d := user.discovery()
	if d.Identity != "kadams@boxdemo.com" || d.Account != "385982796" || d.Enterprise != "5105484" {
		t.Fatalf("unexpected discovery: %+v", d)
	}
}

func TestSalesforceCLIErrorExtractsMessage(t *testing.T) {
	// ANSI-colored JSON error, as the sf CLI emits it.
	raw := "Warning: CLI update available.\n\x1b[97m{\x1b[39m\n  \x1b[94m\"name\"\x1b[39m: \x1b[92m\"NoDefaultEnvError\"\x1b[39m,\n  \x1b[94m\"message\"\x1b[39m: \x1b[92m\"No default environment found. Use -o or --target-org to specify an environment.\"\x1b[39m\n\x1b[97m}\x1b[39m"
	if got := salesforceCLIError(raw); got != "No default environment found. Use -o or --target-org to specify an environment." {
		t.Fatalf("salesforceCLIError = %q", got)
	}
	// Non-JSON output falls back to the first non-empty line, ANSI stripped.
	if got := salesforceCLIError("\x1b[31mcommand not found\x1b[39m\nextra"); got != "command not found" {
		t.Fatalf("fallback salesforceCLIError = %q", got)
	}
}

func TestDecodeCLIJSONSkipsWarningPreamble(t *testing.T) {
	var payload struct {
		Result struct {
			Alias string `json:"alias"`
		} `json:"result"`
	}
	if err := decodeCLIJSON("Warning: CLI update available.\n{\"result\":{\"alias\":\"agentforce\"}}", &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Result.Alias != "agentforce" {
		t.Fatalf("alias = %q, want agentforce", payload.Result.Alias)
	}
}

func TestDatabricksCLIIdentitySkipsPreamble(t *testing.T) {
	// The CLI prints a non-JSON warning line before the JSON body.
	out := "Databricks skills are not installed.\n{\n  \"userName\": \"kadams@box.com\",\n  \"emails\": [{\"value\": \"kadams@box.com\", \"primary\": true}]\n}"
	if got := databricksCLIIdentity(out); got != "kadams@box.com" {
		t.Fatalf("databricksCLIIdentity = %q", got)
	}
	// Falls back to the primary email when userName is absent.
	noUser := "{\n  \"emails\": [{\"value\": \"a@b.com\", \"primary\": false}, {\"value\": \"primary@b.com\", \"primary\": true}]\n}"
	if got := databricksCLIIdentity(noUser); got != "primary@b.com" {
		t.Fatalf("databricksCLIIdentity fallback = %q", got)
	}
	if got := databricksCLIIdentity("no json here"); got != "" {
		t.Fatalf("databricksCLIIdentity on junk = %q, want empty", got)
	}
}

func TestParseDatabricksProfilesKeepsOnlyRealProfiles(t *testing.T) {
	cfg := "[DEFAULT]\n\n[__settings__]\nfoo = bar\n\n[windlass]\nhost = https://x.cloud.databricks.com\ntoken = abc\n\n[prod]\nhost = https://y.cloud.databricks.com\n"
	got := parseDatabricksProfiles([]byte(cfg))
	want := []string{"windlass", "prod"}
	if len(got) != len(want) {
		t.Fatalf("profiles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("profiles = %v, want %v", got, want)
		}
	}
}

func TestFirstLineCollapsesMultilineOutput(t *testing.T) {
	if got := firstLine("\n\nMust provide app auth configuration\nsecond line"); got != "Must provide app auth configuration" {
		t.Fatalf("firstLine = %q", got)
	}
	if got := firstLine("   only line   "); got != "only line" {
		t.Fatalf("firstLine = %q", got)
	}
}

func TestDedupePreservesFirstOccurrenceOrder(t *testing.T) {
	got := dedupe([]string{"agentforce", "sandbox", "agentforce", "prod"})
	want := []string{"agentforce", "sandbox", "prod"}
	if len(got) != len(want) {
		t.Fatalf("dedupe = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe = %v, want %v", got, want)
		}
	}
}
