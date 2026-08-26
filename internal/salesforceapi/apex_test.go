package salesforceapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteAnonymousUsesApexSOAPAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/services/Soap/s/67.0" || r.Header.Get("SOAPAction") != "executeAnonymous" {
			t.Fatalf("request = %s %s action=%q", r.Method, r.URL.Path, r.Header.Get("SOAPAction"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "<apex:sessionId>token</apex:sessionId>") || !strings.Contains(string(body), "System.debug(&#39;seed&#39;);") {
			t.Fatalf("body = %s", body)
		}
		_, _ = fmt.Fprint(w, apexSOAPEnvelope(`<executeAnonymousResponse xmlns="http://soap.sforce.com/2006/08/apex"><result><column>1</column><compiled>true</compiled><line>1</line><success>true</success></result></executeAnonymousResponse>`))
	}))
	defer server.Close()

	err := (&Client{HTTP: server.Client()}).ExecuteAnonymous(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", "System.debug('seed');")
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteAnonymousReportsCompileAndRuntimeFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "compile", body: `<result><column>7</column><compiled>false</compiled><compileProblem>Unexpected token</compileProblem><line>3</line><success>false</success></result>`, want: "compilation failed at line 3, column 7: Unexpected token"},
		{name: "runtime", body: `<result><column>1</column><compiled>true</compiled><exceptionMessage>DML failed</exceptionMessage><exceptionStackTrace>Class.Seed: line 9</exceptionStackTrace><line>9</line><success>false</success></result>`, want: "execution failed at line 9: DML failed: Class.Seed: line 9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, apexSOAPEnvelope(`<executeAnonymousResponse xmlns="http://soap.sforce.com/2006/08/apex">`+test.body+`</executeAnonymousResponse>`))
			}))
			defer server.Close()
			err := (&Client{HTTP: server.Client()}).ExecuteAnonymous(context.Background(), Credential{InstanceURL: server.URL, AccessToken: "token"}, "67.0", "bad source")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
		})
	}
}

func apexSOAPEnvelope(body string) string {
	return `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}
