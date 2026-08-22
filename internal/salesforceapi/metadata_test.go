package salesforceapi

import (
	"context"
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
	if requests != 2 || !result["CustomObject:Contract__c"] {
		t.Fatalf("requests=%d result=%#v", requests, result)
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

func soapEnvelope(body string) string {
	return `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}
