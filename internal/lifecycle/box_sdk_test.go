package lifecycle

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
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
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
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
	sdk := &boxSDK{accessToken: "access-token", httpClient: server.Client(), uploadBaseURL: server.URL}
	fileID, err := sdk.uploadFile(context.Background(), "folder-1", source)
	if err != nil {
		t.Fatal(err)
	}
	if fileID != "file-1" {
		t.Fatalf("file ID = %q", fileID)
	}
}
