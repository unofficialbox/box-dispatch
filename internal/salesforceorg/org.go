// Package salesforceorg centralizes Salesforce CLI target inspection and
// scratch-org creation. It deliberately keeps credentials out of returned
// diagnostics while preserving the complete CLI error and stack payload.
package salesforceorg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

const displayTimeout = 20 * time.Second

var (
	sensitiveAssignment = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|client[_-]?secret|sessionid|password)(["']?\s*[:=]\s*["']?)([^\s"',}]+)`)
	bearerCredential    = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+|bearer\s+)[A-Za-z0-9._~+/=-]+`)
)

// Info is the non-secret portion of `sf org display --json` used by Dispatch.
type Info struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	Alias           string `json:"alias"`
	InstanceURL     string `json:"instanceUrl"`
	Status          string `json:"status"`
	ConnectedStatus string `json:"connectedStatus"`
	ExpirationDate  string `json:"expirationDate"`
	DevHubID        string `json:"devHubId"`
}

// DevHub is the non-secret identity surfaced by `sf org list --json` for an
// authenticated Dev Hub that can create scratch orgs.
type DevHub struct {
	Alias           string `json:"alias"`
	Username        string `json:"username"`
	OrgID           string `json:"orgId"`
	ConnectedStatus string `json:"connectedStatus"`
}

// Target is a credential-free Salesforce alias that Dispatch can safely offer
// to the browser. Aliases are the only selectable identifier so usernames and
// instance URLs never leave the local CLI boundary.
type Target struct {
	Alias           string `json:"alias"`
	ConnectedStatus string `json:"connectedStatus"`
	Status          string `json:"status"`
	ExpirationDate  string `json:"expirationDate"`
	DevHubID        string `json:"devHubId"`
}

// ScratchAccess is a current CLI-owned scratch-org session recovered for a
// legacy Dispatch connection. It stays inside the local Go process and is
// never returned by the browser-facing connection APIs.
type ScratchAccess struct {
	Target         string
	Alias          string
	Username       string
	OrgID          string
	InstanceURL    string
	AccessToken    string
	ExpirationDate string
}

type scratchTarget struct {
	Alias          string `json:"alias"`
	Username       string `json:"username"`
	OrgID          string `json:"orgId"`
	InstanceURL    string `json:"instanceUrl"`
	ExpirationDate string `json:"expirationDate"`
}

func (t Target) IsScratch() bool {
	return strings.TrimSpace(t.DevHubID) != "" || strings.TrimSpace(t.ExpirationDate) != ""
}

func (t Target) Connected() bool {
	return strings.EqualFold(strings.TrimSpace(t.ConnectedStatus), "connected")
}

// Healthy reports whether an authenticated target can be selected for a new
// Dispatch run. Deployment performs a full `sf org display` health check
// later; this keeps obviously expired scratch orgs out of the browser picker.
func (t Target) Healthy(now time.Time) bool {
	if !t.Connected() {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(t.Status))
	if status == "deleted" || status == "expired" {
		return false
	}
	if expiration, ok := parseExpiration(t.ExpirationDate); ok && !expiration.After(dateOnly(now)) {
		return false
	}
	return true
}

func (d DevHub) Target() string {
	if value := strings.TrimSpace(d.Alias); value != "" {
		return value
	}
	return strings.TrimSpace(d.Username)
}

func (d DevHub) Connected() bool {
	return strings.EqualFold(strings.TrimSpace(d.ConnectedStatus), "connected")
}

// Failure separates operator-facing guidance from complete sanitized CLI
// diagnostics. Error returns only the concise message by design.
type Failure struct {
	Summary    string
	Diagnostic string
}

func (f *Failure) Error() string { return f.Summary }

// IsScratch reports whether Salesforce identifies the target as disposable.
func (i Info) IsScratch() bool {
	return strings.TrimSpace(i.DevHubID) != "" || strings.TrimSpace(i.ExpirationDate) != ""
}

// EffectiveStatus normalizes the two status fields emitted by Salesforce CLI
// versions into one value.
func (i Info) EffectiveStatus() string {
	if value := strings.TrimSpace(i.Status); value != "" {
		return value
	}
	return strings.TrimSpace(i.ConnectedStatus)
}

// Target returns the most useful human-readable identifier for the org.
func (i Info) Target() string {
	for _, value := range []string{i.Alias, i.Username, i.ID} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "selected Salesforce org"
}

// Inspect runs a bounded Salesforce CLI status check and rejects stale,
// deleted, or expired targets even when `sf org display` exits successfully.
func Inspect(target string, now time.Time) (Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), displayTimeout)
	defer cancel()
	args := []string{"org", "display", "--json"}
	if target = strings.TrimSpace(target); target != "" {
		args = append(args, "--target-org", target)
	}
	output, runErr := exec.CommandContext(ctx, "sf", args...).CombinedOutput()
	if runErr != nil {
		return Info{}, NewFailure("Unable to inspect Salesforce org "+quotedTarget(target)+". Reauthenticate it or choose another org.", output, runErr)
	}
	info, parseErr := ParseDisplay(output)
	if parseErr != nil {
		return Info{}, NewFailure("Salesforce returned an unreadable org status for "+quotedTarget(target)+". Update the Salesforce CLI and retry.", output, parseErr)
	}
	if failure := HealthFailure(info, now); failure != nil {
		failure.Diagnostic = Diagnostic(output, nil)
		return info, failure
	}
	return info, nil
}

func quotedTarget(target string) string {
	if strings.TrimSpace(target) == "" {
		return "the current default"
	}
	return fmt.Sprintf("%q", target)
}

// ParseDisplay decodes the first JSON object, tolerating CLI update notices.
func ParseDisplay(output []byte) (Info, error) {
	var payload struct {
		Result Info `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return Info{}, err
	}
	return payload.Result, nil
}

// HealthFailure identifies a target that must not receive validation or deploy
// commands. Salesforce's cached auth can remain present after scratch expiry,
// so username existence alone is not a connectivity signal.
func HealthFailure(info Info, now time.Time) *Failure {
	target := fmt.Sprintf("%q", info.Target())
	status := strings.ToLower(info.EffectiveStatus())
	expires, hasExpiration := parseExpiration(info.ExpirationDate)
	today := dateOnly(now)
	expiredByDate := hasExpiration && !expires.After(today)

	if status == "deleted" {
		detail := ""
		if hasExpiration {
			detail = ", expired " + expires.Format("Jan 2, 2006")
		}
		return &Failure{Summary: "Salesforce scratch org " + target + " is deleted" + detail + ". Create or choose a replacement from Connect > Salesforce before validating or deploying."}
	}
	if status == "expired" || expiredByDate {
		when := ""
		if hasExpiration {
			when = " on " + expires.Format("Jan 2, 2006")
		}
		return &Failure{Summary: "Salesforce scratch org " + target + " expired" + when + ". Create or choose a replacement from Connect > Salesforce before validating or deploying."}
	}
	if info.IsScratch() && status != "" && status != "active" && status != "connected" {
		return &Failure{Summary: "Salesforce scratch org " + target + " is not active (status: " + info.EffectiveStatus() + "). Create or choose a replacement from Connect > Salesforce."}
	}
	if strings.TrimSpace(info.Username) == "" {
		return &Failure{Summary: "Salesforce did not return a username for " + target + ". Reauthenticate it or choose another org."}
	}
	return nil
}

func parseExpiration(value string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return dateOnly(parsed), true
		}
	}
	return time.Time{}, false
}

func dateOnly(value time.Time) time.Time {
	y, m, d := value.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ListDevHubs discovers authenticated Dev Hubs without requiring one to be the
// Salesforce CLI's global default.
func ListDevHubs() ([]DevHub, error) {
	ctx, cancel := context.WithTimeout(context.Background(), displayTimeout)
	defer cancel()
	output, runErr := exec.CommandContext(ctx, "sf", "org", "list", "--json").CombinedOutput()
	if runErr != nil {
		return nil, NewFailure("Unable to discover authenticated Salesforce Dev Hubs. Reauthenticate a Dev Hub and retry.", output, runErr)
	}
	hubs, parseErr := ParseDevHubs(output)
	if parseErr != nil {
		return nil, NewFailure("Salesforce returned an unreadable Dev Hub inventory. Update the Salesforce CLI and retry.", output, parseErr)
	}
	return hubs, nil
}

// ListTargets discovers aliased Salesforce orgs already authenticated by the
// local Salesforce CLI. It never returns usernames, organization IDs, or URLs.
func ListTargets() ([]Target, error) {
	ctx, cancel := context.WithTimeout(context.Background(), displayTimeout)
	defer cancel()
	output, runErr := exec.CommandContext(ctx, "sf", "org", "list", "--json").CombinedOutput()
	if runErr != nil {
		return nil, NewFailure("Unable to discover authenticated Salesforce orgs. Reauthenticate an org in the Salesforce CLI and retry.", output, runErr)
	}
	targets, parseErr := ParseTargets(output)
	if parseErr != nil {
		return nil, NewFailure("Salesforce returned an unreadable authenticated-org inventory. Update the Salesforce CLI and retry.", output, parseErr)
	}
	return targets, nil
}

// RecoverScratchAccess retrieves a current access token only for a scratch org
// already authenticated by the local Salesforce CLI. Dispatch-created scratch
// orgs use their stored OAuth refresh token and do not enter this legacy path.
func RecoverScratchAccess(ctx context.Context, orgID, username, instanceURL string) (ScratchAccess, error) {
	output, runErr := exec.CommandContext(ctx, "sf", "org", "list", "--json").CombinedOutput()
	if runErr != nil {
		return ScratchAccess{}, NewFailure("Unable to recover the saved Salesforce scratch-org session.", output, runErr)
	}
	target, err := findScratchTarget(output, orgID, username, instanceURL)
	if err != nil {
		return ScratchAccess{}, &Failure{Summary: "The selected scratch org is not authenticated locally. Reconnect it, then try again.", Diagnostic: err.Error()}
	}
	identifier := firstPopulated(target.Alias, target.Username)
	output, runErr = exec.CommandContext(ctx, "sf", "org", "display", "--target-org", identifier, "--json").CombinedOutput()
	if runErr != nil {
		return ScratchAccess{}, NewFailure("Unable to renew the saved Salesforce scratch-org session.", output, runErr)
	}
	access, err := parseScratchAccess(output)
	if err != nil {
		return ScratchAccess{}, NewFailure("Salesforce returned an unreadable scratch-org session.", output, err)
	}
	access.Alias = firstPopulated(access.Alias, target.Alias)
	access.Target = identifier
	access.Username = firstPopulated(access.Username, target.Username)
	access.OrgID = firstPopulated(access.OrgID, target.OrgID)
	access.InstanceURL = firstPopulated(access.InstanceURL, target.InstanceURL)
	access.ExpirationDate = firstPopulated(access.ExpirationDate, target.ExpirationDate)
	return access, nil
}

// OpenScratchURL asks the Salesforce CLI for a one-time front-door URL. CLI-
// managed scratch sessions use this OTP launch rather than a reusable API
// access token for browser login.
func OpenScratchURL(ctx context.Context, target, returnPath string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("Salesforce scratch-org target is unavailable")
	}
	args := []string{"org", "open", "--target-org", target, "--url-only", "--json"}
	if returnPath = strings.TrimSpace(returnPath); returnPath != "" {
		args = append(args, "--path", returnPath)
	}
	output, runErr := exec.CommandContext(ctx, "sf", args...).CombinedOutput()
	if runErr != nil {
		return "", NewFailure("Unable to open the saved Salesforce scratch org.", output, runErr)
	}
	launchURL, err := parseScratchLaunchURL(output)
	if err != nil {
		return "", &Failure{Summary: "Salesforce returned an invalid scratch-org launch URL.", Diagnostic: err.Error()}
	}
	return launchURL, nil
}

func findScratchTarget(output []byte, orgID, username, instanceURL string) (scratchTarget, error) {
	var payload struct {
		Result struct {
			ScratchOrgs []scratchTarget `json:"scratchOrgs"`
		} `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return scratchTarget{}, err
	}
	wantedHost := normalizedHost(instanceURL)
	for _, candidate := range payload.Result.ScratchOrgs {
		if strings.TrimSpace(orgID) != "" && strings.EqualFold(strings.TrimSpace(candidate.OrgID), strings.TrimSpace(orgID)) {
			return candidate, nil
		}
	}
	for _, candidate := range payload.Result.ScratchOrgs {
		if strings.TrimSpace(username) != "" && strings.EqualFold(strings.TrimSpace(candidate.Username), strings.TrimSpace(username)) {
			return candidate, nil
		}
	}
	var hostMatch scratchTarget
	matches := 0
	for _, candidate := range payload.Result.ScratchOrgs {
		if wantedHost != "" && normalizedHost(candidate.InstanceURL) == wantedHost {
			hostMatch = candidate
			matches++
		}
	}
	if matches == 1 {
		return hostMatch, nil
	}
	return scratchTarget{}, fmt.Errorf("no unique authenticated scratch org matched the saved connection")
}

func parseScratchAccess(output []byte) (ScratchAccess, error) {
	var payload struct {
		Result struct {
			Alias          string `json:"alias"`
			Username       string `json:"username"`
			OrgID          string `json:"id"`
			InstanceURL    string `json:"instanceUrl"`
			AccessToken    string `json:"accessToken"`
			ExpirationDate string `json:"expirationDate"`
		} `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return ScratchAccess{}, err
	}
	if strings.TrimSpace(payload.Result.AccessToken) == "" {
		return ScratchAccess{}, fmt.Errorf("Salesforce CLI did not return an access token")
	}
	return ScratchAccess{
		Alias: payload.Result.Alias, Username: payload.Result.Username, OrgID: payload.Result.OrgID,
		InstanceURL: payload.Result.InstanceURL, AccessToken: payload.Result.AccessToken,
		ExpirationDate: payload.Result.ExpirationDate,
	}, nil
}

func parseScratchLaunchURL(output []byte) (string, error) {
	var payload struct {
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return "", err
	}
	parsed, err := url.Parse(strings.TrimSpace(payload.Result.URL))
	if err != nil || parsed.Scheme != "https" || parsed.Path != "/secur/frontdoor.jsp" || strings.TrimSpace(parsed.Query().Get("otp")) == "" {
		return "", fmt.Errorf("Salesforce CLI did not return a valid one-time front-door URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".salesforce.com") && !strings.HasSuffix(host, ".force.com") {
		return "", fmt.Errorf("Salesforce CLI returned a launch URL on an unexpected host")
	}
	return parsed.String(), nil
}

func normalizedHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func firstPopulated(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// ParseTargets extracts the aliased non-scratch and scratch org records from
// `sf org list --json`, removing duplicate aliases and disconnected records.
func ParseTargets(output []byte) ([]Target, error) {
	var payload struct {
		Result struct {
			NonScratchOrgs []Target `json:"nonScratchOrgs"`
			ScratchOrgs    []Target `json:"scratchOrgs"`
		} `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	targets := make([]Target, 0, len(payload.Result.NonScratchOrgs)+len(payload.Result.ScratchOrgs))
	for _, target := range append(payload.Result.NonScratchOrgs, payload.Result.ScratchOrgs...) {
		target.Alias = strings.TrimSpace(target.Alias)
		if target.Alias == "" || !target.Connected() || seen[target.Alias] {
			continue
		}
		seen[target.Alias] = true
		targets = append(targets, target)
	}
	slices.SortStableFunc(targets, func(a, b Target) int {
		if a.IsScratch() != b.IsScratch() {
			if a.IsScratch() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Alias), strings.ToLower(b.Alias))
	})
	return targets, nil
}

// ParseDevHubs decodes, deduplicates, and sorts the Dev Hub inventory. Known
// connected hubs sort before entries whose connection state is unavailable.
func ParseDevHubs(output []byte) ([]DevHub, error) {
	var payload struct {
		Result struct {
			DevHubs []DevHub `json:"devHubs"`
		} `json:"result"`
	}
	if err := decodeJSON(output, &payload); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	hubs := make([]DevHub, 0, len(payload.Result.DevHubs))
	for _, hub := range payload.Result.DevHubs {
		target := hub.Target()
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		hubs = append(hubs, hub)
	}
	slices.SortStableFunc(hubs, func(a, b DevHub) int {
		if a.Connected() != b.Connected() {
			if a.Connected() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Target()), strings.ToLower(b.Target()))
	})
	return hubs, nil
}

// CreateScratch creates a 30-day Developer scratch org against the explicitly
// selected Dev Hub. Solution-specific package installation remains a later
// lifecycle step; this function only establishes a healthy Salesforce target.
func CreateScratch(alias, devHub string) (Info, error) {
	devHub = strings.TrimSpace(devHub)
	if devHub == "" {
		return Info{}, &Failure{Summary: "Choose an authenticated Salesforce Dev Hub before creating a scratch org."}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	args := scratchCreateArgs(alias, devHub)
	output, runErr := exec.CommandContext(ctx, "sf", args...).CombinedOutput()
	if runErr != nil {
		summary := "Unable to create the Salesforce scratch org."
		lower := strings.ToLower(string(output))
		if strings.Contains(lower, "target-dev-hub") || strings.Contains(lower, "dev hub") {
			summary = "Salesforce Dev Hub " + fmt.Sprintf("%q", devHub) + " could not create the scratch org. Choose another authenticated Dev Hub or reconnect this one, then retry."
		} else {
			summary += " Check the Dev Hub scratch-org limits and retry."
		}
		return Info{}, NewFailure(summary, output, runErr)
	}
	// Creation payloads vary by CLI version, so inspect the stable alias after a
	// successful command instead of depending on the create response shape.
	return Inspect(alias, time.Now())
}

func scratchCreateArgs(alias, devHub string) []string {
	return []string{
		"org", "create", "scratch",
		"--edition", "developer",
		"--duration-days", "30",
		"--target-dev-hub", devHub,
		"--alias", alias,
		"--set-default",
		"--wait", "15",
		"--json",
	}
}

// NewFailure builds a sanitized complete diagnostic while keeping the normal
// UI message concise and actionable.
func NewFailure(summary string, output []byte, runErr error) *Failure {
	return &Failure{Summary: summary, Diagnostic: Diagnostic(output, runErr)}
}

// CLIErrorDetails extracts Salesforce's message/actions for the normal UI and
// preserves the complete sanitized JSON (including stack and data) for `d`.
func CLIErrorDetails(output []byte, runErr error) (string, string) {
	var payload struct {
		Name    string   `json:"name"`
		Message string   `json:"message"`
		Actions []string `json:"actions"`
		Result  struct {
			Details struct {
				ComponentFailures json.RawMessage `json:"componentFailures"`
			} `json:"details"`
			Files []struct {
				FullName string `json:"fullName"`
				Type     string `json:"type"`
				State    string `json:"state"`
				Error    string `json:"error"`
			} `json:"files"`
		} `json:"result"`
	}
	if decodeJSON(output, &payload) == nil {
		if strings.EqualFold(payload.Name, "ERROR_HTTP_420") || strings.Contains(strings.ToLower(payload.Message), "html content") {
			return "Salesforce returned HTTP 420 with HTML instead of Metadata API JSON. The selected org may have expired, been deleted, or lost authorization. Return to Connect > Salesforce, check or replace the org, then validate again.", Diagnostic(output, runErr)
		}
		if strings.Contains(strings.ToLower(payload.Message), "maximum size of request") {
			return "Salesforce rejected the metadata request because it exceeded the API size limit. Ensure node_modules is excluded by .forceignore, rebuild the package, and retry.", Diagnostic(output, runErr)
		}
		if summary := metadataFailureSummary(payload.Result.Details.ComponentFailures, payload.Result.Files); summary != "" {
			return summary, Diagnostic(output, runErr)
		}
		if payload.Message == "" {
			return fallbackSummary(output, runErr), Diagnostic(output, runErr)
		}
		parts := []string{}
		if payload.Name != "" {
			parts = append(parts, payload.Name)
		}
		parts = append(parts, payload.Message)
		if len(payload.Actions) > 0 {
			parts = append(parts, "Next: "+strings.Join(payload.Actions, " "))
		}
		return strings.Join(parts, ": "), Diagnostic(output, runErr)
	}
	return fallbackSummary(output, runErr), Diagnostic(output, runErr)
}

type metadataComponentFailure struct {
	ComponentType string `json:"componentType"`
	FullName      string `json:"fullName"`
	Problem       string `json:"problem"`
}

func metadataFailureSummary(raw json.RawMessage, files []struct {
	FullName string `json:"fullName"`
	Type     string `json:"type"`
	State    string `json:"state"`
	Error    string `json:"error"`
}) string {
	failures := []metadataComponentFailure{}
	if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
		if err := json.Unmarshal(raw, &failures); err != nil {
			var single metadataComponentFailure
			if json.Unmarshal(raw, &single) == nil {
				failures = append(failures, single)
			}
		}
	}
	for _, file := range files {
		if strings.TrimSpace(file.Error) == "" {
			continue
		}
		failures = append(failures, metadataComponentFailure{ComponentType: file.Type, FullName: file.FullName, Problem: file.Error})
	}
	if len(failures) == 0 {
		return ""
	}
	selected := failures[0]
	for _, failure := range failures {
		if strings.Contains(strings.ToLower(failure.Problem), "invalid type: box.toolkit") {
			selected = failure
			break
		}
		if strings.Contains(strings.ToLower(selected.Problem), "dependent class is invalid") && !strings.Contains(strings.ToLower(failure.Problem), "dependent class is invalid") {
			selected = failure
		}
	}
	component := strings.TrimSpace(strings.TrimSpace(selected.ComponentType) + " " + strings.TrimSpace(selected.FullName))
	if component == "" {
		component = "a packaged component"
	}
	problem := strings.TrimSpace(selected.Problem)
	summary := "Salesforce metadata deployment failed for " + component + ": " + problem
	if strings.Contains(strings.ToLower(problem), "invalid type: box.toolkit") {
		summary += ". The required Box for Salesforce managed package is missing or incompatible. Install the version declared in dispatch.bcl, then validate again"
	}
	if additional := len(failures) - 1; additional > 0 {
		summary += fmt.Sprintf(". %d additional component failure(s) are available in the full Salesforce CLI diagnostic", additional)
	}
	return strings.TrimSpace(summary) + "."
}

// Diagnostic returns the complete CLI payload with credential-shaped fields
// redacted. Unlike the concise summary, stack and error.data are retained.
func Diagnostic(output []byte, runErr error) string {
	clean := bytes.TrimSpace(output)
	if start := bytes.IndexByte(clean, '{'); start >= 0 {
		var value any
		if json.Unmarshal(clean[start:], &value) == nil {
			redact(value)
			if pretty, err := json.MarshalIndent(value, "", "  "); err == nil {
				return string(pretty)
			}
		}
	}
	if len(clean) > 0 {
		return redactText(string(clean))
	}
	if runErr != nil {
		return runErr.Error()
	}
	return "No Salesforce CLI diagnostic payload was returned."
}

func redactText(value string) string {
	value = sensitiveAssignment.ReplaceAllString(value, `${1}${2}[REDACTED]`)
	return bearerCredential.ReplaceAllStringFunc(value, func(match string) string {
		if index := strings.LastIndex(match, " "); index >= 0 {
			return match[:index+1] + "[REDACTED]"
		}
		return "[REDACTED]"
	})
}

func redact(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "sessionid") {
				typed[key] = "[REDACTED]"
				continue
			}
			redact(child)
		}
	case []any:
		for _, child := range typed {
			redact(child)
		}
	}
}

func decodeJSON(output []byte, target any) error {
	start := bytes.IndexByte(output, '{')
	if start < 0 {
		return fmt.Errorf("Salesforce CLI output did not contain JSON")
	}
	return json.NewDecoder(bytes.NewReader(output[start:])).Decode(target)
}

func fallbackSummary(output []byte, runErr error) string {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	if detail := strings.TrimSpace(strings.Join(lines, "\n")); detail != "" {
		return detail
	}
	if runErr != nil {
		return runErr.Error()
	}
	return "Salesforce CLI command failed without an error message."
}
