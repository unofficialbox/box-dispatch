package salesforceorg

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHealthFailureRejectsDeletedScratchOrg(t *testing.T) {
	info := Info{Alias: "box-dispatch-old", Username: "test@example.com", Status: "Deleted", ExpirationDate: "2026-08-05", DevHubID: "00Dhub"}
	failure := HealthFailure(info, time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC))
	if failure == nil {
		t.Fatal("deleted scratch org was accepted")
	}
	for _, want := range []string{"box-dispatch-old", "deleted", "Aug 5, 2026", "replacement"} {
		if !strings.Contains(failure.Summary, want) {
			t.Fatalf("summary %q does not contain %q", failure.Summary, want)
		}
	}
}

func TestHealthFailureRejectsExpiredActiveScratchOrg(t *testing.T) {
	info := Info{Alias: "box-dispatch-old", Username: "test@example.com", Status: "Active", ExpirationDate: "2026-08-05", DevHubID: "00Dhub"}
	if failure := HealthFailure(info, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)); failure == nil || !strings.Contains(failure.Summary, "expired") {
		t.Fatalf("failure = %#v, want expired-org failure", failure)
	}
}

func TestHealthFailureAcceptsHealthyScratchOrg(t *testing.T) {
	info := Info{Alias: "box-dispatch-new", Username: "test@example.com", Status: "Active", ExpirationDate: "2026-09-05", DevHubID: "00Dhub"}
	if failure := HealthFailure(info, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)); failure != nil {
		t.Fatalf("healthy org rejected: %v", failure)
	}
}

func TestParseDisplayToleratesCLIWarning(t *testing.T) {
	output := []byte("Warning: update available\n" + `{"result":{"id":"00D1","username":"test@example.com","alias":"scratch","status":"Active","expirationDate":"2026-09-05","devHubId":"00Dhub"}}`)
	info, err := ParseDisplay(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "00D1" || !info.IsScratch() || info.EffectiveStatus() != "Active" {
		t.Fatalf("unexpected info: %#v", info)
	}
}

func TestParseDevHubsPrefersConnectedAndDeduplicates(t *testing.T) {
	output := []byte("Warning: update available\n" + `{"result":{"devHubs":[{"alias":"unknown","username":"unknown@example.com","orgId":"00D0"},{"alias":"devhub","username":"hub@example.com","orgId":"00D1","connectedStatus":"Connected"},{"alias":"devhub","username":"duplicate@example.com","orgId":"00D2","connectedStatus":"Connected"}]}}`)
	hubs, err := ParseDevHubs(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(hubs) != 2 || hubs[0].Alias != "devhub" || !hubs[0].Connected() || hubs[1].Alias != "unknown" {
		t.Fatalf("unexpected Dev Hub inventory: %#v", hubs)
	}
}

func TestScratchCreateArgsTargetsSelectedDevHub(t *testing.T) {
	args := strings.Join(scratchCreateArgs("box-dispatch-test", "devhub"), " ")
	for _, want := range []string{"--target-dev-hub devhub", "--alias box-dispatch-test", "--duration-days 30", "--set-default"} {
		if !strings.Contains(args, want) {
			t.Fatalf("scratch args %q do not contain %q", args, want)
		}
	}
}

func TestCreateScratchRequiresExplicitDevHub(t *testing.T) {
	_, err := CreateScratch("box-dispatch-test", "")
	if err == nil || !strings.Contains(err.Error(), "Choose an authenticated Salesforce Dev Hub") {
		t.Fatalf("error = %v, want explicit Dev Hub requirement", err)
	}
}

func TestCLIErrorDetailsKeepsStackAndRedactsTokens(t *testing.T) {
	output := []byte(`{"name":"MetadataCommandError","message":"Metadata response failed","actions":["Check the org"],"stack":"full stack trace","data":{"accessToken":"secret","nested":{"refreshToken":"also-secret"}}}`)
	summary, diagnostic := CLIErrorDetails(output, errors.New("exit 1"))
	if strings.Contains(summary, "full stack") || !strings.Contains(summary, "Check the org") {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if !strings.Contains(diagnostic, "full stack trace") || !strings.Contains(diagnostic, "[REDACTED]") {
		t.Fatalf("diagnostic lost stack or redaction: %q", diagnostic)
	}
	if strings.Contains(diagnostic, "also-secret") || strings.Contains(diagnostic, `"secret"`) {
		t.Fatalf("diagnostic exposed token: %q", diagnostic)
	}
}

func TestCLIErrorDetailsExplainsHTTP420(t *testing.T) {
	output := []byte(`{"name":"ERROR_HTTP_420","message":"HTTP response contains html content.","stack":"full stack"}`)
	summary, diagnostic := CLIErrorDetails(output, errors.New("exit 1"))
	for _, want := range []string{"HTTP 420", "expired", "Connect > Salesforce"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}
	if !strings.Contains(diagnostic, "full stack") {
		t.Fatalf("diagnostic omitted stack: %q", diagnostic)
	}
}

func TestCLIErrorDetailsSurfacesMetadataComponentFailure(t *testing.T) {
	output := []byte(`{
  "name":"MetadataApiError",
  "message":"Failed to deploy metadata",
  "result":{
    "details":{"componentFailures":[
      {"componentType":"ApexClass","fullName":"DependentClass","problem":"Dependent class is invalid and needs recompilation"},
      {"componentType":"ApexClass","fullName":"BoxEmailAttachmentUploader","problem":"Invalid type: box.Toolkit"}
    ]},
    "files":[{"filePath":"force-app/vitest.setup.ts","state":"Unchanged"}]
  }
}`)
	summary, diagnostic := CLIErrorDetails(output, errors.New("exit 1"))
	for _, want := range []string{"ApexClass BoxEmailAttachmentUploader", "Invalid type: box.Toolkit", "Box for Salesforce", "additional component failure"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}
	if strings.Contains(summary, "vitest.setup.ts") || !strings.Contains(diagnostic, "vitest.setup.ts") {
		t.Fatalf("summary/diagnostic did not separate useful error from raw payload: %q / %q", summary, diagnostic)
	}
}

func TestCLIErrorDetailsAcceptsSingleComponentFailureObject(t *testing.T) {
	output := []byte(`{"result":{"details":{"componentFailures":{"componentType":"ApexClass","fullName":"Demo","problem":"Compile error"}}}}`)
	summary, _ := CLIErrorDetails(output, errors.New("exit 1"))
	if !strings.Contains(summary, "ApexClass Demo: Compile error") {
		t.Fatalf("unexpected summary: %q", summary)
	}
}

func TestDiagnosticRedactsMalformedTextOutput(t *testing.T) {
	diagnostic := Diagnostic([]byte("access_token=secret-token Authorization: Bearer abc.def.ghi"), errors.New("exit 1"))
	if strings.Contains(diagnostic, "secret-token") || strings.Contains(diagnostic, "abc.def.ghi") {
		t.Fatalf("text diagnostic exposed credentials: %q", diagnostic)
	}
	if strings.Count(diagnostic, "[REDACTED]") != 2 {
		t.Fatalf("text diagnostic did not redact both credentials: %q", diagnostic)
	}
}
