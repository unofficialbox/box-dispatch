package webapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/salesforceorg"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

type connectionSaver func(config.ConnectionSettings) error
type salesforceTargetStore func() ([]salesforceorg.Target, error)

type salesforceConnectionOption struct {
	Alias     string `json:"alias"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Selected  bool   `json:"selected"`
}

type salesforceConnectionSelection struct {
	Alias string `json:"alias"`
}

// boxConnectionInput mirrors the supported Dispatch CCG setup without ever
// returning secret material to the browser.
type boxConnectionInput struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	SubjectType  string `json:"subjectType"`
	SubjectID    string `json:"subjectId"`
}

func (b boxConnectionInput) normalized() boxConnectionInput {
	b.ClientID = strings.TrimSpace(b.ClientID)
	b.ClientSecret = strings.TrimSpace(b.ClientSecret)
	b.SubjectType = strings.ToLower(strings.TrimSpace(b.SubjectType))
	b.SubjectID = strings.TrimSpace(b.SubjectID)
	if b.SubjectType == "" {
		b.SubjectType = "user"
	}
	return b
}

func (b boxConnectionInput) validate() error {
	if b.ClientID == "" || b.ClientSecret == "" || b.SubjectID == "" {
		return fmt.Errorf("client ID, client secret, and subject ID are required")
	}
	if b.SubjectType != "user" && b.SubjectType != "enterprise" {
		return fmt.Errorf("subject type must be user or enterprise")
	}
	return nil
}

func saveBoxCCGSelection(settings config.ConnectionSettings, input boxConnectionInput) config.ConnectionSettings {
	settings.BoxCCGClientID = input.ClientID
	settings.BoxCCGClientSecret = input.ClientSecret
	settings.BoxCCGSubjectType = input.SubjectType
	settings.BoxCCGSubjectID = input.SubjectID
	settings.BoxDefaultConnection = boxconn.DispatchCCGName
	if settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "box")
	}
	return settings
}

func loadPersistedConnections() (config.ConnectionSettings, error) {
	return shellstate.LoadConnectionSettings()
}

func savePersistedConnections(settings config.ConnectionSettings) error {
	return shellstate.SaveConnectionSettings(settings)
}

func listSalesforceTargets() ([]salesforceorg.Target, error) {
	return salesforceorg.ListTargets()
}

func presentSalesforceOptions(settings config.ConnectionSettings, targets []salesforceorg.Target) []salesforceConnectionOption {
	options := make([]salesforceConnectionOption, 0, len(targets))
	for _, target := range targets {
		if !target.Healthy(time.Now()) {
			continue
		}
		kind := "Org"
		if target.IsScratch() {
			kind = "Scratch org"
		}
		options = append(options, salesforceConnectionOption{
			Alias: target.Alias, Kind: kind, Status: target.Status,
			ExpiresAt: target.ExpirationDate, Selected: strings.EqualFold(target.Alias, settings.SalesforceAlias),
		})
	}
	return options
}

func saveSalesforceSelection(settings config.ConnectionSettings, target salesforceorg.Target) config.ConnectionSettings {
	settings.SalesforceAlias = target.Alias
	settings.SalesforceOrgID = ""
	settings.SalesforceOrgStatus = target.Status
	settings.SalesforceExpirationDate = target.ExpirationDate
	settings.SalesforceOrgType = "persistent"
	if target.IsScratch() {
		settings.SalesforceOrgType = "scratch"
	}
	if settings.VerifiedConnections != nil {
		delete(settings.VerifiedConnections, "salesforce")
	}
	return settings
}

func loadPlan() (config.SolutionPlan, error) {
	return shellstate.LoadSolutionPlan()
}

func savePlan(plan config.SolutionPlan) error {
	return shellstate.SaveSolutionPlan(plan)
}
