package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "embed"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/index.html
var indexHTML string

// run command
var command string = getenv("SHELL", "bash")

// wait time for server start
var waitTime = 500

type Event string

const (
	EventResize  Event = "resize"
	EventSnedkey Event = "sendKey"
)

type Message struct {
	Event Event
	Data  interface{}
}

func run(ws *websocket.Conn) {
	defer ws.Close()

	var msg Message
	if err := json.NewDecoder(ws).Decode(&msg); err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("failed to decode json: %s\r\n", err)))
		return
	}

	rows, cols, err := windowSize(msg.Data)
	if err != nil {
		_, _ = ws.Write([]byte(fmt.Sprintf("%s\r\n", err)))
		return
	}

	// Create arbitrary command.
	cmd := filter(strings.Split(command, " "))

	var c *exec.Cmd
	if len(cmd) > 1 {
		c = exec.Command(cmd[0], cmd[1:]...)
	} else {
		c = exec.Command(cmd[0])
	}

	winsize := &pty.Winsize{
		Rows: rows,
		Cols: cols,
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

	// write data to process
	go func() {
		for {
			var msg Message
			if err := json.NewDecoder(ws).Decode(&msg); err != nil {
				_, _ = ws.Write([]byte(fmt.Sprintf("failed to creating pty: %s\r\n", err)))
				continue
			}

			if msg.Event == EventResize {
				rows, cols, err := windowSize(msg.Data)
				if err != nil {
					err := fmt.Sprintf("%s\r\n", err)
					_, _ = ws.Write([]byte(err))
					continue
				}

				winsize := &pty.Winsize{
					Rows: rows,
					Cols: cols,
				}

				if err := pty.Setsize(ptmx, winsize); err != nil {
					err := fmt.Sprintf("%s\r\n", err)
					_, _ = ws.Write([]byte(err))
				}
				continue
			}

			data, ok := msg.Data.(string)
			if !ok {
				err := fmt.Sprintf("invalid message data: %#+v\r\n", msg)
				log.Println(err)
				_, _ = ws.Write([]byte(err))
				continue
			}

			_, err := ptmx.WriteString(data)
			if err != nil {
				_, _ = ws.Write([]byte(fmt.Sprintf("failed to write data to ptmx: %s\r\n", err)))
			}
		}
	}()

	w := &wsConn{
		conn: ws,
	}

	_, _ = io.Copy(w, ptmx)
}

type wsConn struct {
	conn *websocket.Conn
	buf  []byte
}

// Checking and buffering `b`
// If `b` is invalid UTF-8, it would be buffered
// if buffer is valid UTF-8, it would write to connection
func (ws *wsConn) Write(b []byte) (i int, err error) {
	if !utf8.Valid(b) {
		buflen := len(ws.buf)
		blen := len(b)
		ws.buf = append(ws.buf, b...)[:buflen+blen]
		if utf8.Valid(ws.buf) {
			_, e := ws.conn.Write(ws.buf)
			ws.buf = ws.buf[:0]
			return blen, e
		}
		return blen, nil
	}

	if len(ws.buf) > 0 {
		n, err := ws.conn.Write(ws.buf)
		ws.buf = ws.buf[:0]
		if err != nil {
			return n, err
		}
	}
	n, e := ws.conn.Write(b)
	return n, e
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
		time.Sleep(time.Duration(waitTime) * time.Microsecond)

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
		fmt.Printf(`Run command

Usage:
  rtty run [command] [flags]

Command
  Execute specified command (default "%s")

Flags:
      --font string        font (default "")
      --font-size string   font size (default "")
  -h, --help               help for run
  -p, --port string        server port (default "9999")
  -v, --view               open browser
`, command)
	})
	rootCmd.AddCommand(runCmd)
}
