// Package browser opens validated web URLs with the operating system's default browser.
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenURL opens rawURL in the default browser without invoking a shell.
func OpenURL(rawURL string) error {
	name, args, err := commandForURL(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func commandForURL(goos, rawURL string) (string, []string, error) {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", nil, fmt.Errorf("invalid browser URL %q", rawURL)
	}
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "linux":
		return "xdg-open", []string{rawURL}, nil
	case "windows":
		return "rundll32.exe", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "", nil, fmt.Errorf("opening browser links is not supported on %s", goos)
	}
}
