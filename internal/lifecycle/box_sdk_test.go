package lifecycle

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-open-go-sdk/auth"
	boxclient "github.com/unofficialbox/box-open-go-sdk/client"
	"github.com/unofficialbox/box-open-go-sdk/gantryruntime"
	"github.com/unofficialbox/box-open-go-sdk/schemas"
)

func TestBoxOAuthSDKUsesAlreadyRefreshedAccessToken(t *testing.T) {
	source, err := boxSDKTokenSource(boxconn.AuthConfig{Method: boxconn.AuthOAuth2}, "current-access-token")
	if err != nil {
		t.Fatal(err)
	}
	token, err := source.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "current-access-token" {
		t.Fatalf("token = %q, want current access token", token)
	}
}

func TestUploadedFileIDUsesUploadResponse(t *testing.T) {
	files := &schemas.Files{Entries: []schemas.FileFull{{Id: "12345"}}}
	fileID, err := uploadedFileID(files, "example.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if fileID != "12345" {
		t.Fatalf("file ID = %q, want upload response ID", fileID)
	}
	if _, err := uploadedFileID(&schemas.Files{}, "example.pdf"); err == nil {
		t.Fatal("missing upload response ID did not return an error")
	}
}

func TestResolveUploadedFileIDRetriesFolderInventory(t *testing.T) {
	attempts := 0
	fileID, err := resolveUploadedFileID(context.Background(), &schemas.Files{}, "example.pdf", 3, 0, func(context.Context) (string, bool, error) {
		attempts++
		return "67890", attempts == 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileID != "67890" || attempts != 2 {
		t.Fatalf("file ID = %q after %d attempts", fileID, attempts)
	}
}

func TestBoxUploadSendsFileBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/files/content" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		var attributes struct {
			Name   string `json:"name"`
			Parent struct {
				ID string `json:"id"`
			} `json:"parent"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("attributes")), &attributes); err != nil {
			t.Fatal(err)
		}
		if attributes.Name != "example.txt" || attributes.Parent.ID != "folder-1" {
			t.Fatalf("attributes = %#v", attributes)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		if header.Filename != "file" {
			t.Fatalf("file name = %q", header.Filename)
		}
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "real file bytes" {
			t.Fatalf("file content = %q", content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_count":1,"entries":[{"id":"file-1","type":"file","name":"example.txt"}]}`)
	}))
	defer server.Close()
	source := filepath.Join(t.TempDir(), "example.txt")
	if err := os.WriteFile(source, []byte("real file bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	sdk := &boxSDK{client: boxclient.NewClient(
		auth.DeveloperToken("access-token"),
		gantryruntime.WithBaseURL("upload", server.URL),
		gantryruntime.WithHTTPClient(server.Client()),
	)}
	fileID, err := sdk.uploadFile(context.Background(), "folder-1", source)
	if err != nil {
		t.Fatal(err)
	}
	if fileID != "file-1" {
		t.Fatalf("file ID = %q", fileID)
	}
}

func TestApplyMetadataTemplateRenamesExistingTemplate(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/metadata_templates/enterprise":
			_, _ = io.WriteString(w, `{"entries":[{"type":"metadata_template","templateKey":"clmContract","displayName":"CLM Contract"}]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/metadata_templates/enterprise/clmContract/schema":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"op":"editTemplate"`) || !strings.Contains(string(body), `"displayName":"Contract"`) {
				t.Fatalf("unexpected update body: %s", body)
			}
			_, _ = io.WriteString(w, `{"type":"metadata_template","templateKey":"clmContract","displayName":"Contract"}`)
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	sdk := &boxSDK{client: boxclient.NewClient(
		auth.DeveloperToken("access-token"),
		gantryruntime.WithBaseURL("api", server.URL),
		gantryruntime.WithHTTPClient(server.Client()),
	)}
	if err := sdk.applyMetadataTemplate(context.Background(), boxMetadataTemplate{TemplateKey: "clmContract", DisplayName: "Contract"}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want get and update", requests)
	}
}
