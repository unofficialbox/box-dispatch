package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

type CheckConfig struct {
	Scenario  string
	Platform  string
	Offline   bool
	Providers []string
}

// ProviderDiscovery carries the identity details a provider reports once it is
// reachable. Fields are provider-specific and left empty when unavailable.
type ProviderDiscovery struct {
	Identity   string   `json:"identity,omitempty"`   // login / username the CLI is authenticated as
	Account    string   `json:"account,omitempty"`    // Box user ID, AWS account number
	Enterprise string   `json:"enterprise,omitempty"` // Box enterprise ID
	Profile    string   `json:"profile,omitempty"`    // Salesforce alias, Databricks/AWS profile
	Host       string   `json:"host,omitempty"`       // Salesforce instance URL, Databricks workspace
	Region     string   `json:"region,omitempty"`     // AWS region
	Options    []string `json:"options,omitempty"`    // selectable authenticated profiles/aliases
	AuthType   string   `json:"auth_type,omitempty"`  // Box auth of the active connection: OAuth2 | CCG | JWT
}

type ProviderResult struct {
	Name              string            `json:"name"`
	Checks            []string          `json:"checks"`
	Guidance          []string          `json:"guidance"`
	ToolInstalled     bool              `json:"tool_installed"`
	ConfigSatisfied   bool              `json:"config_satisfied"`
	ConnectivityOK    bool              `json:"connectivity_ok"`
	Blocked           bool              `json:"blocked"`
	RequiresAttention bool              `json:"requires_attention"`
	Discovery         ProviderDiscovery `json:"discovery"`
}

type CheckReport struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Scenario    string           `json:"scenario"`
	Platform    string           `json:"platform"`
	Offline     bool             `json:"offline"`
	Providers   []ProviderResult `json:"providers"`
}

var allProviders = []string{"box", "salesforce", "databricks", "aws"}

func ProvidersForScenarioAndPlatform(scenario, platform string) []string {
	_ = scenario
	if platform == "" {
		return append([]string{}, allProviders...)
	}
	platform = strings.ToLower(platform)
	for _, p := range allProviders {
		if p == platform {
			return []string{p}
		}
	}
	return append([]string{}, allProviders...)
}

type provider struct {
	name       string
	guidance   func() []string
	connect    func() (bool, string, ProviderDiscovery)
	tool       func() bool
	configured func() bool
	// options lists the locally authenticated profiles a user can pick between.
	// It runs as soon as the tool is present, so profile selection stays
	// available before connectivity succeeds. May be nil.
	options func() []string
}

func Check(cfg CheckConfig) (CheckReport, error) {
	report := CheckReport{
		GeneratedAt: time.Now(),
		Scenario:    cfg.Scenario,
		Platform:    cfg.Platform,
		Offline:     cfg.Offline,
		Providers:   []ProviderResult{},
	}

	for _, name := range cfg.Providers {
		p, ok := allProviderBuilders[name]
		if !ok {
			p = newUnknownProvider(name)
		}
		report.Providers = append(report.Providers, checkProvider(p, cfg))
	}
	return report, nil
}

func checkProvider(p provider, cfg CheckConfig) ProviderResult {
	res := ProviderResult{Name: p.name}
	res.ToolInstalled = p.tool()
	if !res.ToolInstalled {
		res.Blocked = true
		res.RequiresAttention = true
		res.Checks = append(res.Checks, "tool dependency not found")
		res.Guidance = append(res.Guidance, p.guidance()...)
		return res
	}

	if p.options != nil {
		res.Discovery.Options = p.options()
	}

	res.ConfigSatisfied = p.configured()
	res.Checks = append(res.Checks, fmt.Sprintf("%s tools discovered", p.name))

	if !res.ConfigSatisfied {
		res.RequiresAttention = true
		res.Checks = append(res.Checks, "required credentials/config missing")
		res.Guidance = append(res.Guidance, p.guidance()...)
		res.Checks = append(res.Checks, "connectivity skipped until credentials are configured")
		return res
	}

	if cfg.Offline {
		res.Checks = append(res.Checks, "connectivity check skipped (--offline)")
		return res
	}

	ok, detail, discovered := p.connect()
	res.ConnectivityOK = ok
	res.Checks = append(res.Checks, detail)
	mergeDiscovery(&res.Discovery, discovered)
	if !ok {
		res.RequiresAttention = true
		res.Checks = append(res.Checks, "provider not connected")
		res.Guidance = append(res.Guidance, p.guidance()...)
	}
	return res
}

// mergeDiscovery copies non-empty fields from src over dst, so connectivity
// results refine (rather than erase) anything already discovered locally.
func mergeDiscovery(dst *ProviderDiscovery, src ProviderDiscovery) {
	for _, field := range []struct {
		into *string
		from string
	}{
		{&dst.Identity, src.Identity},
		{&dst.Account, src.Account},
		{&dst.Enterprise, src.Enterprise},
		{&dst.Profile, src.Profile},
		{&dst.Host, src.Host},
		{&dst.Region, src.Region},
		{&dst.AuthType, src.AuthType},
	} {
		if strings.TrimSpace(field.from) != "" {
			*field.into = field.from
		}
	}
	if len(src.Options) > 0 {
		dst.Options = src.Options
	}
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCommandOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

// decodeCLIJSON tolerates CLI notices written before a JSON response. The
// Salesforce CLI currently prints update warnings to stderr even when --json
// is requested; CombinedOutput preserves that warning ahead of the payload.
func decodeCLIJSON(out string, target any) error {
	clean := ansiEscape.ReplaceAllString(out, "")
	start := strings.IndexByte(clean, '{')
	if start < 0 {
		return fmt.Errorf("CLI output did not contain JSON")
	}
	return json.NewDecoder(strings.NewReader(clean[start:])).Decode(target)
}

// boxUserFields is the field set both transports request; enterprise is not
// part of the default projection and has to be named explicitly.
const boxUserFields = "id,login,name,enterprise"

type boxUser struct {
	ID         string `json:"id"`
	Login      string `json:"login"`
	Enterprise struct {
		ID string `json:"id"`
	} `json:"enterprise"`
}

func (u boxUser) discovery() ProviderDiscovery {
	return ProviderDiscovery{Identity: u.Login, Account: u.ID, Enterprise: u.Enterprise.ID}
}

// connectivityBox validates the connection box-dispatch will actually deploy
// with. When that is the box-dispatch CCG app, it mints a CCG token and checks
// it directly — the box CLI is a different (OAuth) identity, so testing the CLI
// would report on the wrong connection. Otherwise it prefers an explicit
// BOX_ACCESS_TOKEN, then the authenticated CLI session as the transport (the CLI
// cannot hand back a raw token under an OAuth login).
func connectivityBox() (bool, string, ProviderDiscovery) {
	active, found := boxconn.Active()
	if found && active.Source == boxconn.SourceDispatch {
		ok, detail, discovery := connectivityBoxCCG()
		discovery.AuthType = active.AuthType
		return ok, detail, discovery
	}
	ok, detail, discovery := connectivityBoxTransport()
	if found {
		discovery.AuthType = active.AuthType
	}
	return ok, detail, discovery
}

// connectivityBoxCCG mints a token for the captured CCG app and confirms it can
// read the acting user, so the reported status reflects the CCG connection
// itself rather than the CLI's separate OAuth login.
func connectivityBoxCCG() (bool, string, ProviderDiscovery) {
	var discovery ProviderDiscovery
	settings, err := shellstate.LoadConnectionSettings()
	if err != nil || !settings.HasBoxCCG() {
		return false, "box CCG app is not fully configured", discovery
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	token, err := boxconn.CCGTokenFromSettings(ctx, settings)
	if err != nil {
		return false, "box CCG app could not authenticate: " + firstLine(err.Error()), discovery
	}
	return connectivityBoxBearer(token)
}

func connectivityBoxTransport() (bool, string, ProviderDiscovery) {
	if strings.TrimSpace(os.Getenv("BOX_ACCESS_TOKEN")) != "" {
		return connectivityBoxToken()
	}
	if toolExists("box") {
		return connectivityBoxCLI()
	}
	return false, "missing BOX_ACCESS_TOKEN and no authenticated Box CLI", ProviderDiscovery{}
}

func connectivityBoxCLI() (bool, string, ProviderDiscovery) {
	var discovery ProviderDiscovery
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "box", "users:get", "me", "--json", "--fields="+boxUserFields)
	if err != nil {
		return false, fmt.Sprintf("box cli not authenticated: %s", firstLine(out)), discovery
	}
	var user boxUser
	if err := json.Unmarshal([]byte(out), &user); err != nil {
		return false, "box cli returned an unreadable user record", discovery
	}
	discovery = user.discovery()
	if discovery.Identity == "" {
		return false, "box cli returned no authenticated user", discovery
	}
	return true, fmt.Sprintf("box cli authenticated as %s", discovery.Identity), discovery
}

func connectivityBoxToken() (bool, string, ProviderDiscovery) {
	return connectivityBoxBearer(strings.TrimSpace(os.Getenv("BOX_ACCESS_TOKEN")))
}

// connectivityBoxBearer reads the acting user from the Box API with a bearer
// token, shared by the BOX_ACCESS_TOKEN and CCG paths.
func connectivityBoxBearer(token string) (bool, string, ProviderDiscovery) {
	var discovery ProviderDiscovery
	req, _ := http.NewRequest("GET", "https://api.box.com/2.0/users/me?fields="+boxUserFields, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return false, fmt.Sprintf("box api request failed: %v", err), discovery
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("box api returned %s", resp.Status), discovery
	}
	var user boxUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err == nil {
		discovery = user.discovery()
	}
	if discovery.Identity != "" {
		return true, fmt.Sprintf("box api reachable as %s", discovery.Identity), discovery
	}
	return true, "box api reachable", discovery
}

func connectivitySalesforce() (bool, string, ProviderDiscovery) {
	var discovery ProviderDiscovery
	alias := strings.TrimSpace(os.Getenv("SF_ALIAS"))
	args := []string{"org", "display", "--json"}
	if alias != "" {
		args = append(args, "-o", alias)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "sf", args...)
	if err != nil {
		return false, "sf org display failed: " + salesforceCLIError(out), discovery
	}
	var payload struct {
		Result struct {
			Username    string `json:"username"`
			Alias       string `json:"alias"`
			InstanceURL string `json:"instanceUrl"`
		} `json:"result"`
	}
	if err := decodeCLIJSON(out, &payload); err != nil {
		return false, "sf org display returned unreadable JSON", discovery
	}
	if payload.Result.Username != "" {
		discovery.Identity = payload.Result.Username
		discovery.Profile = payload.Result.Alias
		discovery.Host = payload.Result.InstanceURL
		if discovery.Profile == "" {
			discovery.Profile = alias
		}
		return true, fmt.Sprintf("salesforce org connected: %s", payload.Result.Username), discovery
	}
	return true, "salesforce cli responded", discovery
}

// salesforceOptions lists the aliases/usernames the Salesforce CLI is authenticated against.
func salesforceOptions() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "sf", "org", "list", "--json")
	if err != nil {
		return nil
	}
	var payload struct {
		Result struct {
			NonScratchOrgs []struct {
				Alias    string `json:"alias"`
				Username string `json:"username"`
			} `json:"nonScratchOrgs"`
			ScratchOrgs []struct {
				Alias    string `json:"alias"`
				Username string `json:"username"`
			} `json:"scratchOrgs"`
		} `json:"result"`
	}
	if err := decodeCLIJSON(out, &payload); err != nil {
		return nil
	}
	options := make([]string, 0, len(payload.Result.NonScratchOrgs)+len(payload.Result.ScratchOrgs))
	add := func(alias, username string) {
		if value := strings.TrimSpace(alias); value != "" {
			options = append(options, value)
		} else if value := strings.TrimSpace(username); value != "" {
			options = append(options, value)
		}
	}
	for _, org := range payload.Result.NonScratchOrgs {
		add(org.Alias, org.Username)
	}
	for _, org := range payload.Result.ScratchOrgs {
		add(org.Alias, org.Username)
	}
	return dedupe(options)
}

// connectivityDatabricks prefers an explicit DATABRICKS_HOST/TOKEN pair, falling
// back to the databricks CLI (which authenticates via ~/.databrickscfg profiles
// or an interactive login), so a chosen profile connects without raw creds.
func connectivityDatabricks() (bool, string, ProviderDiscovery) {
	host := strings.TrimSpace(os.Getenv("DATABRICKS_HOST"))
	token := strings.TrimSpace(os.Getenv("DATABRICKS_TOKEN"))
	if host != "" && token != "" {
		return connectivityDatabricksToken()
	}
	if toolExists("databricks") {
		return connectivityDatabricksCLI()
	}
	return false, "missing DATABRICKS_HOST/DATABRICKS_TOKEN and no databricks CLI", ProviderDiscovery{}
}

func connectivityDatabricksToken() (bool, string, ProviderDiscovery) {
	discovery := ProviderDiscovery{Profile: strings.TrimSpace(os.Getenv("DATABRICKS_PROFILE"))}
	host := strings.TrimRight(strings.TrimSpace(os.Getenv("DATABRICKS_HOST")), "/")
	token := strings.TrimSpace(os.Getenv("DATABRICKS_TOKEN"))
	if host == "" || token == "" {
		return false, "missing DATABRICKS_HOST or DATABRICKS_TOKEN", discovery
	}
	discovery.Host = host
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest("GET", host+"/api/2.0/clusters/list", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("databricks api request failed: %v", err), discovery
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return false, fmt.Sprintf("databricks api returned %s", resp.Status), discovery
	}
	discovery.Identity = databricksCurrentUser(client, host, token)
	if discovery.Identity != "" {
		return true, fmt.Sprintf("databricks api reachable as %s", discovery.Identity), discovery
	}
	return true, "databricks api reachable", discovery
}

// databricksCurrentUser resolves the authenticated user, returning "" when the
// workspace does not expose the SCIM endpoint.
func databricksCurrentUser(client *http.Client, host, token string) string {
	req, _ := http.NewRequest("GET", host+"/api/2.0/preview/scim/v2/Me", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		UserName string `json:"userName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.UserName)
}

// connectivityDatabricksCLI validates the databricks CLI's authenticated
// session for the chosen profile (or the default), reporting the user and host.
func connectivityDatabricksCLI() (bool, string, ProviderDiscovery) {
	profile := strings.TrimSpace(os.Getenv("DATABRICKS_PROFILE"))
	discovery := ProviderDiscovery{Profile: profile}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"current-user", "me", "--output", "json"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	out, err := runCommandOutput(ctx, "databricks", args...)
	if err != nil {
		return false, "databricks cli not authenticated: " + firstLine(out), discovery
	}
	identity := databricksCLIIdentity(out)
	if identity == "" {
		return false, "databricks cli returned no authenticated user", discovery
	}
	discovery.Identity = identity
	discovery.Host = databricksCLIHost(profile)
	return true, "databricks cli authenticated as " + identity, discovery
}

// databricksCLIIdentity extracts the authenticated user from `current-user me`
// output, skipping any non-JSON preamble the CLI may print.
func databricksCLIIdentity(out string) string {
	start := strings.IndexByte(out, '{')
	if start < 0 {
		return ""
	}
	var payload struct {
		UserName string `json:"userName"`
		Emails   []struct {
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"emails"`
	}
	if json.NewDecoder(strings.NewReader(out[start:])).Decode(&payload) != nil {
		return ""
	}
	if payload.UserName != "" {
		return payload.UserName
	}
	for _, email := range payload.Emails {
		if email.Primary && strings.TrimSpace(email.Value) != "" {
			return email.Value
		}
	}
	return ""
}

// databricksCLIHost resolves the workspace host for a profile, best-effort.
func databricksCLIHost(profile string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	args := []string{"auth", "describe", "--output", "json"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	out, err := runCommandOutput(ctx, "databricks", args...)
	if err != nil {
		return ""
	}
	start := strings.IndexByte(out, '{')
	if start < 0 {
		return ""
	}
	var payload struct {
		Host    string `json:"host"`
		Details struct {
			Host string `json:"host"`
		} `json:"details"`
	}
	if json.NewDecoder(strings.NewReader(out[start:])).Decode(&payload) != nil {
		return ""
	}
	if payload.Details.Host != "" {
		return payload.Details.Host
	}
	return payload.Host
}

// databricksOptions reads selectable profile names out of ~/.databrickscfg.
// Only sections that carry a host are real auth profiles; the empty [DEFAULT]
// placeholder and the internal [__settings__] section are skipped because the
// CLI cannot target them with --profile.
func databricksOptions() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".databrickscfg"))
	if err != nil {
		return nil
	}
	return parseDatabricksProfiles(data)
}

func parseDatabricksProfiles(data []byte) []string {
	var options []string
	current := ""
	hasHost := false
	flush := func() {
		if current != "" && current != "__settings__" && hasHost {
			options = append(options, current)
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			flush()
			current = strings.TrimSpace(line[1 : len(line)-1])
			hasHost = false
		case strings.HasPrefix(strings.ToLower(line), "host"):
			hasHost = true
		}
	}
	flush()
	return dedupe(options)
}

func connectivityAWS() (bool, string, ProviderDiscovery) {
	discovery := ProviderDiscovery{
		Profile: strings.TrimSpace(os.Getenv("AWS_PROFILE")),
		Region:  firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION")),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return false, out, discovery
	}
	var payload struct {
		Account string `json:"Account"`
		Arn     string `json:"Arn"`
		UserID  string `json:"UserId"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return false, "aws output could not be parsed", discovery
	}
	discovery.Account = payload.Account
	discovery.Identity = awsIdentityFromARN(payload.Arn)
	return true, "aws sts identity resolved", discovery
}

// awsIdentityFromARN reduces an STS ARN to its trailing principal name.
func awsIdentityFromARN(arn string) string {
	arn = strings.TrimSpace(arn)
	if arn == "" {
		return ""
	}
	if index := strings.LastIndex(arn, "/"); index >= 0 && index < len(arn)-1 {
		return arn[index+1:]
	}
	return arn
}

// awsOptions lists locally configured AWS profiles.
func awsOptions() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "aws", "configure", "list-profiles")
	if err != nil {
		return nil
	}
	var options []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			options = append(options, name)
		}
	}
	return dedupe(options)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// salesforceCLIError reduces a failed `sf` command's output to a concise,
// ANSI-free message, preferring the CLI's structured error message.
func salesforceCLIError(out string) string {
	clean := ansiEscape.ReplaceAllString(out, "")
	var payload struct {
		Message string `json:"message"`
	}
	if decodeCLIJSON(clean, &payload) == nil && strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}
	return firstLine(clean)
}

// firstLine reduces multi-line CLI error output to its first non-empty line so
// it renders on a single status row.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func boxGuidance() []string {
	return []string{
		"Authenticate the Box CLI: box login",
		"or export BOX_ACCESS_TOKEN=<your-box-access-token>",
		"Verify with: box users:get me --json",
	}
}

func salesforceGuidance() []string {
	return []string{
		"Install Salesforce CLI: https://developer.salesforce.com/tools/salesforcecli",
		"Set SF_ALIAS and authenticate: sf org login web -a $SF_ALIAS",
		"Validate with: sf org display -o $SF_ALIAS --json",
	}
}

func databricksGuidance() []string {
	return []string{
		"Install Databricks CLI: https://docs.databricks.com/en/dev-tools/cli/index.html",
		"export DATABRICKS_HOST=https://<workspace>.cloud.databricks.com",
		"export DATABRICKS_TOKEN=<databricks-token>",
		"curl -H \"Authorization: Bearer $DATABRICKS_TOKEN\" $DATABRICKS_HOST/api/2.0/clusters/list",
	}
}

func awsGuidance() []string {
	return []string{
		"Install AWS CLI: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
		"export AWS_PROFILE=<aws-profile>",
		"export AWS_REGION=us-east-1",
		"aws sts get-caller-identity",
	}
}

func unknownGuidance(name string) []string {
	return []string{
		"Unsupported provider: " + name,
		"supported providers: box, salesforce, databricks, aws",
	}
}

var allProviderBuilders = map[string]provider{
	"box": {
		name: "box",
		tool: func() bool { return true },
		configured: func() bool {
			return strings.TrimSpace(os.Getenv("BOX_ACCESS_TOKEN")) != "" || toolExists("box")
		},
		connect:  connectivityBox,
		guidance: boxGuidance,
	},
	"salesforce": {
		name: "salesforce",
		tool: func() bool { return toolExists("sf") },
		configured: func() bool {
			// With the sf CLI present we can validate the chosen alias or the
			// authenticated default org, so an explicit SF_ALIAS/token is optional.
			return strings.TrimSpace(os.Getenv("SF_ALIAS")) != "" ||
				strings.TrimSpace(os.Getenv("SALESFORCE_ACCESS_TOKEN")) != "" ||
				toolExists("sf")
		},
		connect:  connectivitySalesforce,
		guidance: salesforceGuidance,
		options:  salesforceOptions,
	},
	"databricks": {
		name: "databricks",
		tool: func() bool { return toolExists("databricks") },
		configured: func() bool {
			// The databricks CLI validates a chosen/default profile, so an
			// explicit host+token pair is optional when the CLI is present.
			hasPair := strings.TrimSpace(os.Getenv("DATABRICKS_HOST")) != "" && strings.TrimSpace(os.Getenv("DATABRICKS_TOKEN")) != ""
			return hasPair || toolExists("databricks")
		},
		connect:  connectivityDatabricks,
		guidance: databricksGuidance,
		options:  databricksOptions,
	},
	"aws": {
		name: "aws",
		tool: func() bool { return toolExists("aws") },
		configured: func() bool {
			return strings.TrimSpace(os.Getenv("AWS_PROFILE")) != "" || strings.TrimSpace(os.Getenv("AWS_REGION")) != "" || strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")) != ""
		},
		connect:  connectivityAWS,
		guidance: awsGuidance,
		options:  awsOptions,
	},
}

func newUnknownProvider(name string) provider {
	return provider{
		name:       name,
		tool:       func() bool { return false },
		configured: func() bool { return false },
		connect:    func() (bool, string, ProviderDiscovery) { return false, "unsupported provider", ProviderDiscovery{} },
		guidance:   func() []string { return unknownGuidance(name) },
	}
}
