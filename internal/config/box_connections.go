package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BoxAppConnection is one locally stored Box login. Secrets stay in the
// owner-only settings file and are never returned to the browser.
type BoxAppConnection struct {
	ID           string `json:"id,omitempty"`
	Alias        string `json:"alias,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	SubjectType  string `json:"subjectType,omitempty"`
	SubjectID    string `json:"subjectId,omitempty"`
	VerifiedAt   string `json:"verifiedAt,omitempty"`
	Identity     string `json:"identity,omitempty"`
	Account      string `json:"account,omitempty"`
	Enterprise   string `json:"enterprise,omitempty"`
}

func (c ConnectionSettings) HydrateBoxConnections() ConnectionSettings {
	if len(c.BoxConnections) > 0 {
		if strings.TrimSpace(c.BoxSelectedConnectionID) == "" {
			c.BoxSelectedConnectionID = c.BoxConnections[0].ID
		}
		return c.projectBoxSelection()
	}
	if c.BoxCCGClientID == "" || c.BoxCCGClientSecret == "" || c.BoxCCGSubjectType == "" || c.BoxCCGSubjectID == "" {
		return c
	}
	alias := strings.TrimSpace(c.BoxCCGAlias)
	if alias == "" {
		alias = "Box CCG"
	}
	app := BoxAppConnection{
		ID: "legacy-box", Alias: alias, ClientID: c.BoxCCGClientID, ClientSecret: c.BoxCCGClientSecret,
		SubjectType: c.BoxCCGSubjectType, SubjectID: c.BoxCCGSubjectID,
	}
	if snapshot := c.VerifiedConnections["box"]; snapshot.VerifiedAt != "" {
		app.VerifiedAt = snapshot.VerifiedAt
		app.Identity = snapshot.Identity
		app.Account = snapshot.Account
		app.Enterprise = snapshot.Enterprise
	}
	c.BoxConnections = []BoxAppConnection{app}
	c.BoxSelectedConnectionID = app.ID
	return c.projectBoxSelection()
}

func (c ConnectionSettings) UpsertBoxConnection(app BoxAppConnection, selectApp bool) ConnectionSettings {
	c = c.HydrateBoxConnections()
	app.Alias = strings.TrimSpace(app.Alias)
	app.ClientID = strings.TrimSpace(app.ClientID)
	app.ClientSecret = strings.TrimSpace(app.ClientSecret)
	app.RefreshToken = strings.TrimSpace(app.RefreshToken)
	app.AccessToken = strings.TrimSpace(app.AccessToken)
	app.SubjectType = strings.ToLower(strings.TrimSpace(app.SubjectType))
	app.SubjectID = strings.TrimSpace(app.SubjectID)
	app.Identity = strings.TrimSpace(app.Identity)
	app.Account = strings.TrimSpace(app.Account)
	if app.Alias == "" {
		app.Alias = firstOrgValue(app.Identity, "Box")
	}
	replaced := false
	for i, existing := range c.BoxConnections {
		if sameBoxConnection(existing, app) {
			app = mergeBoxConnection(existing, app)
			c.BoxConnections[i] = app
			replaced = true
			break
		}
	}
	if !replaced {
		if app.ID == "" {
			app.ID = newBoxConnectionID()
		}
		c.BoxConnections = append(c.BoxConnections, app)
	}
	if selectApp || c.BoxSelectedConnectionID == "" {
		c.BoxSelectedConnectionID = app.ID
	}
	return c.projectBoxSelection()
}

func (c ConnectionSettings) SelectedBoxConnection() (BoxAppConnection, bool) {
	c = c.HydrateBoxConnections()
	for _, app := range c.BoxConnections {
		if app.ID == c.BoxSelectedConnectionID {
			return app, true
		}
	}
	if len(c.BoxConnections) > 0 {
		return c.BoxConnections[0], true
	}
	return BoxAppConnection{}, false
}

func (c ConnectionSettings) RemoveBoxConnection(id string) (ConnectionSettings, error) {
	c = c.HydrateBoxConnections()
	id = strings.TrimSpace(id)
	if id == "" {
		return c, fmt.Errorf("select a connected Box app to remove")
	}
	kept := make([]BoxAppConnection, 0, len(c.BoxConnections))
	removed := false
	for _, app := range c.BoxConnections {
		if app.ID == id {
			removed = true
			continue
		}
		kept = append(kept, app)
	}
	if !removed {
		return c, fmt.Errorf("select a connected Box app to remove")
	}
	c.BoxConnections = kept
	if len(c.BoxConnections) == 0 {
		return c.clearBoxSelection(), nil
	}
	if c.BoxSelectedConnectionID == id {
		c.BoxSelectedConnectionID = c.BoxConnections[0].ID
	}
	if selected, ok := c.SelectedBoxConnection(); ok {
		return c.syncBoxVerification(selected), nil
	}
	return c.projectBoxSelection(), nil
}

func (c ConnectionSettings) clearBoxSelection() ConnectionSettings {
	c.BoxConnections = nil
	c.BoxSelectedConnectionID = ""
	c.BoxCCGAlias = ""
	c.BoxCCGClientID = ""
	c.BoxCCGClientSecret = ""
	c.BoxCCGSubjectType = ""
	c.BoxCCGSubjectID = ""
	c.BoxDefaultConnection = ""
	if c.VerifiedConnections != nil {
		delete(c.VerifiedConnections, "box")
	}
	return c
}

func (c ConnectionSettings) SelectBoxConnection(id string) (ConnectionSettings, error) {
	c = c.HydrateBoxConnections()
	id = strings.TrimSpace(id)
	for _, app := range c.BoxConnections {
		if app.ID == id || strings.EqualFold(app.Alias, id) {
			c.BoxSelectedConnectionID = app.ID
			return c.syncBoxVerification(app), nil
		}
	}
	return c, fmt.Errorf("select a connected Box app")
}

func (c ConnectionSettings) MarkBoxVerified(verification VerifiedConnection) ConnectionSettings {
	c = c.HydrateBoxConnections()
	for i, app := range c.BoxConnections {
		if app.ID != c.BoxSelectedConnectionID {
			continue
		}
		app.VerifiedAt = verification.VerifiedAt
		app.Identity = verification.Identity
		app.Account = verification.Account
		app.Enterprise = verification.Enterprise
		if verification.RefreshToken != "" {
			app.RefreshToken = verification.RefreshToken
		}
		c.BoxConnections[i] = app
		return c.syncBoxVerification(app)
	}
	return c
}

// MarkSelectedBoxUnverified retains the selected connection for reconnecting
// while removing the stale readiness snapshot that would otherwise let a
// failed OAuth refresh appear active in Dispatch.
func (c ConnectionSettings) MarkSelectedBoxUnverified() ConnectionSettings {
	c = c.HydrateBoxConnections()
	for i, app := range c.BoxConnections {
		if app.ID != c.BoxSelectedConnectionID {
			continue
		}
		app.VerifiedAt = ""
		c.BoxConnections[i] = app
		return c.syncBoxVerification(app)
	}
	return c
}

func (c ConnectionSettings) projectBoxSelection() ConnectionSettings {
	for _, app := range c.BoxConnections {
		if app.ID != c.BoxSelectedConnectionID {
			continue
		}
		c.BoxCCGAlias = app.Alias
		c.BoxCCGClientID = app.ClientID
		c.BoxCCGClientSecret = app.ClientSecret
		c.BoxCCGSubjectType = app.SubjectType
		c.BoxCCGSubjectID = app.SubjectID
		break
	}
	return c
}

func (c ConnectionSettings) syncBoxVerification(app BoxAppConnection) ConnectionSettings {
	c = c.projectBoxSelection()
	if app.VerifiedAt == "" {
		if c.VerifiedConnections != nil {
			delete(c.VerifiedConnections, "box")
		}
		return c
	}
	if c.VerifiedConnections == nil {
		c.VerifiedConnections = map[string]VerifiedConnection{}
	}
	authType := "CCG"
	if app.RefreshToken != "" {
		authType = "OAuth2"
	}
	c.VerifiedConnections["box"] = VerifiedConnection{
		VerifiedAt: app.VerifiedAt, Selection: app.Alias, Identity: app.Identity,
		Account: app.Account, Enterprise: app.Enterprise, AuthType: authType,
	}
	return c
}

func sameBoxConnection(existing, next BoxAppConnection) bool {
	if existing.ID != "" && next.ID != "" {
		return existing.ID == next.ID
	}
	if existing.Account != "" && next.Account != "" {
		return existing.Account == next.Account
	}
	if existing.Identity != "" && next.Identity != "" {
		return strings.EqualFold(existing.Identity, next.Identity)
	}
	return existing.ClientID != "" && existing.ClientID == next.ClientID &&
		existing.SubjectType == next.SubjectType && existing.SubjectID == next.SubjectID &&
		strings.EqualFold(existing.Alias, next.Alias)
}

func mergeBoxConnection(existing, next BoxAppConnection) BoxAppConnection {
	next.ID = existing.ID
	next.Alias = firstOrgValue(next.Alias, existing.Alias)
	if next.ClientSecret == "" {
		next.ClientSecret = existing.ClientSecret
	}
	if next.RefreshToken == "" {
		next.RefreshToken = existing.RefreshToken
	}
	if next.AccessToken == "" {
		next.AccessToken = existing.AccessToken
	}
	if next.VerifiedAt == "" {
		next.VerifiedAt = existing.VerifiedAt
		next.Identity = existing.Identity
		next.Account = existing.Account
		next.Enterprise = existing.Enterprise
	}
	return next
}

func newBoxConnectionID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("box-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
}
