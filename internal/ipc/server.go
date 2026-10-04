package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// Server is the listening side. Listen takes the single-instance lock, Serve
// answers requests, Close gives everything back.
type Server struct {
	info     ServerInfo
	sockPath string
	lock     *os.File
	ln       net.Listener

	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	closing   bool          // shutdown started; no new connections are tracked
	serving   bool          // Serve was called
	released  bool          // Close finished
	done      chan struct{} // closed by shutdown
	serveDone chan struct{} // closed when Serve returns
	handlers  sync.WaitGroup
	closeErr  error
}

// Listen takes the lock and starts listening. Another live instance yields an
// error matching ErrAlreadyRunning; a socket left behind by a dead instance is
// removed.
func Listen(info ServerInfo) (*Server, error) {
	if info.PID == 0 {
		info.PID = os.Getpid()
	}
	if info.Origin == "" {
		info.Origin = OriginTerminal
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	if err := ensureDir(dir); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, LockName)
	lock, err := acquireLock(lockPath)
	if errors.Is(err, errLocked) {
		return nil, &AlreadyRunningError{PID: readPID(lockPath)}
	}
	if err != nil {
		return nil, fmt.Errorf("ipc: lock %s: %w", lockPath, err)
	}
	if err := writePID(lock, info.PID); err != nil {
		releaseLock(lock)
		return nil, fmt.Errorf("ipc: lock %s: %w", lockPath, err)
	}
	sock := filepath.Join(dir, SocketName)
	// Holding the lock means any socket file is stale.
	if err := os.Remove(sock); err != nil && !errors.Is(err, fs.ErrNotExist) {
		releaseLock(lock)
		return nil, fmt.Errorf("ipc: remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		releaseLock(lock)
		return nil, fmt.Errorf("ipc: %w", err)
	}
	if err := restrictSocket(sock); err != nil {
		ln.Close()
		releaseLock(lock)
		return nil, fmt.Errorf("ipc: %w", err)
	}
	return &Server{
		info:      info,
		sockPath:  sock,
		lock:      lock,
		ln:        ln,
		conns:     map[net.Conn]struct{}{},
		done:      make(chan struct{}),
		serveDone: make(chan struct{}),
	}, nil
}

func writePID(f *os.File, pid int) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt([]byte(strconv.Itoa(pid)+"\n"), 0)
	return err
}

// readPID reads the holder's pid, waiting briefly for a holder that has just
// taken the lock and not written its pid yet.
func readPID(path string) int {
	for range 10 {
		b, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

// Serve answers requests until ctx is cancelled or Close is called; it
// returns nil then, after every connection has finished.
func (s *Server) Serve(ctx context.Context, c Controller) error {
	s.mu.Lock()
	if s.serving {
		s.mu.Unlock()
		return errors.New("ipc: Serve called twice")
	}
	s.serving = true
	s.mu.Unlock()
	defer close(s.serveDone)
	defer context.AfterFunc(ctx, s.shutdown)()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			stopping := s.isClosing()
			s.shutdown()
			s.handlers.Wait()
			if stopping {
				return nil
			}
			return fmt.Errorf("ipc: accept: %w", err)
		}
		if !s.track(conn) {
			conn.Close()
			continue
		}
		go func() {
			defer s.handlers.Done()
			defer s.untrack(conn)
			s.handle(conn, c)
		}()
	}
}

// Close stops serving, closes every connection (ending subscriptions),
// removes the socket and releases the lock. It is safe to call more than once.
func (s *Server) Close() error {
	s.shutdown()
	s.mu.Lock()
	serving := s.serving
	s.mu.Unlock()
	if serving {
		<-s.serveDone
	}
	s.handlers.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.released {
		return s.closeErr
	}
	s.released = true
	var errs []error
	if err := os.Remove(s.sockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	if err := releaseLock(s.lock); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		s.closeErr = fmt.Errorf("ipc: close: %w", err)
	}
	return s.closeErr
}

func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return
	}
	s.closing = true
	close(s.done)
	s.ln.Close()
	for conn := range s.conns {
		conn.Close()
	}
}

func (s *Server) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

func (s *Server) track(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.conns[conn] = struct{}{}
	s.handlers.Add(1)
	return true
}

func (s *Server) untrack(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

var (
	errEmpty    = errors.New("connection closed without a request")
	errTooLarge = errors.New("too large")
)

// readLine reads one newline-terminated line of at most limit bytes. A final
// line without a newline is accepted when the peer closes after it.
func readLine(br *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > limit+1 || (len(line) == limit+1 && line[limit] != '\n') {
			return nil, errTooLarge
		}
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, nil
		case errors.Is(err, io.EOF):
			return nil, errEmpty
		default:
			return nil, err
		}
	}
}

func (s *Server) handle(conn net.Conn, c Controller) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(requestTimeout))
	br := bufio.NewReader(conn)
	line, err := readLine(br, MaxRequestSize)
	switch {
	case err == nil:
	case errors.Is(err, errEmpty):
		return // a probe from Dial
	case errors.Is(err, errTooLarge):
		s.reply(conn, failure("request too large (max %d bytes)", MaxRequestSize))
		return
	case isTimeout(err):
		s.reply(conn, failure("request timed out after %s", requestTimeout))
		return
	default:
		slog.Debug("ipc: read request", "err", err)
		return
	}
	req, err := decodeRequest(line)
	if err != nil {
		s.reply(conn, failure("%v", err))
		return
	}
	switch req.Cmd {
	case cmdStatus:
		snap := output.NewJSONSnapshot(c.Snapshot())
		resp := success()
		resp.PID, resp.Version, resp.Origin, resp.Snapshot = s.info.PID, s.info.Version, s.info.Origin, &snap
		s.reply(conn, resp)
	case cmdStop:
		c.Stop(session.ReasonIPC)
		s.reply(conn, success())
	case cmdSetActive:
		c.SetActive(*req.Active)
		s.reply(conn, success())
	case cmdExtend:
		c.Extend(time.Duration(*req.Seconds) * time.Second)
		s.reply(conn, success())
	case cmdSubscribe:
		s.stream(conn, c)
	}
}

func (s *Server) reply(conn net.Conn, resp response) error {
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return encoder(conn).Encode(resp)
}

func encoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc
}

// stream forwards session events until the session ends, the client goes
// away or the server shuts down.
func (s *Server) stream(conn net.Conn, c Controller) {
	events, cancel := c.Subscribe()
	defer cancel()
	conn.SetReadDeadline(time.Time{})
	if err := s.reply(conn, success()); err != nil {
		return
	}
	gone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, conn) // returns when the client closes or conn is closed
		close(gone)
	}()
	defer func() {
		conn.Close()
		<-gone
	}()
	enc := encoder(conn)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := enc.Encode(wireEvent{V: ProtocolVersion, JSONEvent: output.NewJSONEvent(ev)}); err != nil {
				return
			}
		case <-gone:
			return
		case <-s.done:
			return
		}
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
