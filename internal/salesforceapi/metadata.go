package salesforceapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	metadataInventoryAttempts    = 3
	maxMetadataSOAPResponseBytes = 64 << 20
)

type MetadataProgress struct {
	ID                 string
	Done               bool
	Success            bool
	Status             string
	ComponentsDeployed int
	ComponentsTotal    int
	ComponentFailures  []string
}

type MetadataProgressFunc func(MetadataProgress)

type MetadataRetrieveProgress struct {
	ID     string
	Done   bool
	Status string
}

type MetadataRetrieveProgressFunc func(MetadataRetrieveProgress)

// MetadataInventoryProgressFunc reports one completed metadata-type query. A
// type is complete even when fullNames is empty, which lets browser clients
// resolve every requested source component as present or missing while the
// larger inventory is still running.
type MetadataInventoryProgressFunc func(metadataType string, fullNames []string)

func (c *Client) ListMetadata(ctx context.Context, credential Credential, apiVersion string, metadataTypes []string) (map[string]bool, error) {
	return c.ListMetadataWithProgress(ctx, credential, apiVersion, metadataTypes, nil)
}

func (c *Client) ListMetadataWithProgress(ctx context.Context, credential Credential, apiVersion string, metadataTypes []string, progress MetadataInventoryProgressFunc) (map[string]bool, error) {
	if !credential.Valid() {
		return nil, fmt.Errorf("Salesforce connection is incomplete")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	types := append([]string(nil), metadataTypes...)
	sort.Strings(types)
	for _, metadataType := range types {
		body := "<met:queries><met:type>" + xmlEscape(metadataType) + "</met:type></met:queries>" +
			"<met:asOfVersion>" + apiVersion + "</met:asOfVersion>"
		envelope, parseErr := c.listMetadataInventory(ctx, credential, apiVersion, body)
		if parseErr != nil {
			if metadataTypeUnavailableUntilDigitalExperiencesEnabled(metadataType, parseErr) {
				if progress != nil {
					progress(metadataType, []string{})
				}
				continue
			}
			return nil, fmt.Errorf("read Salesforce metadata type %s: %w", metadataType, parseErr)
		}
		foundByType := make(map[string][]string, 1)
		for _, property := range envelope.Body.Response.Results {
			if property.Type != "" && property.FullName != "" {
				result[property.Type+":"+property.FullName] = true
				foundByType[property.Type] = append(foundByType[property.Type], property.FullName)
			}
		}
		if progress != nil {
			fullNames := append([]string(nil), foundByType[metadataType]...)
			sort.Strings(fullNames)
			progress(metadataType, fullNames)
		}
	}
	return result, nil
}

// Salesforce rejects Network inventory queries while Digital Experiences is
// disabled. For a deployment that enables Digital Experiences, that response
// means the requested Network is absent; it is not a validation failure.
func metadataTypeUnavailableUntilDigitalExperiencesEnabled(metadataType string, err error) bool {
	if metadataType != "Network" || err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_type") && strings.Contains(message, "cannot use: network")
}

func (c *Client) listMetadataInventory(ctx context.Context, credential Credential, apiVersion, body string) (metadataInventoryEnvelope, error) {
	var lastErr error
	for attempt := 1; attempt <= metadataInventoryAttempts; attempt++ {
		data, err := c.metadataSOAP(ctx, credential, apiVersion, "listMetadata", body)
		if err != nil {
			return metadataInventoryEnvelope{}, fmt.Errorf("list Salesforce metadata: %w", err)
		}
		envelope, parseErr := parseMetadataInventory(data)
		if parseErr == nil {
			return envelope, nil
		}
		lastErr = parseErr
		if !incompleteSOAPResponse(parseErr) || attempt == metadataInventoryAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return metadataInventoryEnvelope{}, ctx.Err()
		case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
		}
	}
	return metadataInventoryEnvelope{}, fmt.Errorf("parse Salesforce metadata inventory after %d attempts: %w", metadataInventoryAttempts, lastErr)
}

type metadataInventoryEnvelope struct {
	Body struct {
		Response struct {
			Results []struct {
				FullName string `xml:"fullName"`
				Type     string `xml:"type"`
			} `xml:"result"`
		} `xml:"listMetadataResponse"`
	} `xml:"Body"`
}

func parseMetadataInventory(data []byte) (metadataInventoryEnvelope, error) {
	var envelope metadataInventoryEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return metadataInventoryEnvelope{}, err
	}
	return envelope, nil
}

func incompleteSOAPResponse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected eof") || message == "eof"
}

func (c *Client) RetrieveMetadata(ctx context.Context, credential Credential, apiVersion string, components []string, progress MetadataRetrieveProgressFunc) ([]byte, error) {
	if !credential.Valid() {
		return nil, fmt.Errorf("Salesforce connection is incomplete")
	}
	if len(components) == 0 {
		return nil, fmt.Errorf("no Salesforce metadata components were selected")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return nil, err
	}
	members := map[string][]string{}
	for _, component := range components {
		metadataType, member, ok := strings.Cut(strings.TrimSpace(component), ":")
		if !ok || metadataType == "" || member == "" {
			return nil, fmt.Errorf("invalid Salesforce metadata component %q", component)
		}
		members[metadataType] = append(members[metadataType], member)
	}
	body := "<met:retrieveRequest><met:apiVersion>" + xmlEscape(apiVersion) + "</met:apiVersion><met:singlePackage>true</met:singlePackage><met:unpackaged>"
	types := make([]string, 0, len(members))
	for metadataType := range members {
		types = append(types, metadataType)
	}
	sort.Strings(types)
	for _, metadataType := range types {
		body += "<met:types>"
		values := append([]string(nil), members[metadataType]...)
		sort.Strings(values)
		for _, member := range values {
			body += "<met:members>" + xmlEscape(member) + "</met:members>"
		}
		body += "<met:name>" + xmlEscape(metadataType) + "</met:name></met:types>"
	}
	body += "<met:version>" + xmlEscape(apiVersion) + "</met:version></met:unpackaged></met:retrieveRequest>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "retrieve", body)
	if err != nil {
		return nil, fmt.Errorf("start Salesforce metadata retrieval: %w", err)
	}
	var started struct {
		Body struct {
			Response struct {
				Result struct {
					ID    string `xml:"id"`
					State string `xml:"state"`
				} `xml:"result"`
			} `xml:"retrieveResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &started); err != nil || strings.TrimSpace(started.Body.Response.Result.ID) == "" {
		return nil, fmt.Errorf("parse Salesforce metadata retrieval response")
	}
	current := MetadataRetrieveProgress{ID: started.Body.Response.Result.ID, Status: started.Body.Response.Result.State}
	if progress != nil {
		progress(current)
	}
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for Salesforce metadata retrieval: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		zipData, next, retrieveErr := c.checkMetadataRetrieve(ctx, credential, apiVersion, current.ID)
		if retrieveErr != nil {
			return nil, retrieveErr
		}
		current = next
		if progress != nil {
			progress(current)
		}
		if !current.Done {
			continue
		}
		if len(zipData) == 0 {
			return nil, fmt.Errorf("Salesforce metadata retrieval completed without a package")
		}
		return zipData, nil
	}
}

func (c *Client) checkMetadataRetrieve(ctx context.Context, credential Credential, apiVersion, id string) ([]byte, MetadataRetrieveProgress, error) {
	body := "<met:asyncProcessId>" + xmlEscape(id) + "</met:asyncProcessId><met:includeZip>true</met:includeZip>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "checkRetrieveStatus", body)
	if err != nil {
		return nil, MetadataRetrieveProgress{ID: id}, fmt.Errorf("read Salesforce metadata retrieval status: %w", err)
	}
	var envelope struct {
		Body struct {
			Response struct {
				Result struct {
					ID       string `xml:"id"`
					Done     bool   `xml:"done"`
					Status   string `xml:"status"`
					Success  bool   `xml:"success"`
					ZipFile  string `xml:"zipFile"`
					Messages []struct {
						FileName string `xml:"fileName"`
						Problem  string `xml:"problem"`
					} `xml:"messages"`
				} `xml:"result"`
			} `xml:"checkRetrieveStatusResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, MetadataRetrieveProgress{ID: id}, fmt.Errorf("parse Salesforce metadata retrieval status: %w", err)
	}
	result := envelope.Body.Response.Result
	current := MetadataRetrieveProgress{ID: firstNonEmpty(result.ID, id), Done: result.Done, Status: result.Status}
	if !result.Done {
		return nil, current, nil
	}
	if !result.Success {
		messages := []string{}
		for _, message := range result.Messages {
			problem := strings.TrimSpace(message.Problem)
			if message.FileName != "" {
				problem = message.FileName + ": " + problem
			}
			if problem != "" {
				messages = append(messages, problem)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, firstNonEmpty(result.Status, "failed"))
		}
		return nil, current, fmt.Errorf("Salesforce metadata retrieval failed: %s", strings.Join(messages, "; "))
	}
	zipData, err := base64.StdEncoding.DecodeString(strings.TrimSpace(result.ZipFile))
	if err != nil {
		return nil, current, fmt.Errorf("decode Salesforce metadata retrieval package: %w", err)
	}
	return zipData, current, nil
}

func (c *Client) DeployMetadata(ctx context.Context, credential Credential, apiVersion string, zipData []byte, progress MetadataProgressFunc) (MetadataProgress, error) {
	if !credential.Valid() {
		return MetadataProgress{}, fmt.Errorf("Salesforce connection is incomplete")
	}
	if len(zipData) == 0 {
		return MetadataProgress{}, fmt.Errorf("Salesforce metadata package is empty")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return MetadataProgress{}, err
	}
	body := "<met:ZipFile>" + base64.StdEncoding.EncodeToString(zipData) + "</met:ZipFile>" +
		"<met:DeployOptions><met:allowMissingFiles>false</met:allowMissingFiles><met:autoUpdatePackage>false</met:autoUpdatePackage>" +
		"<met:checkOnly>false</met:checkOnly><met:ignoreWarnings>false</met:ignoreWarnings><met:performRetrieve>false</met:performRetrieve>" +
		"<met:purgeOnDelete>false</met:purgeOnDelete><met:rollbackOnError>true</met:rollbackOnError><met:singlePackage>true</met:singlePackage>" +
		"<met:testLevel>NoTestRun</met:testLevel></met:DeployOptions>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "deploy", body)
	if err != nil {
		return MetadataProgress{}, fmt.Errorf("start Salesforce metadata deployment: %w", err)
	}
	var started struct {
		Body struct {
			Response struct {
				Result struct {
					ID    string `xml:"id"`
					State string `xml:"state"`
				} `xml:"result"`
			} `xml:"deployResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &started); err != nil || started.Body.Response.Result.ID == "" {
		return MetadataProgress{}, fmt.Errorf("parse Salesforce metadata deployment response")
	}
	current := MetadataProgress{ID: started.Body.Response.Result.ID, Status: started.Body.Response.Result.State}
	if progress != nil {
		progress(current)
	}
	for {
		select {
		case <-ctx.Done():
			return current, fmt.Errorf("wait for Salesforce metadata deployment: %w", ctx.Err())
		case <-time.After(c.pollInterval()):
		}
		current, err = c.checkMetadataDeploy(ctx, credential, apiVersion, current.ID)
		if err != nil {
			return current, err
		}
		if progress != nil {
			progress(current)
		}
		if !current.Done {
			continue
		}
		if !current.Success {
			detail := strings.Join(current.ComponentFailures, "; ")
			if detail == "" {
				detail = current.Status
			}
			return current, fmt.Errorf("Salesforce metadata deployment failed: %s", detail)
		}
		return current, nil
	}
}

func (c *Client) checkMetadataDeploy(ctx context.Context, credential Credential, apiVersion, id string) (MetadataProgress, error) {
	body := "<met:asyncProcessId>" + id + "</met:asyncProcessId><met:includeDetails>true</met:includeDetails>"
	data, err := c.metadataSOAP(ctx, credential, apiVersion, "checkDeployStatus", body)
	if err != nil {
		return MetadataProgress{ID: id}, fmt.Errorf("read Salesforce metadata deployment status: %w", err)
	}
	var envelope struct {
		Body struct {
			Response struct {
				Result struct {
					ID       string `xml:"id"`
					Done     bool   `xml:"done"`
					Success  bool   `xml:"success"`
					Status   string `xml:"status"`
					Deployed int    `xml:"numberComponentsDeployed"`
					Total    int    `xml:"numberComponentsTotal"`
					Details  struct {
						Failures []struct {
							FullName string `xml:"fullName"`
							Problem  string `xml:"problem"`
						} `xml:"componentFailures"`
					} `xml:"details"`
				} `xml:"result"`
			} `xml:"checkDeployStatusResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return MetadataProgress{ID: id}, fmt.Errorf("parse Salesforce metadata deployment status: %w", err)
	}
	result := envelope.Body.Response.Result
	current := MetadataProgress{ID: firstNonEmpty(result.ID, id), Done: result.Done, Success: result.Success, Status: result.Status, ComponentsDeployed: result.Deployed, ComponentsTotal: result.Total}
	for _, failure := range result.Details.Failures {
		message := strings.TrimSpace(failure.Problem)
		if failure.FullName != "" {
			message = failure.FullName + ": " + message
		}
		if message != "" {
			current.ComponentFailures = append(current.ComponentFailures, message)
		}
	}
	return current, nil
}

func (c *Client) metadataVersion(ctx context.Context, credential Credential, version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version != "" {
		return version, nil
	}
	latest, err := c.latestAPIVersion(ctx, credential)
	return strings.TrimPrefix(latest, "v"), err
}

func (c *Client) metadataSOAP(ctx context.Context, credential Credential, apiVersion, operation, operationBody string) ([]byte, error) {
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:met="http://soap.sforce.com/2006/04/metadata">` +
		`<soapenv:Header><met:SessionHeader><met:sessionId>` + xmlEscape(credential.AccessToken) + `</met:sessionId></met:SessionHeader></soapenv:Header>` +
		`<soapenv:Body><met:` + operation + `>` + operationBody + `</met:` + operation + `></soapenv:Body></soapenv:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(credential.InstanceURL, "/")+"/services/Soap/m/"+apiVersion, strings.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("SOAPAction", operation)
	// Preserve the POST body when Salesforce redirects a Metadata API request.
	// Go's default 301/302 handling changes POST to GET, which can turn a valid
	// SOAP call into an empty 200 response and surface as an XML EOF.
	response, err := c.jsonClient(http.MethodPost).Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := readMetadataSOAPResponse(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || bytes.Contains(data, []byte("<faultcode>")) {
		var fault struct {
			Body struct {
				Fault struct {
					Code   string `xml:"faultcode"`
					String string `xml:"faultstring"`
				} `xml:"Fault"`
			} `xml:"Body"`
		}
		_ = xml.Unmarshal(data, &fault)
		message := strings.TrimSpace(fault.Body.Fault.String)
		if message == "" {
			message = fmt.Sprintf("HTTP %d from Salesforce Metadata API", response.StatusCode)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return data, nil
}

func readMetadataSOAPResponse(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxMetadataSOAPResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMetadataSOAPResponseBytes {
		return nil, fmt.Errorf("Salesforce Metadata API response exceeds %d MiB", maxMetadataSOAPResponseBytes>>20)
	}
	return data, nil
}

func xmlEscape(value string) string {
	var output strings.Builder
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}
