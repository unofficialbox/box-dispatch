package config

// ConnectionSettings is the provider selection persisted by the launch shell.
// Persistence lives in internal/shellstate (BCL format); this package only owns
// the type so internal/lifecycle and the shell can share it without depending on
// the BCL writer.
//
// BoxCCGClientSecret is a credential. The whole record is written 0600, but it
// is on-disk plaintext, so the store is only as private as the file.
type ConnectionSettings struct {
	SalesforceAlias   string `json:"salesforceAlias,omitempty"`
	DatabricksHost    string `json:"databricksHost,omitempty"`
	DatabricksProfile string `json:"databricksProfile,omitempty"`
	AWSProfile        string `json:"awsProfile,omitempty"`
	AWSRegion         string `json:"awsRegion,omitempty"`

	// Box Client Credentials Grant app. The subject determines who the token acts
	// as: box_subject_type "user" keeps created resources owned by that user,
	// "enterprise" acts as the service account. Either way the token carries the
	// enterprise app's scopes (e.g. Doc Gen) the CLI's OAuth token lacks.
	BoxCCGClientID     string `json:"boxCcgClientId,omitempty"`
	BoxCCGClientSecret string `json:"boxCcgClientSecret,omitempty"`
	BoxCCGSubjectType  string `json:"boxCcgSubjectType,omitempty"` // "user" or "enterprise"
	BoxCCGSubjectID    string `json:"boxCcgSubjectId,omitempty"`

	// BoxDefaultConnection pins which Box connection box-dispatch uses: a CLI
	// environment name, or the box-dispatch CCG sentinel. Empty means fall back
	// to precedence (CCG when configured, else the CLI's current environment).
	BoxDefaultConnection string `json:"boxDefaultConnection,omitempty"`
}

// HasBoxCCG reports whether a complete CCG credential set has been captured.
func (c ConnectionSettings) HasBoxCCG() bool {
	return c.BoxCCGClientID != "" && c.BoxCCGClientSecret != "" &&
		c.BoxCCGSubjectType != "" && c.BoxCCGSubjectID != ""
}
