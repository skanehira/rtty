package cmd

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
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

	"github.com/creack/pty"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

//go:embed public/index.html
var indexHTML string

//go:embed public/index.js
var indexJS string

//go:embed public
var public embed.FS

// run command
var command string = getenv("SHELL", "bash")

type Event string

const (
	EventResize  Event = "resize"
	EventSnedkey Event = "sendKey"
	EventClose   Event = "close"
)

type Message struct {
	Event Event
	Data  any
}

var sharedSession = newSession()

type session struct {
	mu            sync.Mutex
	ptmx          *os.File
	cmd           *exec.Cmd
	clients       map[*websocket.Conn]*wsConn
	readerRunning bool
	history       []string
	historyBuf    []byte
	historyLimit  int
}

func newSession() *session {
	return &session{
		clients:      make(map[*websocket.Conn]*wsConn),
		historyLimit: 3000,
	}
}

func (s *session) needsStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptmx == nil
}

func (s *session) start(winsize *pty.Winsize) error {
	s.mu.Lock()
	if s.ptmx != nil {
		s.mu.Unlock()
		return nil
	}

	c := filter(strings.Split(command, " "))
	if len(c) > 1 {
		//nolint
		s.cmd = exec.Command(c[0], c[1:]...)
	} else {
		//nolint
		s.cmd = exec.Command(c[0])
	}

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
		_ = client.conn.Close()
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
	_ = conn.Close()
}

func (s *session) broadcast(data []byte) {
	s.mu.Lock()
	clients := collectClients(s.clients)
	s.mu.Unlock()

	for _, client := range clients {
		if err := writeToClient(client, data); err != nil {
			s.removeClient(client.conn)
		}
	}
}

func (s *session) appendHistory(data []byte) {
	if s.historyLimit <= 0 {
		return
	}

	s.mu.Lock()
	s.historyBuf = append(s.historyBuf, data...)
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
	s.mu.Unlock()
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
	_ = client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := client.Write(data)
	return err
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
	case EventSnedkey:
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

func run(ws *websocket.Conn) {
	defer func() {
		if err := ws.Close(); err != nil {
			log.Println("failed to close websocket:", err)
		}
	}()

	wsconn := &wsConn{
		conn: ws,
	}

	var first Message
	if err := json.NewDecoder(ws).Decode(&first); err != nil {
		log.Println("failed to decode message:", err)
		return
	}

	if sharedSession.needsStart() {
		if first.Event != EventResize {
			log.Println("first message must be resize")
			return
		}
		rows, cols, err := windowSize(first.Data)
		if err != nil {
			_, _ = fmt.Fprintf(ws, "%s\r\n", err)
			return
		}
		winsize := &pty.Winsize{Rows: rows, Cols: cols}
		if err := sharedSession.start(winsize); err != nil {
			log.Println(err)
			return
		}
	}

	sharedSession.addClient(ws, wsconn)
	defer sharedSession.removeClient(ws)
	if err := sharedSession.sendHistory(wsconn); err != nil {
		sharedSession.removeClient(ws)
		return
	}

	if err := sharedSession.handleMessage(first); err != nil {
		if err != io.EOF {
			log.Println(err)
		}
		return
	}

	decoder := json.NewDecoder(ws)
	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			return
		}
		if err := sharedSession.handleMessage(msg); err != nil {
			if err != io.EOF {
				log.Println(err)
			}
			return
		}
	}
}

type wsConn struct {
	conn *websocket.Conn
}

func (ws *wsConn) Write(b []byte) (i int, err error) {
	return ws.conn.Write(b)
}

func checkOrigin(allowOrigins []string) func(config *websocket.Config, req *http.Request) error {
	return func(config *websocket.Config, req *http.Request) (err error) {
		config.Origin, err = websocket.Origin(config, req)
		if err == nil && config.Origin == nil {
			return fmt.Errorf("null origin")
		}
		if err != nil {
			return err
		}

		if slices.Contains(allowOrigins, config.Origin.Host) {
			return nil
		}

		msg := "not allowed origin: %s"
		log.Printf(msg, config.Origin.Host)
		return fmt.Errorf(msg, config.Origin.Host)
	}
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run command",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			command = args[0]
		}
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
		mux.Handle("/ws", websocket.Server{Handler: run, Handshake: checkOrigin(allowedOrigins)})

		server := &http.Server{
			Addr:    addr + ":" + port,
			Handler: mux,
		}

		go func() {
			log.Println("allowed origins", allowedOrigins)
			log.Println("running command: " + command)
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
		<-quit

		sharedSession.shutdown()
		if err := server.Shutdown(context.Background()); err != nil {
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
`, command)
	})
	rootCmd.AddCommand(runCmd)
}
