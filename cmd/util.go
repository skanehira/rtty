package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

	//nolint
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		log.Printf("%s: %s\n", out, err)
	}
}

func filter(ss []string) []string {
	rs := []string{}

	for _, s := range ss {
		if s == "" {
			continue
		}
		rs = append(rs, s)
	}

	return rs
}

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}
	return def
}

func windowSize(msg interface{}) (rows, cols uint16, err error) {
	data, ok := msg.(map[string]interface{})
	if !ok {
		return 0, 0, fmt.Errorf("invalid message: %#+v", msg)
	}

	rows = uint16(data["rows"].(float64))
	cols = uint16(data["cols"].(float64))

	return
}
