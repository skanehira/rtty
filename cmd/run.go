package cmd

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

//go:embed public/index.html
var indexHTML string

//go:embed public/index.js
var indexJS string

//go:embed public
var public embed.FS

// run command
var defaultCommand = getenv("SHELL", "bash")

type Event string

const (
	EventResize  Event = "resize"
	EventSendKey Event = "sendKey"
	EventClose   Event = "close"
)

type Message struct {
	Event Event
	Data  any
}

type session struct {
	mu            sync.Mutex
	ptmx          *os.File
	cmd           *exec.Cmd
	clients       map[*websocket.Conn]*wsConn
	readerRunning bool
	history       []string
	historyBuf    []byte
	historyLimit  int
	historyBufCap int
}

func newSession() *session {
	return &session{
		clients:       make(map[*websocket.Conn]*wsConn),
		historyLimit:  3000,
		historyBufCap: 1024 * 1024,
	}
}

func (s *session) needsStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptmx == nil
}

func (s *session) start(winsize *pty.Winsize, commandArgs []string) error {
	s.mu.Lock()
	if s.ptmx != nil {
		s.mu.Unlock()
		return nil
	}
	if len(commandArgs) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("command is empty")
	}

	//nolint:gosec // command is user-configured and intentionally executed
	s.cmd = exec.Command(commandArgs[0], commandArgs[1:]...)

	ptmx, err := pty.StartWithSize(s.cmd, winsize)
	if err != nil {
		s.cmd = nil
		s.mu.Unlock()
		return fmt.Errorf("failed to create pty: %w", err)
	}

	s.ptmx = ptmx
	s.startReaderLocked()
	cmd := s.cmd
	s.mu.Unlock()

	go s.waitProcess(cmd, ptmx)
	return nil
}

func (s *session) waitProcess(cmd *exec.Cmd, ptmx *os.File) {
	if cmd == nil {
		return
	}
	if err := cmd.Wait(); err != nil {
		log.Println("failed to wait process:", err)
	}

	s.mu.Lock()
	if s.ptmx == ptmx {
		_ = ptmx.Close()
	}
	s.mu.Unlock()
}

func (s *session) startReaderLocked() {
	if s.readerRunning || s.ptmx == nil {
		return
	}
	s.readerRunning = true
	ptmx := s.ptmx
	go s.readLoop(ptmx)
}

func (s *session) readLoop(ptmx *os.File) {
	buf := make([]byte, 32*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			s.appendHistory(buf[:n])
			s.broadcast(buf[:n])
		}
		if err != nil {
			break
		}
	}

	s.finalize(ptmx)
}

func (s *session) finalize(ptmx *os.File) {
	s.mu.Lock()
	if s.ptmx != ptmx {
		s.mu.Unlock()
		return
	}

	clients := collectClients(s.clients)
	s.clients = make(map[*websocket.Conn]*wsConn)
	s.ptmx = nil
	s.cmd = nil
	s.readerRunning = false
	s.history = nil
	s.historyBuf = nil
	s.mu.Unlock()

	for _, client := range clients {
		_ = writeToClient(client, []byte("\r\n[Session ended]\r\n"))
		_ = client.conn.Close(websocket.StatusNormalClosure, "session ended")
	}
}

func (s *session) addClient(conn *websocket.Conn, client *wsConn) {
	s.mu.Lock()
	s.clients[conn] = client
	s.mu.Unlock()
}

func (s *session) removeClient(conn *websocket.Conn) {
	s.mu.Lock()
	_, ok := s.clients[conn]
	if ok {
		delete(s.clients, conn)
	}
	s.mu.Unlock()
	_ = conn.Close(websocket.StatusNormalClosure, "closed")
}

func (s *session) broadcast(data []byte) {
	s.mu.Lock()
	clients := collectClients(s.clients)
	s.mu.Unlock()
	if len(clients) == 0 {
		return
	}
	payload := append([]byte(nil), data...)
	for _, client := range clients {
		go func(client *wsConn) {
			if err := writeToClient(client, payload); err != nil {
				s.removeClient(client.conn)
			}
		}(client)
	}
}

func (s *session) appendHistory(data []byte) {
	if s.historyLimit <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyBufCap > 0 && len(s.historyBuf) > s.historyBufCap {
		s.historyBuf = s.historyBuf[:0]
	}
	s.historyBuf = append(s.historyBuf, data...)
	if s.historyBufCap > 0 && len(s.historyBuf) > s.historyBufCap {
		s.historyBuf = s.historyBuf[len(s.historyBuf)-s.historyBufCap:]
	}
	for {
		idx := bytes.IndexByte(s.historyBuf, '\n')
		if idx == -1 {
			break
		}
		line := string(s.historyBuf[:idx+1])
		s.history = append(s.history, line)
		s.historyBuf = s.historyBuf[idx+1:]
		if len(s.history) > s.historyLimit {
			s.history = s.history[len(s.history)-s.historyLimit:]
		}
	}
}

func (s *session) sendHistory(client *wsConn) error {
	s.mu.Lock()
	history := append([]string(nil), s.history...)
	buf := append([]byte(nil), s.historyBuf...)
	s.mu.Unlock()

	if len(history) == 0 && len(buf) == 0 {
		return nil
	}

	var builder strings.Builder
	for _, line := range history {
		builder.WriteString(line)
	}
	if len(buf) > 0 {
		builder.Write(buf)
	}

	return writeToClient(client, []byte(builder.String()))
}

func collectClients(clients map[*websocket.Conn]*wsConn) []*wsConn {
	items := make([]*wsConn, 0, len(clients))
	for _, client := range clients {
		items = append(items, client)
	}
	return items
}

func writeToClient(client *wsConn, data []byte) error {
	if !utf8.Valid(data) {
		data = bytes.ToValidUTF8(data, []byte("?"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.conn.Write(ctx, websocket.MessageText, data)
}

func (s *session) resize(rows uint16, cols uint16) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("pty not running")
	}

	winsize := &pty.Winsize{Rows: rows, Cols: cols}
	if err := pty.Setsize(ptmx, winsize); err != nil {
		return fmt.Errorf("failed to set window size: %w", err)
	}
	return nil
}

func (s *session) writeToPTY(data string) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("pty not running")
	}

	if _, err := ptmx.WriteString(data); err != nil {
		return fmt.Errorf("failed to write data to ptmx: %w", err)
	}
	return nil
}

func (s *session) handleMessage(msg Message) error {
	switch msg.Event {
	case EventClose:
		log.Println("close websocket")
		return io.EOF
	case EventResize:
		rows, cols, err := windowSize(msg.Data)
		if err != nil {
			return err
		}
		return s.resize(rows, cols)
	case EventSendKey:
		data, ok := msg.Data.(string)
		if !ok {
			return fmt.Errorf("invalid message data: %v", msg.Data)
		}
		return s.writeToPTY(data)
	default:
		data, ok := msg.Data.(string)
		if !ok {
			return nil
		}
		return s.writeToPTY(data)
	}
}

func (s *session) shutdown() {
	s.mu.Lock()
	ptmx := s.ptmx
	cmd := s.cmd
	s.mu.Unlock()

	if ptmx != nil {
		_ = ptmx.Close()
	}
	if cmd != nil {
		_ = cmd.Process.Kill()
	}
}

func run(sess *session, allowOrigins []string, commandArgs []string) http.HandlerFunc {
	commandArgs = append([]string(nil), commandArgs...)
	return func(w http.ResponseWriter, r *http.Request) {
		if err := checkOrigin(allowOrigins, r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			log.Println("failed to accept websocket:", err)
			return
		}
		defer func() {
			_ = ws.Close(websocket.StatusNormalClosure, "closed")
		}()

		wsconn := &wsConn{
			conn: ws,
		}

		var first Message
		if err := wsjson.Read(r.Context(), ws, &first); err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return
			}
			log.Println("failed to decode message:", err)
			return
		}

		if sess.needsStart() {
			if first.Event != EventResize {
				log.Println("first message must be resize")
				return
			}
			rows, cols, err := windowSize(first.Data)
			if err != nil {
				_ = writeToClient(wsconn, []byte(fmt.Sprintf("%s\r\n", err)))
				return
			}
			winsize := &pty.Winsize{Rows: rows, Cols: cols}
			if err := sess.start(winsize, commandArgs); err != nil {
				log.Println(err)
				return
			}
		}

		sess.addClient(ws, wsconn)
		defer sess.removeClient(ws)
		if err := sess.sendHistory(wsconn); err != nil {
			sess.removeClient(ws)
			return
		}

		if err := sess.handleMessage(first); err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			return
		}

		for {
			var msg Message
			if err := wsjson.Read(r.Context(), ws, &msg); err != nil {
				if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
					return
				}
				return
			}
			if err := sess.handleMessage(msg); err != nil {
				if err != io.EOF {
					log.Println(err)
				}
				return
			}
		}
	}
}

type wsConn struct {
	conn *websocket.Conn
}

func checkOrigin(allowOrigins []string, req *http.Request) error {
	origin := req.Header.Get("Origin")
	if origin == "" {
		return fmt.Errorf("null origin")
	}
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" {
		return fmt.Errorf("invalid origin: %s", origin)
	}
	if slices.Contains(allowOrigins, originURL.Host) {
		return nil
	}

	msg := "not allowed origin: %s"
	log.Printf(msg, originURL.Host)
	return fmt.Errorf(msg, originURL.Host)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run command",
	Run: func(cmd *cobra.Command, args []string) {
		var commandArgs []string
		var commandDisplay string
		if len(args) > 0 {
			if len(args) == 1 {
				parsed, err := parseCommand(args[0])
				if err != nil {
					log.Println(err)
					return
				}
				commandArgs = parsed
			} else {
				commandArgs = append([]string(nil), args...)
			}
		} else {
			parsed, err := parseCommand(defaultCommand)
			if err != nil {
				log.Println(err)
				return
			}
			commandArgs = parsed
		}
		commandDisplay = strings.Join(commandArgs, " ")
		portFlag, err := cmd.PersistentFlags().GetInt("port")
		if err != nil {
			log.Println(err)
			return
		}
		port := strconv.Itoa(portFlag)

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

		waitTime, err := cmd.PersistentFlags().GetInt("wait-time")
		if err != nil {
			log.Println(err)
			return
		}

		addr, err := cmd.PersistentFlags().GetString("addr")
		if err != nil {
			log.Println(err)
			return
		}

		indexJS = strings.Replace(indexJS, "{addr}", template.JSEscapeString(addr), 1)
		indexJS = strings.Replace(indexJS, "{port}", port, 1)
		indexJS = strings.Replace(indexJS, "{fontFamily}", template.JSEscapeString(font), 1)
		indexJS = strings.Replace(indexJS, "{fontSize}", template.JSEscapeString(fontSize), 1)

		var serverErr error
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(indexHTML))
		})
		sub, err := fs.Sub(public, "public")
		if err != nil {
			return
		}
		publicHandler := http.FileServer(http.FS(sub))
		mux.Handle("/css/", publicHandler)
		mux.Handle("/js/", publicHandler)
		mux.HandleFunc("/index.js", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(indexJS))
		})

		allowedOrigins, err := cmd.PersistentFlags().GetStringArray("allow-origin")
		allowedOrigins = append(allowedOrigins, "localhost:"+port)
		if err != nil {
			log.Println(err)
		}
		sess := newSession()
		mux.Handle("/ws", run(sess, allowedOrigins, commandArgs))

		server := &http.Server{
			Addr:    addr + ":" + port,
			Handler: mux,
		}

		go func() {
			log.Println("allowed origins", allowedOrigins)
			log.Println("running command: " + commandDisplay)
			log.Printf("running http://%s:%s\n", addr, port)

			if serverErr := server.ListenAndServe(); serverErr != nil {
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
				OpenBrowser(fmt.Sprintf("http://%s:%s", addr, port))
			}
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
		go func() {
			<-quit
			signal.Stop(quit)
			close(quit)
		}()
		<-quit

		sess.shutdown()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Println("failed to shutdown server", err)
		}
	},
}

func init() {
	runCmd.PersistentFlags().IntP("port", "p", 9999, "server port")
	runCmd.PersistentFlags().StringP("addr", "a", "localhost", "server address")
	runCmd.PersistentFlags().String("font", "", "font")
	runCmd.PersistentFlags().String("font-size", "", "font size")
	runCmd.PersistentFlags().Int("wait-time", 500, "wait time before opening the browser (microseconds)")
	runCmd.PersistentFlags().BoolP("view", "v", false, "open browser")
	runCmd.PersistentFlags().StringArray("allow-origin", []string{}, "allow origin")
	runCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Printf(`Run command

Usage:
  rtty run [command] [flags]

Command
  Execute specified command (default "%s")

Flags:
  -a, --addr string                server address (default "localhost")
      --allow-origin stringArray   allow origin (default ["localhost:9999"])
      --font string                font
      --font-size string           font size
      --wait-time int              wait time before opening the browser (microseconds) (default 500)
  -h, --help                       help for run
  -p, --port int                   server port (default 9999)
  -v, --view                       open browser
`, defaultCommand)
	})
	rootCmd.AddCommand(runCmd)
}
