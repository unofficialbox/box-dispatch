package config

// ConnectionSettings is the provider selection persisted by the launch shell.
// Persistence lives in internal/shellstate (BCL format); this package only owns
// the type so internal/lifecycle and the shell can share it without depending on
// the BCL writer.
//
// BoxCCGClientSecret is a credential. The whole record is written 0600, but it
// is on-disk plaintext, so the store is only as private as the file.
type ConnectionSettings struct {
	SalesforceAlias          string `json:"salesforceAlias,omitempty"`
	SalesforceOrgID          string `json:"salesforceOrgId,omitempty"`
	SalesforceOrgType        string `json:"salesforceOrgType,omitempty"`
	SalesforceOrgStatus      string `json:"salesforceOrgStatus,omitempty"`
	SalesforceExpirationDate string `json:"salesforceExpirationDate,omitempty"`
	SalesforceDevHubAlias    string `json:"salesforceDevHubAlias,omitempty"`
	DatabricksHost           string `json:"databricksHost,omitempty"`
	DatabricksProfile        string `json:"databricksProfile,omitempty"`
	AWSProfile               string `json:"awsProfile,omitempty"`
	AWSRegion                string `json:"awsRegion,omitempty"`

	// Box Client Credentials Grant app. The subject determines who the token acts
	// as: box_subject_type "user" keeps created resources owned by that user,
	// "enterprise" acts as the service account. Either way the token carries the
	// enterprise app's scopes (e.g. Doc Gen) the CLI's OAuth token lacks.
	BoxCCGClientID     string `json:"boxCcgClientId,omitempty"`
	BoxCCGClientSecret string `json:"boxCcgClientSecret,omitempty"`
	BoxCCGSubjectType  string `json:"boxCcgSubjectType,omitempty"` // "user" or "enterprise"
	BoxCCGSubjectID    string `json:"boxCcgSubjectId,omitempty"`

	// BoxDefaultConnection records the selected CCG app. Older installations may
	// contain a Box CLI environment name; Dispatch surfaces a migration message
	// instead of attempting to use that unsupported path.
	BoxDefaultConnection string `json:"boxDefaultConnection,omitempty"`

	// VerifiedConnections stores credential-free identity details from the last
	// successful provider check. The shell restores these snapshots on later
	// runs and invalidates them whenever the selected connection changes.
	VerifiedConnections map[string]VerifiedConnection `json:"verifiedConnections,omitempty"`
}

// VerifiedConnection is the non-secret result of a successful provider check.
// Selection identifies the configured alias/profile/environment that produced
// the result so manually edited settings cannot reuse a mismatched snapshot.
type VerifiedConnection struct {
	VerifiedAt string   `json:"verifiedAt"`
	Selection  string   `json:"selection,omitempty"`
	Identity   string   `json:"identity,omitempty"`
	Account    string   `json:"account,omitempty"`
	Enterprise string   `json:"enterprise,omitempty"`
	Profile    string   `json:"profile,omitempty"`
	Host       string   `json:"host,omitempty"`
	Region     string   `json:"region,omitempty"`
	Options    []string `json:"options,omitempty"`
	AuthType   string   `json:"authType,omitempty"`
	OrgID      string   `json:"orgId,omitempty"`
	OrgStatus  string   `json:"orgStatus,omitempty"`
	OrgType    string   `json:"orgType,omitempty"`
	ExpiresAt  string   `json:"expiresAt,omitempty"`
}

// HasBoxCCG reports whether a complete CCG credential set has been captured.
func (c ConnectionSettings) HasBoxCCG() bool {
	return c.BoxCCGClientID != "" && c.BoxCCGClientSecret != "" &&
		c.BoxCCGSubjectType != "" && c.BoxCCGSubjectID != ""
}
