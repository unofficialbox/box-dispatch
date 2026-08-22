// Package salesforceapi provides the REST-native Salesforce control plane used
// by the Dispatch web application. Credentials remain in the Go process and
// are never returned to the browser.
package salesforceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Credential struct {
	InstanceURL  string
	AccessToken  string
	ClientID     string
	ClientSecret string
}

func (c Credential) Valid() bool {
	return strings.TrimSpace(c.InstanceURL) != "" && strings.TrimSpace(c.AccessToken) != ""
}

type OrgStatus struct {
	Available bool   `json:"available"`
	OrgID     string `json:"orgId,omitempty"`
	Username  string `json:"username,omitempty"`
	Status    string `json:"status"`
}

type ScratchRequest struct {
	Alias        string
	OrgName      string
	Edition      string
	DurationDays int
	CallbackURL  string
}

type ScratchOrg struct {
	ID             string `json:"id"`
	Alias          string `json:"alias"`
	Username       string `json:"username"`
	OrgID          string `json:"orgId"`
	InstanceURL    string `json:"instanceUrl"`
	AccessToken    string `json:"-"`
	Status         string `json:"status"`
	ExpirationDate string `json:"expirationDate,omitempty"`
}

type Client struct {
	HTTP         *http.Client
	PollInterval time.Duration
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, PollInterval: time.Second}
}

func (c *Client) Check(ctx context.Context, credential Credential) (OrgStatus, error) {
	if !credential.Valid() {
		return OrgStatus{}, fmt.Errorf("Salesforce connection is incomplete")
	}
	var identity struct {
		OrganizationID    string `json:"organization_id"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := c.doJSON(ctx, credential, http.MethodGet, "/services/oauth2/userinfo", nil, &identity); err != nil {
		return OrgStatus{Available: false, Status: "Unavailable"}, fmt.Errorf("Salesforce org is unavailable: %w", err)
	}
	return OrgStatus{Available: true, OrgID: identity.OrganizationID, Username: identity.PreferredUsername, Status: "Active"}, nil
}

func (c *Client) CreateScratch(ctx context.Context, devHub Credential, request ScratchRequest) (ScratchOrg, error) {
	if !devHub.Valid() {
		return ScratchOrg{}, fmt.Errorf("connect a Salesforce Dev Hub before creating a scratch org")
	}
	if strings.TrimSpace(devHub.ClientID) == "" {
		return ScratchOrg{}, fmt.Errorf("the Salesforce Connected App client ID is required to authorize a new scratch org")
	}
	request = normalizeScratchRequest(request)
	version, err := c.latestAPIVersion(ctx, devHub)
	if err != nil {
		return ScratchOrg{}, err
	}
	payload := map[string]any{
		"Alias": request.Alias, "OrgName": request.OrgName, "Edition": request.Edition,
		"DurationDays": request.DurationDays, "ConnectedAppConsumerKey": devHub.ClientID,
		"ConnectedAppCallbackUrl": request.CallbackURL,
	}
	var created struct {
		ID      string   `json:"id"`
		Success bool     `json:"success"`
		Errors  []string `json:"errors"`
	}
	path := "/services/data/" + version + "/sobjects/ScratchOrgInfo"
	if err := c.doJSON(ctx, devHub, http.MethodPost, path, payload, &created); err != nil {
		return ScratchOrg{}, fmt.Errorf("create Salesforce scratch org: %w", err)
	}
	if !created.Success || created.ID == "" {
		return ScratchOrg{}, fmt.Errorf("Salesforce did not accept the scratch-org request: %s", strings.Join(created.Errors, "; "))
	}

	for {
		select {
		case <-ctx.Done():
			return ScratchOrg{}, fmt.Errorf("wait for Salesforce scratch org: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		info, err := c.getScratchInfo(ctx, devHub, version, created.ID)
		if err != nil {
			return ScratchOrg{}, err
		}
		switch strings.ToLower(strings.TrimSpace(info.Status)) {
		case "active":
			token, instanceURL, err := c.exchangeAuthCode(ctx, devHub, info.LoginURL, request.CallbackURL, info.AuthCode)
			if err != nil {
				return ScratchOrg{}, err
			}
			return ScratchOrg{ID: created.ID, Alias: request.Alias, Username: info.SignupUsername, OrgID: info.ScratchOrg, InstanceURL: instanceURL, AccessToken: token, Status: info.Status, ExpirationDate: info.ExpirationDate}, nil
		case "error":
			return ScratchOrg{}, fmt.Errorf("Salesforce scratch-org creation failed: %s", firstNonEmpty(info.ErrorCode, "unknown Salesforce error"))
		}
	}
}

type scratchInfo struct {
	Status         string `json:"Status"`
	SignupUsername string `json:"SignupUsername"`
	ScratchOrg     string `json:"ScratchOrg"`
	LoginURL       string `json:"LoginUrl"`
	AuthCode       string `json:"AuthCode"`
	ExpirationDate string `json:"ExpirationDate"`
	ErrorCode      string `json:"ErrorCode"`
}

func (c *Client) getScratchInfo(ctx context.Context, credential Credential, version, id string) (scratchInfo, error) {
	var info scratchInfo
	path := "/services/data/" + version + "/sobjects/ScratchOrgInfo/" + url.PathEscape(id)
	if err := c.doJSON(ctx, credential, http.MethodGet, path, nil, &info); err != nil {
		return info, fmt.Errorf("read Salesforce scratch-org status: %w", err)
	}
	return info, nil
}

func (c *Client) latestAPIVersion(ctx context.Context, credential Credential) (string, error) {
	var versions []struct {
		Version string `json:"version"`
	}
	if err := c.doJSON(ctx, credential, http.MethodGet, "/services/data/", nil, &versions); err != nil {
		return "", fmt.Errorf("discover Salesforce REST API version: %w", err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("Salesforce returned no REST API versions")
	}
	slices.SortFunc(versions, func(a, b struct {
		Version string `json:"version"`
	}) int {
		aVersion, _ := strconv.ParseFloat(a.Version, 64)
		bVersion, _ := strconv.ParseFloat(b.Version, 64)
		if aVersion < bVersion {
			return -1
		}
		if aVersion > bVersion {
			return 1
		}
		return 0
	})
	return "v" + versions[len(versions)-1].Version, nil
}

func (c *Client) exchangeAuthCode(ctx context.Context, credential Credential, loginURL, callbackURL, code string) (string, string, error) {
	if strings.TrimSpace(code) == "" {
		return "", "", fmt.Errorf("Salesforce activated the scratch org but did not return an authorization code")
	}
	base := strings.TrimRight(firstNonEmpty(loginURL, credential.InstanceURL), "/")
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {credential.ClientID}, "redirect_uri": {callbackURL}, "code": {code}}
	if credential.ClientSecret != "" {
		values.Set("client_secret", credential.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/services/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient().Do(req)
	if err != nil {
		return "", "", fmt.Errorf("authorize new Salesforce scratch org: %w", err)
	}
	defer response.Body.Close()
	var result struct {
		AccessToken string `json:"access_token"`
		InstanceURL string `json:"instance_url"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := decodeResponse(response, &result); err != nil {
		return "", "", fmt.Errorf("authorize new Salesforce scratch org: %w", err)
	}
	if result.AccessToken == "" {
		return "", "", fmt.Errorf("authorize new Salesforce scratch org: %s", firstNonEmpty(result.Description, result.Error))
	}
	return result.AccessToken, result.InstanceURL, nil
}

func (c *Client) doJSON(ctx context.Context, credential Credential, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(credential.InstanceURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeResponse(response, out)
}

func decodeResponse(response *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errors []struct {
			Message   string `json:"message"`
			ErrorCode string `json:"errorCode"`
		}
		if json.Unmarshal(data, &errors) == nil && len(errors) > 0 {
			return fmt.Errorf("%s: %s", errors[0].ErrorCode, errors[0].Message)
		}
		return fmt.Errorf("HTTP %d from Salesforce", response.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func normalizeScratchRequest(request ScratchRequest) ScratchRequest {
	request.Alias = strings.TrimSpace(request.Alias)
	if request.Alias == "" {
		request.Alias = "box-dispatch-" + time.Now().UTC().Format("20060102-150405")
	}
	request.OrgName = strings.TrimSpace(request.OrgName)
	if request.OrgName == "" {
		request.OrgName = "Box Dispatch"
	}
	request.Edition = strings.TrimSpace(request.Edition)
	if request.Edition == "" {
		request.Edition = "Developer"
	}
	if request.DurationDays < 1 || request.DurationDays > 30 {
		request.DurationDays = 30
	}
	request.CallbackURL = strings.TrimSpace(request.CallbackURL)
	if request.CallbackURL == "" {
		request.CallbackURL = "http://localhost:1717/OauthRedirect"
	}
	return request
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
func (c *Client) pollInterval() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
