package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CheckConfig struct {
	Scenario  string
	Platform  string
	Offline   bool
	Providers []string
}

type ProviderResult struct {
	Name              string   `json:"name"`
	Checks           []string `json:"checks"`
	Guidance         []string `json:"guidance"`
	ToolInstalled    bool     `json:"tool_installed"`
	ConfigSatisfied  bool     `json:"config_satisfied"`
	ConnectivityOK   bool     `json:"connectivity_ok"`
	Blocked          bool     `json:"blocked"`
	RequiresAttention bool    `json:"requires_attention"`
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
	connect    func() (bool, string)
	tool       func() bool
	configured func() bool
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

	ok, detail := p.connect()
	res.ConnectivityOK = ok
	res.Checks = append(res.Checks, detail)
	if !ok {
		res.RequiresAttention = true
		res.Checks = append(res.Checks, "provider not connected")
		res.Guidance = append(res.Guidance, p.guidance()...)
	}
	return res
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

func connectivityBox() (bool, string) {
	token := strings.TrimSpace(os.Getenv("BOX_ACCESS_TOKEN"))
	if token == "" {
		return false, "missing BOX_ACCESS_TOKEN"
	}
	req, _ := http.NewRequest("GET", "https://api.box.com/2.0/users/me", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return false, fmt.Sprintf("box api request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("box api returned %s", resp.Status)
	}
	return true, "box api reachable"
}

func connectivitySalesforce() (bool, string) {
	alias := strings.TrimSpace(os.Getenv("SF_ALIAS"))
	args := []string{"org", "display", "--json"}
	if alias != "" {
		args = append(args, "-o", alias)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "sf", args...)
	if err != nil {
		return false, fmt.Sprintf("sf org display failed: %s", out)
	}
	var payload struct {
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err == nil && payload.Result.Username != "" {
		return true, fmt.Sprintf("salesforce org connected: %s", payload.Result.Username)
	}
	return true, "salesforce cli responded"
}

func connectivityDatabricks() (bool, string) {
	host := strings.TrimRight(strings.TrimSpace(os.Getenv("DATABRICKS_HOST")), "/")
	token := strings.TrimSpace(os.Getenv("DATABRICKS_TOKEN"))
	if host == "" || token == "" {
		return false, "missing DATABRICKS_HOST or DATABRICKS_TOKEN"
	}
	req, _ := http.NewRequest("GET", host+"/api/2.0/clusters/list", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return false, fmt.Sprintf("databricks api request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return false, fmt.Sprintf("databricks api returned %s", resp.Status)
	}
	return true, "databricks api reachable"
}

func connectivityAWS() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := runCommandOutput(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return false, out
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return false, "aws output could not be parsed"
	}
	return true, "aws sts identity resolved"
}

func boxGuidance() []string {
	return []string{
		"export BOX_ACCESS_TOKEN=<your-box-access-token>",
		"curl -H \"Authorization: Bearer $BOX_ACCESS_TOKEN\" https://api.box.com/2.0/users/me",
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
		name:       "box",
		tool:       func() bool { return true },
		configured: func() bool { return strings.TrimSpace(os.Getenv("BOX_ACCESS_TOKEN")) != "" },
		connect:    connectivityBox,
		guidance:   boxGuidance,
	},
	"salesforce": {
		name:       "salesforce",
		tool:       func() bool { return toolExists("sf") },
		configured: func() bool { return strings.TrimSpace(os.Getenv("SF_ALIAS")) != "" || strings.TrimSpace(os.Getenv("SALESFORCE_ACCESS_TOKEN")) != "" },
		connect:    connectivitySalesforce,
		guidance:   salesforceGuidance,
	},
	"databricks": {
		name:       "databricks",
		tool:       func() bool { return toolExists("databricks") },
		configured: func() bool { return strings.TrimSpace(os.Getenv("DATABRICKS_HOST")) != "" && strings.TrimSpace(os.Getenv("DATABRICKS_TOKEN")) != "" },
		connect:    connectivityDatabricks,
		guidance:   databricksGuidance,
	},
	"aws": {
		name:       "aws",
		tool:       func() bool { return toolExists("aws") },
		configured: func() bool { return strings.TrimSpace(os.Getenv("AWS_PROFILE")) != "" || strings.TrimSpace(os.Getenv("AWS_REGION")) != "" || strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")) != "" },
		connect:    connectivityAWS,
		guidance:   awsGuidance,
	},
}

func newUnknownProvider(name string) provider {
	return provider{
		name:       name,
		tool:       func() bool { return false },
		configured: func() bool { return false },
		connect:    func() (bool, string) { return false, "unsupported provider" },
		guidance:   func() []string { return unknownGuidance(name) },
	}
}
