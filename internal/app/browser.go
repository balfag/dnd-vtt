package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// openURL opens url in the default browser without waiting for it to exit.
func openURL(url string) error {
	name, args := "xdg-open", []string{url}
	switch runtime.GOOS {
	case "windows":
		// Not "cmd /c start": it would treat "&" in a URL as a command separator.
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		name = "open"
	}
	// The browser must outlive the server start, so there is nothing to cancel.
	// url is built by announce from localhost, a port and a base64url token;
	// it never contains user input.
	cmd := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // G204: fixed program, server-built URL
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start browser: %w", err)
	}
	// Reap the process so it does not linger as a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}
