package salesforceapi

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxApexSOAPResponseBytes = 1 << 20

// ExecuteAnonymous runs an idempotent package-owned Apex seed script through
// the Salesforce Apex SOAP API. Credentials remain inside the Go process.
func (c *Client) ExecuteAnonymous(ctx context.Context, credential Credential, apiVersion, source string) error {
	if !credential.Valid() {
		return fmt.Errorf("Salesforce connection is incomplete")
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("Salesforce Apex source is empty")
	}
	apiVersion, err := c.metadataVersion(ctx, credential, apiVersion)
	if err != nil {
		return err
	}
	envelope := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:apex="http://soap.sforce.com/2006/08/apex">` +
		`<soapenv:Header><apex:SessionHeader><apex:sessionId>` + xmlEscape(credential.AccessToken) + `</apex:sessionId></apex:SessionHeader></soapenv:Header>` +
		`<soapenv:Body><apex:executeAnonymous><apex:String>` + xmlEscape(source) + `</apex:String></apex:executeAnonymous></soapenv:Body></soapenv:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(credential.InstanceURL, "/")+"/services/Soap/s/"+apiVersion, strings.NewReader(envelope))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("SOAPAction", "executeAnonymous")
	response, err := c.jsonClient(http.MethodPost).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxApexSOAPResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxApexSOAPResponseBytes {
		return fmt.Errorf("Salesforce Apex API response exceeds 1 MiB")
	}
	var result struct {
		Body struct {
			Response struct {
				Result struct {
					Column              int    `xml:"column"`
					Compiled            bool   `xml:"compiled"`
					CompileProblem      string `xml:"compileProblem"`
					ExceptionMessage    string `xml:"exceptionMessage"`
					ExceptionStackTrace string `xml:"exceptionStackTrace"`
					Line                int    `xml:"line"`
					Success             bool   `xml:"success"`
				} `xml:"result"`
			} `xml:"executeAnonymousResponse"`
			Fault struct {
				Code   string `xml:"faultcode"`
				String string `xml:"faultstring"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse Salesforce Apex response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || strings.TrimSpace(result.Body.Fault.String) != "" {
		message := strings.TrimSpace(result.Body.Fault.String)
		if message == "" {
			message = fmt.Sprintf("HTTP %d from Salesforce Apex API", response.StatusCode)
		}
		return fmt.Errorf("%s", message)
	}
	execution := result.Body.Response.Result
	if !execution.Compiled {
		return fmt.Errorf("Salesforce Apex compilation failed at line %d, column %d: %s", execution.Line, execution.Column, strings.TrimSpace(execution.CompileProblem))
	}
	if !execution.Success {
		message := strings.TrimSpace(execution.ExceptionMessage)
		if stack := strings.TrimSpace(execution.ExceptionStackTrace); stack != "" {
			message = strings.TrimSpace(message + ": " + stack)
		}
		return fmt.Errorf("Salesforce Apex execution failed at line %d: %s", execution.Line, message)
	}
	if !bytes.Contains(data, []byte("executeAnonymousResponse")) {
		return fmt.Errorf("Salesforce Apex API returned an unexpected response")
	}
	return nil
}
