package config

// The runtime domain (checker, lifecycle, launch shell) keys providers as
// box/salesforce/databricks/aws. The BCL configuration keys the same providers
// as box/salesforce-agentforce/databricks/bedrock-agentcore. These helpers
// translate between the two namespaces so neither side has to adopt the other's
// identifiers.

var bclProviderIDByInternalKey = map[string]string{
	"box":        "box",
	"salesforce": "salesforce-agentforce",
	"databricks": "databricks",
	"aws":        "bedrock-agentcore",
}

var internalKeyByBCLProviderID = func() map[string]string {
	out := make(map[string]string, len(bclProviderIDByInternalKey))
	for internalKey, bclID := range bclProviderIDByInternalKey {
		out[bclID] = internalKey
	}
	return out
}()

// InternalProviderKey maps a BCL provider ID to the runtime domain key,
// returning the input unchanged when it is already an internal key or unknown.
func InternalProviderKey(bclID string) string {
	if key, ok := internalKeyByBCLProviderID[bclID]; ok {
		return key
	}
	return bclID
}

// BCLProviderID maps a runtime domain key to its BCL provider ID, returning the
// input unchanged when it is already a BCL ID or unknown.
func BCLProviderID(internalKey string) string {
	if id, ok := bclProviderIDByInternalKey[internalKey]; ok {
		return id
	}
	return internalKey
}
