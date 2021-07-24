package cmd

import (
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
