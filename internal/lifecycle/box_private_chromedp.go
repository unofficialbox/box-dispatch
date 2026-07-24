package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// The CDP transport attaches to a Chrome the operator already has running and
// signed in to Box, so no Box credentials are ever stored or replayed. Chrome
// must expose the DevTools endpoint:
//
//	Google\ Chrome --remote-debugging-port=9222
//
// This works identically on macOS, Windows and Linux, does not steal window
// focus, and waits on the page directly. It runs the same injected script, so
// the dependency on Box's internal application API is unchanged.
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
// DevTools HTTP endpoint. A failure here means Chrome is not listening.
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

// chromeUnavailableMessage explains how to expose the DevTools endpoint, since
// a normally launched Chrome does not listen for CDP.
func chromeUnavailableMessage() string {
	return fmt.Sprintf(
		"could not attach to Chrome at %s.\nStart Chrome with remote debugging and sign in to Box, then retry:\n  macOS:   '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' --remote-debugging-port=9222\n  Windows: chrome.exe --remote-debugging-port=9222\n  Linux:   google-chrome --remote-debugging-port=9222\nSet %s to use a different endpoint.",
		cdpEndpoint(), cdpEndpointEnv)
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

// boxTabSession is an attached browser tab sitting on the tenant Box host.
type boxTabSession struct {
	ctx      context.Context
	launched bool
	cancels  []context.CancelFunc
}

func (s *boxTabSession) close() {
	for i := len(s.cancels) - 1; i >= 0; i-- {
		s.cancels[i]()
	}
}

// openBoxTab attaches to a listening browser, starting one if necessary, and
// returns a context bound to a tab on the tenant Box host. It reports whether
// the browser had to be launched, which tells the caller a fresh profile may not
// be signed in yet.
func openBoxTab(ctx context.Context) (*boxTabSession, error) {
	session := &boxTabSession{}
	endpoint := cdpEndpoint()
	wsURL, err := cdpWebSocketURL(ctx, endpoint)
	if err != nil {
		if launchErr := launchChrome(ctx, endpoint); launchErr != nil {
			return nil, fmt.Errorf("%s\n%s", chromeUnavailableMessage(), launchErr)
		}
		session.launched = true
		wsURL, err = cdpWebSocketURL(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("%s\n%s", chromeUnavailableMessage(), err)
		}
	}

	allocatorCtx, cancelAllocator := chromedp.NewRemoteAllocator(ctx, wsURL)
	session.cancels = append(session.cancels, cancelAllocator)

	browserCtx, cancelBrowser := chromedp.NewContext(allocatorCtx)
	session.cancels = append(session.cancels, cancelBrowser)

	// A browser we just started has not registered its opening tab yet, so give
	// it a moment before concluding there is no Box tab and opening another.
	attempts := 1
	if session.launched {
		attempts = 20
	}
	var infos []*target.Info
	var targets []*chromedpTarget
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(250 * time.Millisecond)
		}
		infos, err = chromedp.Targets(browserCtx)
		if err != nil {
			session.close()
			return nil, fmt.Errorf("list Chrome tabs: %w", err)
		}
		targets = targets[:0]
		for _, info := range infos {
			targets = append(targets, &chromedpTarget{ID: info.TargetID.String(), Type: info.Type, URL: info.URL})
		}
		if _, pickErr := pickBoxTarget(targets); pickErr == nil {
			break
		}
	}

	if picked, pickErr := pickBoxTarget(targets); pickErr == nil {
		for _, info := range infos {
			if info.TargetID.String() == picked {
				tabCtx, cancelTab := chromedp.NewContext(allocatorCtx, chromedp.WithTargetID(info.TargetID))
				session.cancels = append(session.cancels, cancelTab)
				session.ctx = tabCtx
				return session, nil
			}
		}
	}

	// No Box tab yet: open one on the tenant host the adapter requires.
	target := enterpriseBoxURL(ctx)
	if !strings.Contains(target, boxTabHost) {
		session.close()
		return nil, fmt.Errorf("could not determine the enterprise Box host; sign in with the Box CLI first")
	}
	tabCtx, cancelTab := chromedp.NewContext(allocatorCtx)
	session.cancels = append(session.cancels, cancelTab)
	if navErr := chromedp.Run(tabCtx, chromedp.Navigate(target)); navErr != nil {
		session.close()
		return nil, fmt.Errorf("open %s in the attached browser: %w", target, navErr)
	}
	session.ctx = tabCtx
	return session, nil
}

// ensureBoxPrivateSession verifies a browser is attached and actually signed in
// to Box, so a deploy or reset fails before doing any work rather than part way
// through. An unauthenticated tab lands on the login host instead of the tenant
// host, which is the signal used here.
func ensureBoxPrivateSession() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session, err := openBoxTab(ctx)
	if err != nil {
		return err
	}
	defer session.close()

	host, err := tabHostname(session.ctx)
	if err != nil {
		return fmt.Errorf("check the Box browser session: %w", err)
	}
	if strings.HasSuffix(host, boxTabHost) {
		return nil
	}
	where := "is on " + host
	if strings.TrimSpace(host) == "" {
		where = "has not finished loading Box"
	}
	message := fmt.Sprintf("not signed in to Box: the browser %s instead of a %s tab.\nSign in to Box in the box-dispatch browser window, then retry", where, boxTabHost)
	if session.launched {
		message += "\n(a browser was just started with a new profile, so it has no Box session yet)"
	}
	return fmt.Errorf("%s", message)
}

// tabHostname reads the tab's hostname, waiting for a freshly opened tab to
// finish navigating. Without the wait a tab is judged while it is still
// about:blank and reports an empty host.
func tabHostname(tabCtx context.Context) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	for {
		var host string
		if err := chromedp.Run(tabCtx, chromedp.Evaluate("location.hostname", &host)); err != nil {
			return "", err
		}
		if strings.TrimSpace(host) != "" {
			return host, nil
		}
		if time.Now().After(deadline) {
			return "", nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// executeBoxPrivateBrowser runs the private-surface script in an already
// authenticated Box tab over the Chrome DevTools Protocol. This is the only
// transport: it works identically on macOS, Windows and Linux.
func executeBoxPrivateBrowser(request boxPrivateRequest) (boxPrivateResponse, error) {
	var response boxPrivateResponse
	payload, err := json.Marshal(request)
	if err != nil {
		return response, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	session, err := openBoxTab(ctx)
	if err != nil {
		return response, err
	}
	defer session.close()
	tabCtx, launched := session.ctx, session.launched

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
			return response, withSignInHint(fmt.Errorf("%s", firstNonEmpty(response.Error, "private Box API request failed")), launched)
		}
		return response, nil
	}
	return response, withSignInHint(fmt.Errorf("timed out waiting for the attached Box tab"), launched)
}

// withSignInHint explains the likely cause when box-dispatch had to start the
// browser itself: a freshly created profile has no Box session yet.
func withSignInHint(err error, launched bool) error {
	if !launched {
		return err
	}
	return fmt.Errorf("%w\nA browser was just started with a new profile. Sign in to Box in that window, then run this again", err)
}
