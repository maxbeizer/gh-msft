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
	return openURL(runtime.GOOS, rawURL, func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	})
}

func openURL(goos, rawURL string, run func(string, ...string) error) error {
	name, args, err := commandForURL(goos, rawURL)
	if err != nil {
		return err
	}
	if err := run(name, args...); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
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
