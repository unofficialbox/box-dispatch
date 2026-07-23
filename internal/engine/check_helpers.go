package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/model"
	"github.com/unofficialbox/box-dispatch/internal/providers"
)

type ProviderCheckResult struct {
	Provider          string
	ConfigMissing     bool
	ToolsMissing      []string
	MissingTokens     []string
	ConnectivityError error
}

func (e *Engine) checkProvider(scenarioName, providerKey string, provider providers.ProviderSpec, cfg *config.RuntimeConfig, offline bool) ProviderCheckResult {
	result := ProviderCheckResult{
		Provider: providerKey,
	}

	for _, tool := range provider.RequiredTools {
		if _, err := os.Stat(tool); err == nil {
			continue
		}
		if _, err := exec.LookPath(tool); err != nil {
			result.ToolsMissing = append(result.ToolsMissing, tool)
		}
	}

	providerCfg, ok := cfg.Providers[providerKey]
	if !ok {
		result.ConfigMissing = true
		return result
	}

	resolved, missingTokens := config.ResolveProviderConfig(providerCfg, scenarioName, environmentMap())
	result.MissingTokens = missingTokens
	if len(result.ToolsMissing) > 0 || len(result.MissingTokens) > 0 {
		return result
	}

	if offline {
		return result
	}

	result.ConnectivityError = e.validateConnectivity(providerKey, resolved)
	return result
}

func (e *Engine) appendDependencyPhases(report *model.CommandReport, prefix, providerKey string, provider providers.ProviderSpec, result ProviderCheckResult, phaseStartTime time.Time, offline bool) {
	if result.ConfigMissing {
		report.AddPhase(prefix+"."+providerKey, string(model.PhaseBlocked), elapsedMs(phaseStartTime), fmt.Errorf("provider config missing from runtime file"))
		return
	}
	if len(result.ToolsMissing) > 0 {
		report.AddManual(providerKey + ".tools")
		report.AddPhase(prefix+"."+providerKey+".tools", string(model.PhaseWarn), elapsedMs(phaseStartTime), fmt.Errorf("missing tools: %s", unresolvedMissing(result.ToolsMissing)))
		return
	}
	if len(result.MissingTokens) > 0 {
		report.AddManual(providerKey + ".environment")
		report.AddPhase(prefix+"."+providerKey+".tokens", string(model.PhaseBlocked), elapsedMs(phaseStartTime), fmt.Errorf("missing tokens: %s", unresolvedMissing(result.MissingTokens)))
		return
	}
	if result.ConnectivityError != nil {
		report.AddManual(providerKey + ".connect")
		report.AddPhase(prefix+"."+providerKey+".connect", string(model.PhaseBlocked), elapsedMs(phaseStartTime), result.ConnectivityError)
		report.Validation[prefix+"."+providerKey+".connectGuidance"] = providerConnectivityGuide(providerKey)
		return
	}

	report.Validation[prefix+"."+providerKey] = map[string]any{
		"requiredTools":       provider.RequiredTools,
		"requiredTokens":      provider.RequiredTokens,
		"connectivityChecked": !offline,
	}
	report.AddPhase(prefix+"."+providerKey+".ok", string(model.PhasePassed), elapsedMs(phaseStartTime), nil)
}

func providerConnectivityGuide(providerKey string) []string {
	switch providerKey {
	case "box":
		return []string{
			"Set BOX_ACCESS_TOKEN and retry this check.",
			"Example: export BOX_ACCESS_TOKEN=<token>",
		}
	case "salesforce-agentforce":
		return []string{
			"Authenticate Salesforce and set SF_ALIAS (or re-run `sf org login web`).",
			"Set SF_PROJECT_DIR to the scenario's Salesforce project directory.",
		}
	case "databricks":
		return []string{
			"Set DATABRICKS_HOST and DATABRICKS_TOKEN in your environment.",
			"Optional quick check: databricks tokens list -p",
		}
	case "bedrock-agentcore":
		return []string{
			"Set AWS_PROFILE, AWS_REGION, and AGENTCORE_AGENT_NAME.",
			"Then verify AWS credentials with `aws sts get-caller-identity`.",
		}
	default:
		return []string{
			"Confirm required credentials and local tool availability.",
		}
	}
}

func (e *Engine) validateConnectivity(providerKey string, resolved map[string]any) error {
	values, ok := resolved["env"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing provider env section")
	}
	tokens := map[string]string{}
	for key, raw := range values {
		if text, ok := raw.(string); ok {
			tokens[key] = text
		}
	}

	switch providerKey {
	case "box":
		return validateBoxConnectivity(tokens)
	case "salesforce-agentforce":
		return validateSalesforceConnectivity(tokens)
	case "databricks":
		return validateDatabricksConnectivity(tokens)
	case "bedrock-agentcore":
		return validateAWSConnectivity(tokens)
	default:
		return nil
	}
}

func validateBoxConnectivity(tokens map[string]string) error {
	accessToken := strings.TrimSpace(tokens["BOX_ACCESS_TOKEN"])
	if accessToken == "" {
		return fmt.Errorf("BOX_ACCESS_TOKEN is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.box.com/2.0/users/me", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("box connectivity failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body := &strings.Builder{}
		_, _ = io.Copy(body, resp.Body)
		return fmt.Errorf("box API returned %d: %s", resp.StatusCode, truncate(body.String(), 160))
	}
	return nil
}

func validateSalesforceConnectivity(tokens map[string]string) error {
	alias := strings.TrimSpace(tokens["SF_ALIAS"])
	if alias == "" {
		return fmt.Errorf("SF_ALIAS is empty")
	}

	cmd := exec.Command("sf", "org", "display", "-o", alias, "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("salesforce connectivity check failed: %w :: %s", err, truncate(string(output), 220))
	}

	var payload struct {
		Result struct {
			InstanceURL string `json:"instanceUrl"`
			Alias       string `json:"alias"`
		} `json:"result"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return fmt.Errorf("salesforce output could not be parsed: %w", err)
	}
	if payload.Result.InstanceURL == "" && payload.Status == "" {
		return fmt.Errorf("salesforce returned no active org metadata")
	}
	return nil
}

func validateDatabricksConnectivity(tokens map[string]string) error {
	host := strings.TrimSpace(tokens["DATABRICKS_HOST"])
	if host == "" {
		return fmt.Errorf("DATABRICKS_HOST is empty")
	}
	rawToken := strings.TrimSpace(tokens["DATABRICKS_TOKEN"])
	if rawToken == "" {
		return fmt.Errorf("DATABRICKS_TOKEN is empty")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	url := strings.TrimRight(host, "/") + "/api/2.0/clusters/list"

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 6 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("databricks connectivity failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body := &bytes.Buffer{}
		_, _ = io.Copy(body, resp.Body)
		return fmt.Errorf("databricks API returned %d: %s", resp.StatusCode, truncate(body.String(), 160))
	}
	return nil
}

func validateAWSConnectivity(tokens map[string]string) error {
	profile := strings.TrimSpace(tokens["AWS_PROFILE"])
	region := strings.TrimSpace(tokens["AWS_REGION"])
	agentName := strings.TrimSpace(tokens["AGENTCORE_AGENT_NAME"])
	if profile == "" {
		return fmt.Errorf("AWS_PROFILE is empty")
	}
	if region == "" {
		return fmt.Errorf("AWS_REGION is empty")
	}
	if agentName == "" {
		return fmt.Errorf("AGENTCORE_AGENT_NAME is empty")
	}

	env := os.Environ()
	env = append(env, "AWS_PROFILE="+profile, "AWS_REGION="+region)
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aws connectivity check failed: %w :: %s", err, truncate(string(output), 220))
	}
	return nil
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
