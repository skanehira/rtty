package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os/exec"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/*
var static embed.FS

type InitData struct {
	WindowSize struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"window_size"`
	Cmd string `json:"cmd"`
}

func shell(ws *websocket.Conn) {
	defer ws.Close()

	data := map[string]interface{}{}
	if err := json.NewDecoder(ws).Decode(&data); err != nil {
		ws.Write([]byte(fmt.Sprintf("failed to decode json: %s\r\n", err)))
		return
	}

	// Create arbitrary command.
	c := exec.Command("zsh")

	// Start the command with a pty.
	ptmx, err := pty.Start(c)
	if err != nil {
		ws.Write([]byte(fmt.Sprintf("failed to creating pty: %s\r\n", err)))
		return
	}
	// Make sure to close the pty at the end.
	defer func() {
		_ = ptmx.Close()
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	}() // Best effort.

	pty.Setsize(ptmx, &pty.Winsize{Rows: 100, Cols: 100, X: 0, Y: 0})

	// Copy stdin to the pty and the pty to stdout.
	// NOTE: The goroutine will keep reading until the next keystroke before returning.
	go func() { _, _ = io.Copy(ptmx, ws) }()
	_, _ = io.Copy(ws, ptmx)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run websocket server",
	Run: func(cmd *cobra.Command, args []string) {
		public, err := fs.Sub(static, "public")
		if err != nil {
			log.Println(err)
			return
		}
		http.Handle("/", http.FileServer(http.FS(public)))
		http.Handle("/ws", websocket.Handler(shell))
		if err := http.ListenAndServe(":3000", nil); err != nil {
			log.Println(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
