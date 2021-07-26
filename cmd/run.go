package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	_ "embed"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/index.html
var indexHTML string

//go:embed public/index.js
var indexJS string

// run command
var command string = getenv("SHELL", "bash")

// wait time for server start
var waitTime = 500
var checkProcInterval = 5

type Event string

const (
	EventResize  Event = "resize"
	EventSnedkey Event = "sendKey"
	EventClose   Event = "close"
)

type Message struct {
	Event Event
	Data  interface{}
}

var ptmx *os.File
var execCmd *exec.Cmd

func run(ws *websocket.Conn) {
	defer ws.Close()

	wsconn := &wsConn{
		conn: ws,
	}

	if ptmx == nil {
		var msg Message
		if err := json.NewDecoder(ws).Decode(&msg); err != nil {
			log.Println("failed to decode message:", err)
			return
		}

		rows, cols, err := windowSize(msg.Data)
		if err != nil {
			_, _ = ws.Write([]byte(fmt.Sprintf("%s\r\n", err)))
			return
		}
		winsize := &pty.Winsize{
			Rows: rows,
			Cols: cols,
		}

		c := filter(strings.Split(command, " "))
		if len(c) > 1 {
			//nolint
			execCmd = exec.Command(c[0], c[1:]...)
		} else {
			//nolint
			execCmd = exec.Command(c[0])
		}

		ptmx, err = pty.StartWithSize(execCmd, winsize)
		if err != nil {
			log.Println("failed to create pty", err)
			return
		}
	}

	// write data to process
	go func() {
		for {
			var msg Message
			if err := json.NewDecoder(ws).Decode(&msg); err != nil {
				return
			}

			if msg.Event == EventClose {
				log.Println("close websocket")
				ws.Close()
				return
			}

			if msg.Event == EventResize {
				rows, cols, err := windowSize(msg.Data)
				if err != nil {
					log.Println(err)
					return
				}

				winsize := &pty.Winsize{
					Rows: rows,
					Cols: cols,
				}

				if err := pty.Setsize(ptmx, winsize); err != nil {
					log.Println("failed to set window size:", err)
					return
				}
				continue
			}

			data, ok := msg.Data.(string)
			if !ok {
				log.Println("invalid message data:", data)
				return
			}

			_, err := ptmx.WriteString(data)
			if err != nil {
				log.Println("failed to write data to ptmx:", err)
				return
			}
		}
	}()

	_, _ = io.Copy(wsconn, ptmx)
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

		indexJS = strings.Replace(indexJS, "{port}", port, 1)
		indexJS = strings.Replace(indexJS, "{fontFamily}", font, 1)
		indexJS = strings.Replace(indexJS, "{fontSize}", fontSize, 1)

		var serverErr error
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(indexHTML))
		})
		mux.HandleFunc("/index.js", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(indexJS))
		})
		mux.Handle("/ws", websocket.Handler(run))

		server := &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		}

		go func() {
			log.Println("running command: " + command)
			log.Println("running http://localhost:" + port)

			if serverErr := server.ListenAndServe(); serverErr != nil {
				log.Println(serverErr)
			}
		}()

		// check process state
		go func() {
			ticker := time.NewTicker(time.Duration(checkProcInterval) * time.Second)
			for range ticker.C {
				if execCmd != nil {
					state, err := execCmd.Process.Wait()
					if err != nil {
						return
					}

					if state.ExitCode() != -1 {
						ptmx.Close()
						ptmx = nil
						execCmd = nil
					}
				}
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

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
		<-quit

		if ptmx != nil {
			_ = ptmx.Close()
		}
		if execCmd != nil {
			_ = execCmd.Process.Kill()
			_, _ = execCmd.Process.Wait()
		}
		if err := server.Shutdown(context.Background()); err != nil {
			log.Println("failed to shutdown server", err)
		}
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
