package app

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openURL opens url in the default browser without waiting for it to exit.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Not "cmd /c start": it would treat "&" in a URL as a command separator.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start browser: %w", err)
	}
	// Reap the process so it does not linger as a zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}
