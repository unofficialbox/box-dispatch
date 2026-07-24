package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unofficialbox/box-dispatch/internal/boxconn"
	"github.com/unofficialbox/box-dispatch/internal/config"
	"github.com/unofficialbox/box-dispatch/internal/shellstate"
)

// box-dispatch launches its own Chrome rather than co-opting the operator's:
// Chrome cannot enable remote debugging on a process that is already running,
// and it refuses a --user-data-dir that a live Chrome already holds. So the
// browser gets a dedicated, persistent profile. The operator signs in to Box
// there once and every later run reuses that session.
const (
	chromeExecutableEnv = "BOX_DISPATCH_CHROME"
	chromeAutoLaunchEnv = "BOX_DISPATCH_CHROME_AUTOLAUNCH"
	boxAppFallbackURL   = "https://app.box.com"
)

// chromeAutoLaunchEnabled reports whether box-dispatch may start a browser.
func chromeAutoLaunchEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(chromeAutoLaunchEnv)))
	return value != "false" && value != "0" && value != "no"
}

// chromeCandidates lists the Chromium-family executables to try, per platform.
// Any of them speaks CDP, so Edge/Chromium/Brave work as well as Chrome.
func chromeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"google-chrome",
			"chromium",
		}
	case "windows":
		candidates := []string{}
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if strings.TrimSpace(base) == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return append(candidates, "chrome.exe", "msedge.exe")
	default:
		return []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"microsoft-edge", "brave-browser",
		}
	}
}

// chromeExecutable resolves a browser to launch, honouring an explicit override.
func chromeExecutable() (string, error) {
	if override := strings.TrimSpace(os.Getenv(chromeExecutableEnv)); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("%s is set to %q, which is not executable: %w", chromeExecutableEnv, override, err)
		}
		return override, nil
	}
	for _, candidate := range chromeCandidates() {
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("no Chrome, Chromium or Edge installation was found; set %s to the browser executable", chromeExecutableEnv)
}

// chromeProfileDir is the persistent profile the launched browser uses, so the
// Box sign-in survives between runs.
func chromeProfileDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "box-dispatch", "chrome-profile"), nil
}

// cdpPort extracts the port to expose from the configured DevTools endpoint.
func cdpPort(endpoint string) string {
	if parsed, err := url.Parse(endpoint); err == nil {
		if port := parsed.Port(); port != "" {
			return port
		}
	}
	return "9222"
}

// enterpriseBoxURL derives the tenant web host (https://<tenant>.ent.box.com)
// from the acting user's avatar URL, which is served from that host. The private
// adapter needs the tenant host, not the generic app.box.com.
//
// It reads the avatar through the connection box-dispatch actually deploys with:
// when that is the box-dispatch CCG app, a CCG token is minted and the Box API
// is queried directly. The box CLI is a separate login that can be broken,
// expired, or scoped differently — deriving the host from it is what aborted the
// whole Box provider ("could not determine the enterprise Box host") when the
// CLI environment went bad even though CCG was healthy. The CLI is used only as
// the fallback when CCG is not the active connection.
func enterpriseBoxURL(ctx context.Context) string {
	if settings, err := shellstate.LoadConnectionSettings(); err == nil && prefersBoxCCG(settings) {
		if host := enterpriseHostViaCCG(ctx, settings); host != "" {
			return host
		}
	}
	output, err := exec.CommandContext(ctx, "box", "users:get", "me", "--json", "--fields=id,login,avatar_url").Output()
	if err == nil {
		var payload struct {
			AvatarURL string `json:"avatar_url"`
		}
		if json.Unmarshal(output, &payload) == nil {
			if host := hostFromAvatarURL(payload.AvatarURL); host != "" {
				return host
			}
		}
	}
	return boxAppFallbackURL
}

// enterpriseHostViaCCG mints a CCG token and reads the acting user's avatar_url
// from the Box API, so the tenant host reflects the CCG connection rather than
// the CLI login. Returns "" on any failure so the caller can fall back.
func enterpriseHostViaCCG(ctx context.Context, settings config.ConnectionSettings) string {
	token, err := boxconn.CCGTokenFromSettings(ctx, settings)
	if err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.box.com/2.0/users/me?fields=avatar_url", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		AvatarURL string `json:"avatar_url"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return ""
	}
	return hostFromAvatarURL(payload.AvatarURL)
}

// hostFromAvatarURL extracts the tenant web origin from an avatar URL, or "" when
// it is not a tenant host.
func hostFromAvatarURL(avatarURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(avatarURL))
	if err != nil || parsed.Host == "" || !strings.Contains(parsed.Host, boxTabHost) {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// launchChrome starts a dedicated browser exposing the DevTools endpoint and
// waits for it to accept connections. The process is released so it outlives
// this command and can be reused by the next run.
func launchChrome(ctx context.Context, endpoint string) error {
	if !chromeAutoLaunchEnabled() {
		return fmt.Errorf("automatic browser launch is disabled by %s", chromeAutoLaunchEnv)
	}
	executable, err := chromeExecutable()
	if err != nil {
		return err
	}
	profile, err := chromeProfileDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	command := exec.Command(executable,
		"--remote-debugging-port="+cdpPort(endpoint),
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
		enterpriseBoxURL(ctx),
	)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start %s: %w", executable, err)
	}
	// Detach: the browser should stay up for subsequent runs.
	_ = command.Process.Release()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, probeErr := cdpWebSocketURL(probeCtx, endpoint)
		cancel()
		if probeErr == nil {
			return nil
		}
	}
	return fmt.Errorf("%s did not expose a DevTools endpoint on port %s", executable, cdpPort(endpoint))
}
