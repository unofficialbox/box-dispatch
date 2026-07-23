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
