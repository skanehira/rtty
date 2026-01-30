package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/shlex"
)

func OpenBrowser(url string) {
	args := []string{}
	switch runtime.GOOS {
	case "windows":
		r := strings.NewReplacer("&", "^&")
		args = []string{"cmd", "start", "/", r.Replace(url)}
	case "linux":
		args = []string{"xdg-open", url}
	case "darwin":
		args = []string{"open", url}
	}

	//nolint:gosec // command is OS-specific helper
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		log.Printf("%s: %s\n", out, err)
	}
}

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}
	return def
}

func parseCommand(command string) ([]string, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("command is empty")
	}
	parts, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("command is empty")
	}
	return parts, nil
}

func windowSize(msg any) (uint16, uint16, error) {
	data, ok := msg.(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("invalid message: %#+v", msg)
	}

	rows, ok := data["rows"].(float64)
	if !ok {
		return 0, 0, fmt.Errorf("invalid rows: %#+v", data["rows"])
	}

	cols, ok := data["cols"].(float64)
	if !ok {
		return 0, 0, fmt.Errorf("invalid cols: %#+v", data["cols"])
	}

	return uint16(rows), uint16(cols), nil
}
