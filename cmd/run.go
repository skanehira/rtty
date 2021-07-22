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
	"sync"
	"time"

	_ "embed"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/index.html
var indexHTML string

// run command
var command string = "bash"

type InitData struct {
	WindowSize struct {
		Width  uint16 `json:"width"`
		Height uint16 `json:"height"`
	} `json:"window_size"`
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
	c := exec.Command(command)

	// Start the command with a pty.
	winsize := &pty.Winsize{
		Rows: data.WindowSize.Height,
		Cols: data.WindowSize.Width,
		X:    0,
		Y:    0,
	}
	ptmx, err := pty.StartWithSize(c, winsize)
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

	go func() {
		_, _ = io.Copy(ptmx, ws)
	}()
	_, _ = io.Copy(ws, ptmx)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run command",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			command = args[0]
		}
		port, err := cmd.PersistentFlags().GetString("port")
		if err != nil {
			log.Println(err)
			return
		}

		font, err := cmd.PersistentFlags().GetString("font")
		if err != nil {
			log.Println(err)
			return
		}
		fontSize, err := cmd.PersistentFlags().GetString("font-size")
		if err != nil {
			log.Println(err)
			return
		}

		html := strings.Replace(indexHTML, "{port}", port, 1)
		html = strings.Replace(html, "{fontFamily}", font, 1)
		html = strings.Replace(html, "{fontSize}", fontSize, 1)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(html))
		})
		http.Handle("/ws", websocket.Handler(run))

		var serverErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Println("running command: " + command)
			log.Println("running http://localhost:" + port)

			if serverErr := http.ListenAndServe(":"+port, nil); serverErr != nil {
				log.Println(serverErr)
			}
		}()

		// wait for run server
		time.Sleep(500 * time.Microsecond)

		if serverErr == nil {
			// open browser
			openView, err := cmd.PersistentFlags().GetBool("view")
			if err != nil {
				log.Println(err)
			} else if openView {
				OpenBrowser("http://localhost:" + port)
			}
		}

		wg.Wait()
	},
}

func init() {
	runCmd.PersistentFlags().StringP("port", "p", "9999", "server port")
	runCmd.PersistentFlags().String("font", "", "font")
	runCmd.PersistentFlags().String("font-size", "", "font size")
	runCmd.PersistentFlags().BoolP("view", "v", false, "open browser")
	runCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Print(`Run command

Usage:
  rtty run [command] [flags]

Command
  Execute specified command (default "bash")

Flags:
      --font string        font (default "")
      --font-size string   font size (default "")
  -h, --help               help for run
  -p, --port string        server port (default "9999")
  -v, --view               open browser
`)
	})
	rootCmd.AddCommand(runCmd)
}
