package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

// enterpriseBoxURL derives the tenant web host from the authenticated Box CLI
// user, whose avatar URL is served from it (https://<tenant>.ent.box.com/...).
// The private adapter needs that host, not the generic app.box.com.
func enterpriseBoxURL(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "box", "users:get", "me", "--json", "--fields=id,login,avatar_url").Output()
	if err != nil {
		return boxAppFallbackURL
	}
	var payload struct {
		AvatarURL string `json:"avatar_url"`
	}
	if json.Unmarshal(output, &payload) != nil {
		return boxAppFallbackURL
	}
	parsed, err := url.Parse(strings.TrimSpace(payload.AvatarURL))
	if err != nil || parsed.Host == "" || !strings.Contains(parsed.Host, boxTabHost) {
		return boxAppFallbackURL
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
