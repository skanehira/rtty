package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	_ "embed"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/index.html
var indexHTML string

type InitData struct {
	WindowSize struct {
		Width  uint16 `json:"width"`
		Height uint16 `json:"height"`
	} `json:"window_size"`
	Cmd string `json:"cmd"`
}

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

	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		log.Printf("%s: %s\n", out, err)
	}
}

func run(ws *websocket.Conn) {
	defer ws.Close()

	var data InitData
	if err := json.NewDecoder(ws).Decode(&data); err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("failed to decode json: %s\r\n", err)))
		return
	}

	// Create arbitrary command.
	c := exec.Command(data.Cmd)

	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("failed to creating pty: %s\r\n", err)))
		return
	}

	// Make sure to close the pty at the end.
	defer func() {
		_ = ptmx.Close()
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	}() // Best effort.

	// Update pty window size
	winsize := &pty.Winsize{
		Rows: data.WindowSize.Height,
		Cols: data.WindowSize.Width,
		X:    0,
		Y:    0,
	}
	if err := pty.Setsize(ptmx, winsize); err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("failed to set pty window size: %s", err)))
		return
	}

	go func() {
		_, _ = io.Copy(ptmx, ws)
	}()
	_, _ = io.Copy(ws, ptmx)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run server",
	Run: func(cmd *cobra.Command, args []string) {
		port, err := cmd.PersistentFlags().GetString("port")
		if err != nil {
			log.Println(err)
			return
		}
		html := strings.Replace(indexHTML, "{port}", port, 1)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(html))
		})
		http.Handle("/ws", websocket.Handler(run))

		log.Println("start server with port: " + port)

		OpenBrowser("http://localhost:" + port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Println(err)
		}
	},
}

func init() {
	runCmd.PersistentFlags().StringP("port", "p", "9999", "server port")
	rootCmd.AddCommand(runCmd)
}
