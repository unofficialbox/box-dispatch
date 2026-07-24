package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// The CDP transport attaches to a Chrome the operator already has running and
// signed in to Box, so no Box credentials are ever stored or replayed. Chrome
// must expose the DevTools endpoint:
//
//	Google\ Chrome --remote-debugging-port=9222
//
// Compared with the AppleScript transport this works on any platform, does not
// steal window focus, and waits on the page instead of polling through osascript.
// It runs the same injected script, so the dependency on Box's internal
// application API is unchanged.
const (
	defaultCDPEndpoint = "http://127.0.0.1:9222"
	cdpEndpointEnv     = "BOX_DISPATCH_CDP_URL"
	boxTabHost         = ".ent.box.com"
)

// cdpEndpoint returns the DevTools HTTP endpoint to attach to.
func cdpEndpoint() string {
	if value := strings.TrimSpace(os.Getenv(cdpEndpointEnv)); value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultCDPEndpoint
}

// cdpWebSocketURL resolves the browser-level websocket debugger URL from the
// DevTools HTTP endpoint. A failure here means Chrome is not listening, which is
// how the caller decides to fall back to another transport.
func cdpWebSocketURL(ctx context.Context, endpoint string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/json/version", nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("devtools endpoint returned %s", response.Status)
	}
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("devtools endpoint did not advertise a websocket url")
	}
	return payload.WebSocketDebuggerURL, nil
}

// boxPrivateChromeAvailable reports whether an attachable Chrome is listening.
func boxPrivateChromeAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := cdpWebSocketURL(ctx, cdpEndpoint())
	return err == nil
}

// pickBoxTarget selects the authenticated enterprise Box tab from the attached
// browser's targets.
func pickBoxTarget(targets []*chromedpTarget) (string, error) {
	for _, target := range targets {
		if target.Type == "page" && strings.Contains(target.URL, boxTabHost) {
			return target.ID, nil
		}
	}
	return "", fmt.Errorf("no authenticated %s tab is open in the attached Chrome", boxTabHost)
}

// chromedpTarget is the subset of target info pickBoxTarget needs, kept separate
// so the selection logic is testable without a browser.
type chromedpTarget struct {
	ID   string
	Type string
	URL  string
}

// executeBoxPrivateChrome runs the private-surface script in an already
// authenticated Box tab over the Chrome DevTools Protocol.
func executeBoxPrivateChrome(request boxPrivateRequest) (boxPrivateResponse, error) {
	var response boxPrivateResponse
	payload, err := json.Marshal(request)
	if err != nil {
		return response, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	wsURL, err := cdpWebSocketURL(ctx, cdpEndpoint())
	if err != nil {
		return response, fmt.Errorf("attach to Chrome at %s: %w", cdpEndpoint(), err)
	}
	allocatorCtx, cancelAllocator := chromedp.NewRemoteAllocator(ctx, wsURL)
	defer cancelAllocator()

	browserCtx, cancelBrowser := chromedp.NewContext(allocatorCtx)
	defer cancelBrowser()

	infos, err := chromedp.Targets(browserCtx)
	if err != nil {
		return response, fmt.Errorf("list Chrome tabs: %w", err)
	}
	targets := make([]*chromedpTarget, 0, len(infos))
	for _, info := range infos {
		targets = append(targets, &chromedpTarget{ID: info.TargetID.String(), Type: info.Type, URL: info.URL})
	}
	picked, err := pickBoxTarget(targets)
	if err != nil {
		return response, err
	}

	tabCtx, cancelTab := browserCtx, func() {}
	for _, info := range infos {
		if info.TargetID.String() == picked {
			tabCtx, cancelTab = chromedp.NewContext(allocatorCtx, chromedp.WithTargetID(info.TargetID))
			break
		}
	}
	defer cancelTab()

	// The script assigns its result to window.__boxDispatchPrivateResult when its
	// async work settles; ";true" gives Evaluate a serialisable return value.
	var started bool
	if err := chromedp.Run(tabCtx, chromedp.Evaluate(boxPrivateBrowserScript(string(payload))+";true", &started)); err != nil {
		return response, fmt.Errorf("run the private Box API adapter in the attached tab: %w", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		var raw string
		read := `window.__boxDispatchPrivateResult===null||window.__boxDispatchPrivateResult===undefined?"":JSON.stringify(window.__boxDispatchPrivateResult)`
		if err := chromedp.Run(tabCtx, chromedp.Evaluate(read, &raw)); err != nil {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			continue
		}
		_ = writeBoxPrivateCapture([]byte(raw))
		if !response.OK {
			return response, fmt.Errorf("%s", firstNonEmpty(response.Error, "private Box API request failed"))
		}
		return response, nil
	}
	return response, fmt.Errorf("timed out waiting for the attached Box tab")
}
