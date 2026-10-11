package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
)

// DeploymentTargetFingerprint excludes credentials and display/readiness fields.
// It identifies the actual selected accounts rather than a token's lifetime.
func DeploymentTargetFingerprint(settings ConnectionSettings) string {
	box, _ := settings.SelectedBoxConnection()
	sf, _ := settings.SelectedSalesforceOrg()
	if strings.TrimSpace(box.ClientID) == "" && box.RefreshToken != "" {
		box.ClientID = strings.TrimSpace(os.Getenv("BOX_CLIENT_ID"))
	}
	if strings.TrimSpace(sf.ClientID) == "" {
		sf.ClientID = strings.TrimSpace(settings.SalesforceClientID)
	}
	value := struct {
		Box        [8]string
		Salesforce [5]string
	}{
		Box:        [8]string{box.ID, box.ClientID, box.SubjectType, box.SubjectID, box.Identity, box.Account, box.Enterprise, normalizeTargetURL(box.Hostname)},
		Salesforce: [5]string{sf.ID, sf.ClientID, sf.Username, sf.OrgID, normalizeTargetURL(sf.InstanceURL)},
	}
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeTargetURL(value string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "/"))
}

// CloneConnectionSettings keeps read/modify/save callers from mutating a store
// through a shared slice or verification map before its write guard runs.
func CloneConnectionSettings(settings ConnectionSettings) ConnectionSettings {
	settings.BoxConnections = append([]BoxAppConnection(nil), settings.BoxConnections...)
	settings.SalesforceOrgs = append([]SalesforceOrgConnection(nil), settings.SalesforceOrgs...)
	if settings.VerifiedConnections != nil {
		snapshots := make(map[string]VerifiedConnection, len(settings.VerifiedConnections))
		for key, value := range settings.VerifiedConnections {
			value.Options = append([]string(nil), value.Options...)
			snapshots[key] = value
		}
		settings.VerifiedConnections = snapshots
	}
	return settings
}
