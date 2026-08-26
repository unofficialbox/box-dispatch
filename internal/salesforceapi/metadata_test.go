package salesforceapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListMetadataUsesMetadataSOAPAPI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/services/Soap/m/67.0" || r.Header.Get("SOAPAction") != "listMetadata" {
			t.Fatalf("request = %s action=%q", r.URL.Path, r.Header.Get("SOAPAction"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "<met:sessionId>token</met:sessionId>") {
			t.Fatalf("body = %s", body)
		}
		_, _ = fmt.Fprint(w, soapEnvelope(`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	result, err := (&Client{HTTP: server.Client()}).ListMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"CustomObject", "CustomField", "Layout", "ApexClass"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 4 || !result["CustomObject:Contract__c"] {
		t.Fatalf("requests=%d result=%#v", requests, result)
	}
}

func TestListMetadataAcceptsInventoryLargerThanEightMiB(t *testing.T) {
	largePadding := strings.Repeat(" ", 9<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, soapEnvelope(largePadding+`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	result, err := (&Client{HTTP: server.Client()}).ListMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"CustomObject"})
	if err != nil {
		t.Fatal(err)
	}
	if !result["CustomObject:Contract__c"] {
		t.Fatalf("result=%#v", result)
	}
}

func TestListMetadataReportsEveryCompletedType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, soapEnvelope(`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	updates := map[string][]string{}
	_, err := (&Client{HTTP: server.Client()}).ListMetadataWithProgress(
		context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0",
		[]string{"CustomObject", "CustomField"},
		func(metadataType string, fullNames []string) { updates[metadataType] = fullNames },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || len(updates["CustomObject"]) != 1 || updates["CustomObject"][0] != "Contract__c" {
		t.Fatalf("updates = %#v", updates)
	}
	if customFields, ok := updates["CustomField"]; !ok || len(customFields) != 0 {
		t.Fatalf("empty type was not reported: %#v", updates)
	}
}

func TestListMetadataTreatsNetworkAsMissingWhenDigitalExperiencesIsDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "<met:type>Network</met:type>") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, soapEnvelope(`<soapenv:Fault xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><faultcode>sf:INVALID_TYPE</faultcode><faultstring>INVALID_TYPE: Cannot use: Network in this organization</faultstring></soapenv:Fault>`))
			return
		}
		_, _ = fmt.Fprint(w, soapEnvelope(`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	updates := map[string][]string{}
	result, err := (&Client{HTTP: server.Client()}).ListMetadataWithProgress(
		context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0",
		[]string{"Network", "CustomObject"},
		func(metadataType string, fullNames []string) { updates[metadataType] = fullNames },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result["CustomObject:Contract__c"] {
		t.Fatalf("result = %#v", result)
	}
	if networks, ok := updates["Network"]; !ok || len(networks) != 0 {
		t.Fatalf("Network was not reported as missing: %#v", updates)
	}
}

func TestListMetadataPreservesUnexpectedNetworkErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, soapEnvelope(`<soapenv:Fault xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><faultcode>sf:UNKNOWN_EXCEPTION</faultcode><faultstring>Salesforce is unavailable</faultstring></soapenv:Fault>`))
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client()}).ListMetadata(
		context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"Network"},
	)
	if err == nil || !strings.Contains(err.Error(), "Salesforce is unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestListMetadataRetriesIncompleteSOAPResponse(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><soapenv:Envelope`)
			return
		}
		_, _ = fmt.Fprint(w, soapEnvelope(`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	result, err := (&Client{HTTP: server.Client()}).ListMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"CustomObject"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || !result["CustomObject:Contract__c"] {
		t.Fatalf("requests=%d result=%#v", requests, result)
	}
}

func TestListMetadataPreservesPOSTWhenMetadataEndpointRedirects(t *testing.T) {
	redirected := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services/Soap/m/67.0" {
			http.Redirect(w, r, "/metadata", http.StatusFound)
			return
		}
		redirected++
		if r.Method != http.MethodPost || r.Header.Get("SOAPAction") != "listMetadata" {
			t.Fatalf("redirected request = %s %q", r.Method, r.Header.Get("SOAPAction"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "<met:listMetadata>") {
			t.Fatalf("redirected body = %s", body)
		}
		_, _ = fmt.Fprint(w, soapEnvelope(`<listMetadataResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><fullName>Contract__c</fullName><type>CustomObject</type></result></listMetadataResponse>`))
	}))
	defer server.Close()

	result, err := (&Client{HTTP: server.Client()}).ListMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"CustomObject"})
	if err != nil {
		t.Fatal(err)
	}
	if redirected != 1 || !result["CustomObject:Contract__c"] {
		t.Fatalf("redirected=%d result=%#v", redirected, result)
	}
}

func TestDeployMetadataReportsComponentProgress(t *testing.T) {
	checks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.Header.Get("SOAPAction") {
		case "deploy":
			if !strings.Contains(string(body), "<met:ZipFile>") || !strings.Contains(string(body), "<met:singlePackage>true") {
				t.Fatalf("deploy body = %s", body)
			}
			_, _ = fmt.Fprint(w, soapEnvelope(`<deployResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>0Af123</id><state>Queued</state></result></deployResponse>`))
		case "checkDeployStatus":
			checks++
			done, success, status, deployed := "false", "false", "InProgress", 1
			if checks > 1 {
				done, success, status, deployed = "true", "true", "Succeeded", 2
			}
			_, _ = fmt.Fprintf(w, soapEnvelope(`<checkDeployStatusResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>0Af123</id><done>%s</done><success>%s</success><status>%s</status><numberComponentsDeployed>%d</numberComponentsDeployed><numberComponentsTotal>2</numberComponentsTotal></result></checkDeployStatusResponse>`), done, success, status, deployed)
		default:
			t.Fatalf("unexpected SOAP action %q", r.Header.Get("SOAPAction"))
		}
	}))
	defer server.Close()

	updates := []MetadataProgress{}
	result, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).DeployMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []byte("zip"), func(update MetadataProgress) {
		updates = append(updates, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.ComponentsDeployed != 2 || len(updates) != 3 {
		t.Fatalf("result=%#v updates=%#v", result, updates)
	}
}

func TestDeployMetadataSurfacesComponentFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("SOAPAction") {
		case "deploy":
			_, _ = fmt.Fprint(w, soapEnvelope(`<deployResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>0Af123</id><state>Queued</state></result></deployResponse>`))
		case "checkDeployStatus":
			_, _ = fmt.Fprint(w, soapEnvelope(`<checkDeployStatusResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>0Af123</id><done>true</done><success>false</success><status>Failed</status><details><componentFailures><fullName>Contract__c</fullName><problem>Invalid field</problem></componentFailures></details></result></checkDeployStatusResponse>`))
		}
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).DeployMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []byte("zip"), nil)
	if err == nil || !strings.Contains(err.Error(), "Contract__c: Invalid field") {
		t.Fatalf("err = %v", err)
	}
}

func TestRetrieveMetadataBuildsManifestAndReturnsPackage(t *testing.T) {
	checks := 0
	wantZip := []byte("retrieved metadata zip")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.Header.Get("SOAPAction") {
		case "retrieve":
			request := string(body)
			for _, value := range []string{
				"<met:apiVersion>67.0</met:apiVersion>",
				"<met:members>Contract__c.Amount__c</met:members>",
				"<met:name>CustomField</met:name>",
				"<met:members>Communities</met:members>",
				"<met:name>Settings</met:name>",
			} {
				if !strings.Contains(request, value) {
					t.Fatalf("retrieve request missing %s: %s", value, request)
				}
			}
			_, _ = fmt.Fprint(w, soapEnvelope(`<retrieveResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>09S123</id><state>Queued</state></result></retrieveResponse>`))
		case "checkRetrieveStatus":
			checks++
			if checks == 1 {
				_, _ = fmt.Fprint(w, soapEnvelope(`<checkRetrieveStatusResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>09S123</id><done>false</done><success>false</success><status>InProgress</status></result></checkRetrieveStatusResponse>`))
				return
			}
			_, _ = fmt.Fprintf(w, soapEnvelope(`<checkRetrieveStatusResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>09S123</id><done>true</done><success>true</success><status>Succeeded</status><zipFile>%s</zipFile></result></checkRetrieveStatusResponse>`), base64.StdEncoding.EncodeToString(wantZip))
		default:
			t.Fatalf("unexpected SOAP action %q", r.Header.Get("SOAPAction"))
		}
	}))
	defer server.Close()

	updates := []MetadataRetrieveProgress{}
	got, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).RetrieveMetadata(
		context.Background(),
		Credential{InstanceURL: server.URL, AccessToken: "token"},
		"67.0",
		[]string{"Settings:Communities", "CustomField:Contract__c.Amount__c"},
		func(update MetadataRetrieveProgress) { updates = append(updates, update) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(wantZip) || len(updates) != 3 || !updates[len(updates)-1].Done {
		t.Fatalf("zip=%q updates=%#v", got, updates)
	}
}

func TestRetrieveMetadataSurfacesFailureMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("SOAPAction") {
		case "retrieve":
			_, _ = fmt.Fprint(w, soapEnvelope(`<retrieveResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>09S123</id><state>Queued</state></result></retrieveResponse>`))
		case "checkRetrieveStatus":
			_, _ = fmt.Fprint(w, soapEnvelope(`<checkRetrieveStatusResponse xmlns="http://soap.sforce.com/2006/04/metadata"><result><id>09S123</id><done>true</done><success>false</success><status>Failed</status><messages><fileName>package.xml</fileName><problem>Unknown component</problem></messages></result></checkRetrieveStatusResponse>`))
		}
	}))
	defer server.Close()

	_, err := (&Client{HTTP: server.Client(), PollInterval: time.Millisecond}).RetrieveMetadata(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", []string{"CustomObject:Missing__c"}, nil)
	if err == nil || !strings.Contains(err.Error(), "package.xml: Unknown component") {
		t.Fatalf("err = %v", err)
	}
}

func soapEnvelope(body string) string {
	return `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}
