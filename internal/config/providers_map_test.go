package config

import "testing"

func TestProviderIDMappingRoundTrips(t *testing.T) {
	for internalKey, bclID := range map[string]string{
		"box":        "box",
		"salesforce": "salesforce-agentforce",
		"databricks": "databricks",
		"aws":        "bedrock-agentcore",
	} {
		if got := BCLProviderID(internalKey); got != bclID {
			t.Fatalf("BCLProviderID(%q) = %q, want %q", internalKey, got, bclID)
		}
		if got := InternalProviderKey(bclID); got != internalKey {
			t.Fatalf("InternalProviderKey(%q) = %q, want %q", bclID, got, internalKey)
		}
	}
}

func TestProviderIDMappingPassesThroughUnknown(t *testing.T) {
	if got := InternalProviderKey("unknown"); got != "unknown" {
		t.Fatalf("InternalProviderKey(unknown) = %q, want passthrough", got)
	}
	if got := BCLProviderID("unknown"); got != "unknown" {
		t.Fatalf("BCLProviderID(unknown) = %q, want passthrough", got)
	}
	// An internal key handed to InternalProviderKey stays itself.
	if got := InternalProviderKey("box"); got != "box" {
		t.Fatalf("InternalProviderKey(box) = %q, want box", got)
	}
}
