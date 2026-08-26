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
	Available   bool   `json:"available"`
	OrgID       string `json:"orgId,omitempty"`
	Username    string `json:"username,omitempty"`
	Status      string `json:"status"`
	InstanceURL string `json:"instanceUrl,omitempty"`
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
	RefreshToken   string `json:"-"`
	Status         string `json:"status"`
	ExpirationDate string `json:"expirationDate,omitempty"`
}

type Client struct {
	HTTP         *http.Client
	PollInterval time.Duration
}

func NewClient() *Client {
	// CustomField and other high-cardinality Metadata API inventories routinely
	// take several minutes before Salesforce returns response headers. Keep a
	// bounded provider request while allowing the browser to report live work.
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Minute}, PollInterval: time.Second}
}

func (c *Client) Check(ctx context.Context, credential Credential) (OrgStatus, error) {
	if !credential.Valid() {
		return OrgStatus{}, fmt.Errorf("Salesforce connection is incomplete")
	}
	identity, err := c.userInfo(ctx, credential)
	if err == nil {
		return OrgStatus{Available: true, OrgID: identity.OrganizationID, Username: identity.PreferredUsername, Status: "Ready", InstanceURL: identity.instanceHost()}, nil
	}
	// Scratch-org signup tokens often omit the identity scope, so userinfo 403s
	// even when the org is live. REST versions plus Organization prove the token.
	status, apiErr := c.checkRESTAvailability(ctx, credential)
	if apiErr != nil {
		return OrgStatus{Available: false, Status: "Unavailable"}, fmt.Errorf("Salesforce org is unavailable: %w", apiErr)
	}
	return status, nil
}

func (c *Client) checkRESTAvailability(ctx context.Context, credential Credential) (OrgStatus, error) {
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return OrgStatus{}, err
	}
	status := OrgStatus{Available: true, Status: "Ready", InstanceURL: credential.InstanceURL}
	if orgID, orgErr := c.organizationID(ctx, credential, version); orgErr == nil {
		status.OrgID = orgID
	}
	// Scratch-org signup tokens commonly omit the identity scope. A fresh
	// scratch org has exactly one active System Administrator, so that unique
	// record is still a safe deployment identity. Never guess when an
	// established org has more than one candidate.
	if username, usernameErr := c.uniqueStandardUsername(ctx, credential, version); usernameErr == nil {
		status.Username = username
	}
	return status, nil
}

func (c *Client) organizationID(ctx context.Context, credential Credential, version string) (string, error) {
	var result struct {
		Records []struct {
			ID string `json:"Id"`
		} `json:"records"`
	}
	if err := c.query(ctx, credential, version, "SELECT Id FROM Organization", &result); err != nil {
		return "", err
	}
	if len(result.Records) == 0 || strings.TrimSpace(result.Records[0].ID) == "" {
		return "", fmt.Errorf("Salesforce returned no Organization record")
	}
	return result.Records[0].ID, nil
}

func (c *Client) uniqueStandardUsername(ctx context.Context, credential Credential, version string) (string, error) {
	var result struct {
		Records []struct {
			Username string `json:"Username"`
		} `json:"records"`
	}
	if err := c.query(ctx, credential, version, "SELECT Username FROM User WHERE IsActive=true AND UserType='Standard' AND Profile.Name='System Administrator' ORDER BY CreatedDate ASC LIMIT 2", &result); err != nil {
		return "", err
	}
	if len(result.Records) != 1 || strings.TrimSpace(result.Records[0].Username) == "" {
		return "", fmt.Errorf("Salesforce did not return exactly one active System Administrator")
	}
	return strings.TrimSpace(result.Records[0].Username), nil
}

func (c *Client) CreateScratch(ctx context.Context, devHub Credential, request ScratchRequest) (ScratchOrg, error) {
	if !devHub.Valid() {
		return ScratchOrg{}, fmt.Errorf("connect a Salesforce Dev Hub before creating a scratch org")
	}
	request = normalizeScratchRequest(request)
	devHub = c.resolveCredential(ctx, devHub)
	resolvedDevHub, version, err := c.resolveAPIVersion(ctx, devHub)
	if err != nil {
		return ScratchOrg{}, err
	}
	devHub = resolvedDevHub
	payload := map[string]any{
		"OrgName": request.OrgName, "Edition": request.Edition, "DurationDays": request.DurationDays,
		"ConnectedAppConsumerKey": ScratchSignupClientID, "ConnectedAppCallbackUrl": request.CallbackURL,
	}
	var created struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Errors  []any  `json:"errors"`
	}
	api, err := c.createScratchOrgInfo(ctx, devHub, version, payload, &created)
	if err != nil {
		return ScratchOrg{}, interpretScratchCreateError(err)
	}
	if !created.Success || created.ID == "" {
		return ScratchOrg{}, fmt.Errorf("Salesforce did not accept the scratch-org request: %s", formatSalesforceErrors(created.Errors))
	}

	for {
		select {
		case <-ctx.Done():
			return ScratchOrg{}, fmt.Errorf("wait for Salesforce scratch org: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		info, err := c.getScratchInfo(ctx, devHub, api, created.ID)
		if err != nil {
			return ScratchOrg{}, err
		}
		switch strings.ToLower(strings.TrimSpace(info.Status)) {
		case "active":
			token, err := c.exchangeScratchAuthCode(ctx, info.LoginURL, request.CallbackURL, info.AuthCode)
			if err != nil {
				return ScratchOrg{}, fmt.Errorf("Salesforce created the scratch org, but Dispatch could not sign in: %w", err)
			}
			return ScratchOrg{ID: created.ID, Alias: request.Alias, Username: info.SignupUsername, OrgID: info.ScratchOrg, InstanceURL: token.InstanceURL, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Status: info.Status, ExpirationDate: info.ExpirationDate}, nil
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

func (c *Client) getScratchInfo(ctx context.Context, credential Credential, api scratchOrgAPI, id string) (scratchInfo, error) {
	var info scratchInfo
	if err := c.doJSON(ctx, credential, http.MethodGet, api.path(id), nil, &info); err != nil {
		return info, fmt.Errorf("read Salesforce scratch-org status: %w", err)
	}
	return info, nil
}

type scratchOrgAPI struct {
	version string
	tooling bool
}

func (api scratchOrgAPI) path(id string) string {
	path := "/services/data/" + api.version + "/sobjects/ScratchOrgInfo"
	if api.tooling {
		path = "/services/data/" + api.version + "/tooling/sobjects/ScratchOrgInfo"
	}
	if strings.TrimSpace(id) != "" {
		path += "/" + url.PathEscape(id)
	}
	return path
}

func (c *Client) createScratchOrgInfo(ctx context.Context, credential Credential, version string, payload any, created any) (scratchOrgAPI, error) {
	// Salesforce CLI uses the REST sObject API. Tooling often 404s even when
	// Dev Hub is enabled and the user is a System Administrator.
	candidates := []scratchOrgAPI{{version: version}, {version: version, tooling: true}}
	var lastErr error
	for _, api := range candidates {
		err := c.doJSON(ctx, credential, http.MethodPost, api.path(""), payload, created)
		if err == nil {
			return api, nil
		}
		lastErr = err
		if !missingSalesforceSchema(err) {
			return scratchOrgAPI{}, err
		}
	}
	return scratchOrgAPI{}, lastErr
}

func (c *Client) HasDevHub(ctx context.Context, credential Credential) (bool, error) {
	if !credential.Valid() {
		return false, fmt.Errorf("connect a Salesforce Dev Hub before creating a scratch org")
	}
	credential, version, err := c.resolveAPIVersion(ctx, credential)
	if err != nil {
		return false, err
	}
	flagged, read, queryErr := c.readOrganizationIsDevHub(ctx, credential, version)
	if read {
		return flagged, nil
	}
	for _, api := range []scratchOrgAPI{{version: version}, {version: version, tooling: true}} {
		if describeErr := c.doJSON(ctx, credential, http.MethodGet, api.path("")+"/describe", nil, &struct{}{}); describeErr == nil {
			return true, nil
		} else if !missingSalesforceSchema(describeErr) {
			return false, describeErr
		}
	}
	if queryErr != nil && !missingSalesforceSchema(queryErr) {
		return false, fmt.Errorf("could not confirm Salesforce Dev Hub status: %w", queryErr)
	}
	return false, nil
}

func missingSalesforceSchema(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "INVALID_FIELD") || strings.Contains(message, "NOT_FOUND")
}

func (c *Client) readOrganizationIsDevHub(ctx context.Context, credential Credential, version string) (bool, bool, error) {
	var result struct {
		Records []struct {
			IsDevHub bool `json:"IsDevHub"`
		} `json:"records"`
	}
	queryErr := c.query(ctx, credential, version, "SELECT IsDevHub FROM Organization", &result)
	if queryErr == nil && len(result.Records) > 0 {
		return result.Records[0].IsDevHub, true, nil
	}
	toolingPath := "/services/data/" + version + "/tooling/query?q=" + url.QueryEscape("SELECT IsDevHub FROM Organization")
	toolingErr := c.doJSON(ctx, credential, http.MethodGet, toolingPath, nil, &result)
	if toolingErr == nil && len(result.Records) > 0 {
		return result.Records[0].IsDevHub, true, nil
	}
	if queryErr != nil {
		return false, false, queryErr
	}
	if toolingErr != nil {
		return false, false, toolingErr
	}
	return false, false, fmt.Errorf("Salesforce returned no Organization record")
}

func interpretScratchCreateError(err error) error {
	message := err.Error()
	if strings.Contains(message, "NOT_FOUND") {
		return fmt.Errorf("create Salesforce scratch org: Salesforce could not start scratch-org signup with this login. Confirm this user can create scratch orgs in the Dev Hub: %s", message)
	}
	return fmt.Errorf("create Salesforce scratch org: %w", err)
}

type userInfo struct {
	OrganizationID    string `json:"organization_id"`
	PreferredUsername string `json:"preferred_username"`
	URLs              struct {
		Rest         string `json:"rest"`
		ToolingRest  string `json:"tooling_rest"`
		CustomDomain string `json:"custom_domain"`
	} `json:"urls"`
}

func (info userInfo) instanceHost() string {
	return firstSalesforceAPIHost(info.URLs.Rest, info.URLs.ToolingRest, info.URLs.CustomDomain)
}

func (c *Client) userInfo(ctx context.Context, credential Credential) (userInfo, error) {
	var identity userInfo
	err := c.doJSON(ctx, credential, http.MethodGet, "/services/oauth2/userinfo", nil, &identity)
	return identity, err
}

func (c *Client) resolveCredential(ctx context.Context, credential Credential) Credential {
	identity, err := c.userInfo(ctx, credential)
	if err != nil {
		return credential
	}
	if host := identity.instanceHost(); host != "" {
		credential.InstanceURL = host
	}
	return credential
}

func firstSalesforceAPIHost(values ...string) string {
	for _, raw := range values {
		host := instanceHost(raw)
		if host != "" && !isSalesforceUIHost(host) {
			return host
		}
	}
	return ""
}

func instanceHost(values ...string) string {
	for _, raw := range values {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Host == "" {
			continue
		}
		if parsed.Scheme != "https" && parsed.Scheme != "http" {
			continue
		}
		return parsed.Scheme + "://" + parsed.Host
	}
	return ""
}

func isSalesforceUIHost(raw string) bool {
	host := strings.ToLower(raw)
	return strings.Contains(host, "lightning.force.com") || strings.Contains(host, "salesforce-setup.com") || strings.Contains(host, "file.force.com") || strings.Contains(host, "visualforce.com")
}

func formatSalesforceErrors(errors []any) string {
	parts := make([]string, 0, len(errors))
	for _, item := range errors {
		switch typed := item.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				parts = append(parts, typed)
			}
		default:
			data, err := json.Marshal(typed)
			if err == nil && len(data) > 0 && string(data) != "null" {
				parts = append(parts, string(data))
			}
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) latestAPIVersion(ctx context.Context, credential Credential) (string, error) {
	_, version, err := c.resolveAPIVersion(ctx, credential)
	return version, err
}

func (c *Client) resolveAPIVersion(ctx context.Context, credential Credential) (Credential, string, error) {
	var versions []struct {
		Version string `json:"version"`
	}
	resolvedURL, err := c.doJSONRedirects(ctx, credential, http.MethodGet, "/services/data/", nil, &versions, 3)
	if err != nil {
		return credential, "", fmt.Errorf("discover Salesforce REST API version: %w", err)
	}
	if strings.TrimSpace(resolvedURL) != "" {
		credential.InstanceURL = resolvedURL
	}
	if len(versions) == 0 {
		return credential, "", fmt.Errorf("Salesforce returned no REST API versions")
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
	return credential, "v" + versions[len(versions)-1].Version, nil
}

func (c *Client) exchangeScratchAuthCode(ctx context.Context, loginURL, callbackURL, code string) (TokenResponse, error) {
	if strings.TrimSpace(code) == "" {
		return TokenResponse{}, fmt.Errorf("Salesforce activated the scratch org but did not return an authorization code")
	}
	var lastErr error
	for _, tokenURL := range scratchTokenLoginURLs(loginURL) {
		token, err := c.ExchangeAuthorizationCode(ctx, AuthorizationCodeRequest{
			LoginURL: tokenURL, ClientID: ScratchSignupClientID, RedirectURL: callbackURL, Code: code,
		})
		if err == nil {
			return token, nil
		}
		lastErr = err
	}
	return TokenResponse{}, lastErr
}

func scratchTokenLoginURLs(loginURL string) []string {
	urls := []string{strings.TrimSpace(loginURL)}
	if _, err := NormalizeLoginURL(loginURL); err != nil {
		return urls
	}
	for _, fallback := range []string{DefaultProductionLogin, DefaultSandboxLogin} {
		if !sameInstanceURL(fallback, loginURL) {
			urls = append(urls, fallback)
		}
	}
	return urls
}

func (c *Client) doJSON(ctx context.Context, credential Credential, method, path string, body any, out any) error {
	_, err := c.doJSONRedirects(ctx, credential, method, path, body, out, 3)
	return err
}

func (c *Client) doJSONRedirects(ctx context.Context, credential Credential, method, path string, body any, out any, remaining int) (string, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(credential.InstanceURL, "/")+path, reader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.jsonRequestClient().Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if remaining > 0 && isHTTPRedirect(response.StatusCode) {
		if host := instanceHost(response.Header.Get("Location")); host != "" && !sameInstanceURL(host, credential.InstanceURL) {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			credential.InstanceURL = host
			return c.doJSONRedirects(ctx, credential, method, path, body, out, remaining-1)
		}
	}
	resolvedURL := credential.InstanceURL
	if response.Request != nil && response.Request.URL != nil {
		resolvedURL = firstNonEmpty(instanceHost(response.Request.URL.String()), resolvedURL)
	}
	return resolvedURL, decodeResponse(response, out)
}

func (c *Client) jsonRequestClient() *http.Client {
	base := c.httpClient()
	cloned := *base
	cloned.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &cloned
}

func (c *Client) jsonClient(method string) *http.Client {
	base := c.httpClient()
	if !keepsRequestBodyOnRedirect(method) {
		return base
	}
	cloned := *base
	parent := cloned.CheckRedirect
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		if req.Method != via[0].Method {
			// net/http changes 301/302/303 POST redirects to GET before it
			// invokes CheckRedirect. Salesforce can redirect API endpoints, but
			// the receiving Metadata and REST endpoints still require the body.
			// Restore the original request instead of accepting an empty GET.
			req.Method = via[0].Method
			if via[0].GetBody == nil {
				return http.ErrUseLastResponse
			}
			body, err := via[0].GetBody()
			if err != nil {
				return err
			}
			req.Body = body
			req.GetBody = via[0].GetBody
			req.ContentLength = via[0].ContentLength
		}
		if parent != nil {
			return parent(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &cloned
}

func keepsRequestBodyOnRedirect(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func isHTTPRedirect(status int) bool {
	return status == http.StatusMovedPermanently || status == http.StatusFound || status == http.StatusTemporaryRedirect || status == http.StatusPermanentRedirect
}

func sameInstanceURL(left, right string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(left), "/"), strings.TrimRight(strings.TrimSpace(right), "/"))
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
		var oauth struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if json.Unmarshal(data, &oauth) == nil && (oauth.Error != "" || oauth.Description != "") {
			return fmt.Errorf("%s", firstNonEmpty(oauth.Description, oauth.Error))
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
		request.CallbackURL = ScratchSignupCallbackURL
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
