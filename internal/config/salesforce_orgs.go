package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// SalesforceOrgConnection is one locally stored Salesforce org. Tokens stay in
// the owner-only settings file and are never returned to the browser.
type SalesforceOrgConnection struct {
	ID             string `json:"id,omitempty"`
	Alias          string `json:"alias,omitempty"`
	Username       string `json:"username,omitempty"`
	OrgID          string `json:"orgId,omitempty"`
	OrgType        string `json:"orgType,omitempty"`
	InstanceURL    string `json:"instanceUrl,omitempty"`
	AccessToken    string `json:"accessToken,omitempty"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	ClientSecret   string `json:"clientSecret,omitempty"`
	Status         string `json:"status,omitempty"`
	ExpirationDate string `json:"expirationDate,omitempty"`
	DevHub         bool   `json:"devHub,omitempty"`
}

func (c ConnectionSettings) HasSalesforceDevHub() bool {
	hydrated := c.HydrateSalesforceOrgs()
	if _, ok := hydrated.DevHubOrg(); ok {
		return true
	}
	return c.SalesforceDevHubURL != "" && c.SalesforceDevHubToken != ""
}

func (c ConnectionSettings) HydrateSalesforceOrgs() ConnectionSettings {
	if len(c.SalesforceOrgs) > 0 {
		if strings.TrimSpace(c.SalesforceSelectedOrgID) == "" {
			c.SalesforceSelectedOrgID = c.SalesforceOrgs[0].ID
		}
		return c.projectSalesforceSelection()
	}
	if c.SalesforceInstanceURL != "" && c.SalesforceAccessToken != "" {
		alias := strings.TrimSpace(c.SalesforceAlias)
		if alias == "" {
			alias = "Connected Salesforce org"
		}
		sameHub := c.SalesforceDevHubURL == "" || c.SalesforceDevHubURL == c.SalesforceInstanceURL
		org := SalesforceOrgConnection{
			ID: "legacy-org", Alias: alias, OrgID: c.SalesforceOrgID, OrgType: firstOrgValue(c.SalesforceOrgType, "persistent"),
			InstanceURL: c.SalesforceInstanceURL, AccessToken: c.SalesforceAccessToken, RefreshToken: c.SalesforceRefreshToken,
			Status: c.SalesforceOrgStatus, ExpirationDate: c.SalesforceExpirationDate, DevHub: sameHub && !strings.EqualFold(c.SalesforceOrgType, "scratch"),
		}
		c.SalesforceOrgs = append(c.SalesforceOrgs, org)
		c.SalesforceSelectedOrgID = org.ID
		if org.DevHub {
			c.SalesforceDevHubOrgID = org.ID
		}
	}
	if c.SalesforceDevHubURL != "" && c.SalesforceDevHubToken != "" && c.SalesforceDevHubURL != c.SalesforceInstanceURL {
		alias := strings.TrimSpace(c.SalesforceDevHubAlias)
		if alias == "" {
			alias = "Connected Salesforce Dev Hub"
		}
		hub := SalesforceOrgConnection{
			ID: "legacy-devhub", Alias: alias, OrgType: "persistent", InstanceURL: c.SalesforceDevHubURL,
			AccessToken: c.SalesforceDevHubToken, RefreshToken: c.SalesforceDevHubRefreshToken, Status: "Active", DevHub: true,
		}
		c.SalesforceOrgs = append(c.SalesforceOrgs, hub)
		c.SalesforceDevHubOrgID = hub.ID
		if c.SalesforceSelectedOrgID == "" {
			c.SalesforceSelectedOrgID = hub.ID
		}
	}
	return c.projectSalesforceSelection()
}

func (c ConnectionSettings) UpsertSalesforceOrg(org SalesforceOrgConnection, selectOrg bool) ConnectionSettings {
	c = c.HydrateSalesforceOrgs()
	org.InstanceURL = strings.TrimRight(strings.TrimSpace(org.InstanceURL), "/")
	org.Alias = strings.TrimSpace(org.Alias)
	org.Username = strings.TrimSpace(org.Username)
	org.OrgID = strings.TrimSpace(org.OrgID)
	org.AccessToken = strings.TrimSpace(org.AccessToken)
	org.RefreshToken = strings.TrimSpace(org.RefreshToken)
	org.ClientID = strings.TrimSpace(org.ClientID)
	org.ClientSecret = strings.TrimSpace(org.ClientSecret)
	if strings.EqualFold(org.OrgType, "scratch") {
		org.DevHub = false
	}
	replaced := false
	for i, existing := range c.SalesforceOrgs {
		if sameSalesforceOrg(existing, org) {
			org = mergeSalesforceOrg(existing, org)
			c.SalesforceOrgs[i] = org
			replaced = true
			break
		}
	}
	if !replaced {
		if org.ID == "" {
			org.ID = newSalesforceOrgID()
		}
		c.SalesforceOrgs = append(c.SalesforceOrgs, org)
	}
	if selectOrg || c.SalesforceSelectedOrgID == "" {
		c.SalesforceSelectedOrgID = org.ID
	}
	if org.DevHub {
		c.SalesforceDevHubOrgID = org.ID
		c.markDevHub(org.ID)
	}
	return c.projectSalesforceSelection()
}

func (c ConnectionSettings) RemoveSalesforceOrg(id string) (ConnectionSettings, error) {
	c = c.HydrateSalesforceOrgs()
	id = strings.TrimSpace(id)
	if id == "" {
		return c, fmt.Errorf("select a connected Salesforce org to remove")
	}
	kept := make([]SalesforceOrgConnection, 0, len(c.SalesforceOrgs))
	removed := false
	for _, org := range c.SalesforceOrgs {
		if org.ID == id {
			removed = true
			continue
		}
		kept = append(kept, org)
	}
	if !removed {
		return c, fmt.Errorf("select a connected Salesforce org to remove")
	}
	c.SalesforceOrgs = kept
	if c.SalesforceDevHubOrgID == id {
		c.SalesforceDevHubOrgID = ""
	}
	if len(c.SalesforceOrgs) == 0 {
		return c.clearSalesforceSelection(), nil
	}
	if c.SalesforceSelectedOrgID == id {
		c.SalesforceSelectedOrgID = c.SalesforceOrgs[0].ID
	}
	if c.SalesforceDevHubOrgID == "" {
		if hub, ok := c.DevHubOrg(); ok {
			c.SalesforceDevHubOrgID = hub.ID
			c.markDevHub(hub.ID)
		}
	}
	c = c.projectSalesforceSelection()
	if c.SalesforceDevHubOrgID == "" {
		c.SalesforceDevHubAlias = ""
		c.SalesforceDevHubURL = ""
		c.SalesforceDevHubToken = ""
		c.SalesforceDevHubRefreshToken = ""
	}
	if c.VerifiedConnections != nil {
		delete(c.VerifiedConnections, "salesforce")
	}
	return c, nil
}

func (c ConnectionSettings) clearSalesforceSelection() ConnectionSettings {
	c.SalesforceOrgs = nil
	c.SalesforceSelectedOrgID = ""
	c.SalesforceDevHubOrgID = ""
	c.SalesforceAlias = ""
	c.SalesforceOrgID = ""
	c.SalesforceOrgType = ""
	c.SalesforceOrgStatus = ""
	c.SalesforceExpirationDate = ""
	c.SalesforceInstanceURL = ""
	c.SalesforceAccessToken = ""
	c.SalesforceRefreshToken = ""
	c.SalesforceDevHubAlias = ""
	c.SalesforceDevHubURL = ""
	c.SalesforceDevHubToken = ""
	c.SalesforceDevHubRefreshToken = ""
	if c.VerifiedConnections != nil {
		delete(c.VerifiedConnections, "salesforce")
	}
	return c
}

func (c ConnectionSettings) SelectSalesforceOrg(id string) (ConnectionSettings, error) {
	c = c.HydrateSalesforceOrgs()
	id = strings.TrimSpace(id)
	for _, org := range c.SalesforceOrgs {
		if org.ID == id || strings.EqualFold(org.Alias, id) {
			c.SalesforceSelectedOrgID = org.ID
			return c.projectSalesforceSelection(), nil
		}
	}
	return c, fmt.Errorf("select a connected Salesforce org")
}

// SelectedSalesforceOrg returns the selected org as one credential record. A
// caller must not combine its client ID with a secret from another record.
func (c ConnectionSettings) SelectedSalesforceOrg() (SalesforceOrgConnection, bool) {
	c = c.HydrateSalesforceOrgs()
	for _, org := range c.SalesforceOrgs {
		if org.ID == c.SalesforceSelectedOrgID {
			return org, true
		}
	}
	return SalesforceOrgConnection{}, false
}

// InvalidateSelectedSalesforceVerification clears the readiness snapshot after
// the selected org rejects authentication. The connection remains available so
// the user can refresh or reconnect it without losing its local identity.
func (c ConnectionSettings) InvalidateSelectedSalesforceVerification(status string) ConnectionSettings {
	c = c.HydrateSalesforceOrgs()
	c.SalesforceOrgStatus = strings.TrimSpace(status)
	c = c.SyncSelectedSalesforceOrg()
	if c.VerifiedConnections != nil {
		delete(c.VerifiedConnections, "salesforce")
	}
	return c
}

func (c ConnectionSettings) SyncSelectedSalesforceOrg() ConnectionSettings {
	// Callers use this after updating the flattened selected-org fields. Do not
	// re-project an existing row first or it would overwrite those fresh values
	// with the stale values from the row being synchronized.
	if len(c.SalesforceOrgs) == 0 {
		c = c.HydrateSalesforceOrgs()
	}
	for i, org := range c.SalesforceOrgs {
		if org.ID != c.SalesforceSelectedOrgID {
			continue
		}
		org.Alias = firstOrgValue(c.SalesforceAlias, org.Alias)
		org.OrgID = firstOrgValue(c.SalesforceOrgID, org.OrgID)
		org.OrgType = firstOrgValue(c.SalesforceOrgType, org.OrgType)
		org.Status = firstOrgValue(c.SalesforceOrgStatus, org.Status)
		org.ExpirationDate = firstOrgValue(c.SalesforceExpirationDate, org.ExpirationDate)
		org.InstanceURL = firstOrgValue(c.SalesforceInstanceURL, org.InstanceURL)
		org.AccessToken = firstOrgValue(c.SalesforceAccessToken, org.AccessToken)
		org.RefreshToken = firstOrgValue(c.SalesforceRefreshToken, org.RefreshToken)
		c.SalesforceOrgs[i] = org
		break
	}
	return c.projectSalesforceSelection()
}

func (c ConnectionSettings) MarkSelectedAsDevHub() ConnectionSettings {
	c = c.HydrateSalesforceOrgs()
	for _, org := range c.SalesforceOrgs {
		if org.ID != c.SalesforceSelectedOrgID || strings.EqualFold(org.OrgType, "scratch") {
			continue
		}
		c.SalesforceDevHubOrgID = org.ID
		c.markDevHub(org.ID)
		return c.projectSalesforceSelection()
	}
	return c
}

func (c ConnectionSettings) DevHubOrg() (SalesforceOrgConnection, bool) {
	if id := strings.TrimSpace(c.SalesforceDevHubOrgID); id != "" {
		for _, org := range c.SalesforceOrgs {
			if org.ID == id && org.AccessToken != "" && org.InstanceURL != "" {
				return org, true
			}
		}
	}
	for _, org := range c.SalesforceOrgs {
		if org.DevHub && org.AccessToken != "" && org.InstanceURL != "" {
			return org, true
		}
	}
	return SalesforceOrgConnection{}, false
}

func (c ConnectionSettings) projectSalesforceSelection() ConnectionSettings {
	for _, org := range c.SalesforceOrgs {
		if org.ID != c.SalesforceSelectedOrgID {
			continue
		}
		c.SalesforceAlias = org.Alias
		c.SalesforceOrgID = org.OrgID
		c.SalesforceOrgType = org.OrgType
		c.SalesforceOrgStatus = org.Status
		c.SalesforceExpirationDate = org.ExpirationDate
		c.SalesforceInstanceURL = org.InstanceURL
		c.SalesforceAccessToken = org.AccessToken
		c.SalesforceRefreshToken = org.RefreshToken
		break
	}
	if hub, ok := c.DevHubOrg(); ok {
		c.SalesforceDevHubAlias = hub.Alias
		c.SalesforceDevHubURL = hub.InstanceURL
		c.SalesforceDevHubToken = hub.AccessToken
		c.SalesforceDevHubRefreshToken = hub.RefreshToken
	}
	return c
}

func (c *ConnectionSettings) markDevHub(id string) {
	for i, org := range c.SalesforceOrgs {
		org.DevHub = org.ID == id && !strings.EqualFold(org.OrgType, "scratch")
		c.SalesforceOrgs[i] = org
	}
}

func sameSalesforceOrg(existing, next SalesforceOrgConnection) bool {
	if existing.ID != "" && next.ID != "" && existing.ID == next.ID {
		return true
	}
	if existing.OrgID != "" && next.OrgID != "" && existing.OrgID == next.OrgID {
		return true
	}
	// Incomplete rows have no org ID yet. Treat the same instance URL as one unconfirmed org.
	return existing.OrgID == "" && next.OrgID == "" &&
		existing.InstanceURL != "" && existing.InstanceURL == next.InstanceURL
}

func mergeSalesforceOrg(existing, next SalesforceOrgConnection) SalesforceOrgConnection {
	next.ID = existing.ID
	next.Alias = firstOrgValue(next.Alias, existing.Alias)
	next.Username = firstOrgValue(next.Username, existing.Username)
	next.OrgID = firstOrgValue(next.OrgID, existing.OrgID)
	next.OrgType = firstOrgValue(next.OrgType, existing.OrgType)
	next.Status = firstOrgValue(next.Status, existing.Status)
	next.ExpirationDate = firstOrgValue(next.ExpirationDate, existing.ExpirationDate)
	if next.RefreshToken == "" && (next.AccessToken == "" || next.AccessToken == existing.AccessToken) {
		next.RefreshToken = existing.RefreshToken
	}
	nextClientID := strings.TrimSpace(next.ClientID)
	existingClientID := strings.TrimSpace(existing.ClientID)
	if nextClientID == "" {
		next.ClientID = existingClientID
	}
	// Client IDs and secrets are an atomic pair. Preserve an omitted secret only
	// when the client ID is unchanged; carrying it across an OAuth client change
	// produces an invalid client_id/client_secret combination at refresh time.
	if strings.TrimSpace(next.ClientSecret) == "" && (nextClientID == "" || nextClientID == existingClientID) {
		next.ClientSecret = existing.ClientSecret
	}
	if existing.DevHub && !strings.EqualFold(next.OrgType, "scratch") {
		next.DevHub = true
	}
	return next
}

func newSalesforceOrgID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("org-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}

func firstOrgValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
