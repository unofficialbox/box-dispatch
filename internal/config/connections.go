package config

// ConnectionSettings is the non-secret provider selection persisted by the
// launch shell. Persistence lives in internal/shellstate (BCL format); this
// package only owns the type so internal/lifecycle and the shell can share it
// without depending on the BCL writer.
type ConnectionSettings struct {
	SalesforceAlias   string `json:"salesforceAlias,omitempty"`
	DatabricksHost    string `json:"databricksHost,omitempty"`
	DatabricksProfile string `json:"databricksProfile,omitempty"`
	AWSProfile        string `json:"awsProfile,omitempty"`
	AWSRegion         string `json:"awsRegion,omitempty"`
}
