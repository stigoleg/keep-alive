// Package ipc keeps keepalive to one instance per user and lets other
// invocations control it over a local socket.
//
// # Endpoint
//
// The running instance holds an exclusive lock on keepalive.lock (its pid is
// written inside) and listens on the AF_UNIX stream socket keepalive.sock,
// both in one directory:
//
//   - $KEEPALIVE_RUNTIME_DIR when set;
//   - unix: $XDG_RUNTIME_DIR/keepalive, else <user cache dir>/keepalive, else
//     /tmp/keepalive-<uid> when that path would exceed the socket path limit
//     (104 bytes on macOS, 108 on Linux);
//   - windows: %LOCALAPPDATA%\keepalive.
//
// The directory is created with mode 0700 and must be a real directory owned
// by the user; the socket is 0600. An existing directory that other users
// may open ($KEEPALIVE_RUNTIME_DIR=$HOME, /tmp) is left as it is, and a
// private keepalive-<uid> directory inside it is used instead.
//
// # Protocol
//
// Newline-delimited JSON, one request per connection except subscribe. Every
// message carries "v":1.
//
//	{"v":1,"cmd":"status"}                 -> {"v":1,"ok":true,"pid":123,"version":"2.0.0","origin":"service","snapshot":{…}}
//	{"v":1,"cmd":"stop"}                   -> {"v":1,"ok":true}
//	{"v":1,"cmd":"set_active","active":true}
//	{"v":1,"cmd":"extend","seconds":900}   (negative shortens)
//	{"v":1,"cmd":"subscribe"}              -> {"v":1,"ok":true}, then one event object per line
//	any failure                            -> {"v":1,"ok":false,"error":"…"}
//
// The snapshot is the "snapshot" object of the --json printer, and each
// streamed event is the --json printer's event object with "v":1 in front.
// The stream ends when the session stops, the server shuts down or the client
// closes. Requests are limited to 64 KiB and must arrive within 5 seconds.
package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

const (
	// ProtocolVersion is the "v" every message carries.
	ProtocolVersion = 1
	// MaxRequestSize bounds one request line.
	MaxRequestSize = 64 << 10

	SocketName = "keepalive.sock"
	LockName   = "keepalive.lock"

	// EnvRuntimeDir overrides the socket and lock directory.
	EnvRuntimeDir = "KEEPALIVE_RUNTIME_DIR"
	// EnvOrigin is set to "service" by the login service definitions.
	EnvOrigin = "KEEPALIVE_ORIGIN"

	OriginService  = "service"
	OriginTerminal = "terminal"
)

// requestTimeout bounds reading a request on the server and a whole request
// on the client; a variable so tests can shorten it.
var requestTimeout = 5 * time.Second

// writeTimeout bounds writing one reply or event.
const writeTimeout = 5 * time.Second

// Command names on the wire.
const (
	cmdStatus    = "status"
	cmdStop      = "stop"
	cmdSetActive = "set_active"
	cmdExtend    = "extend"
	cmdSubscribe = "subscribe"
)

// ErrNotRunning means no instance is listening.
var ErrNotRunning = errors.New("no keepalive instance is running")

// ErrAlreadyRunning means another instance holds the lock; the error returned
// by Listen is an *AlreadyRunningError that matches it.
var ErrAlreadyRunning = errors.New("keepalive is already running")

// AlreadyRunningError carries the pid found in the lock file (0 when it could
// not be read).
type AlreadyRunningError struct{ PID int }

func (e *AlreadyRunningError) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("%v (pid %d)", ErrAlreadyRunning, e.PID)
	}
	return ErrAlreadyRunning.Error()
}

func (e *AlreadyRunningError) Is(target error) bool { return target == ErrAlreadyRunning }

// RemoteError is a failure reported by the server ("ok":false).
type RemoteError struct{ Message string }

func (e *RemoteError) Error() string { return "keepalive instance: " + e.Message }

// Controller is what the server drives; *session.Session implements it.
type Controller interface {
	Snapshot() session.Snapshot
	Stop(session.Reason)
	SetActive(bool)
	Extend(time.Duration)
	Subscribe() (<-chan session.Event, func())
}

var _ Controller = (*session.Session)(nil)

// ServerInfo is reported by status. A zero PID means os.Getpid(); an empty
// Origin means OriginTerminal.
type ServerInfo struct {
	PID     int
	Version string
	Origin  string
}

// OriginFromEnv returns OriginService when the login service started this
// process (KEEPALIVE_ORIGIN=service), else OriginTerminal.
func OriginFromEnv() string {
	if os.Getenv(EnvOrigin) == OriginService {
		return OriginService
	}
	return OriginTerminal
}

// Status is the reply to status.
type Status struct {
	PID      int
	Version  string
	Origin   string
	Snapshot output.JSONSnapshot
	Raw      json.RawMessage // the reply line as received, for --json output
}

// Event is one streamed session event: the --json printer's object, decoded,
// plus the raw line (which also carries "v").
type Event struct {
	output.JSONEvent
	Raw json.RawMessage `json:"-"`
}

type request struct {
	V       int    `json:"v"`
	Cmd     string `json:"cmd"`
	Active  *bool  `json:"active,omitempty"`
	Seconds *int64 `json:"seconds,omitempty"`
}

type response struct {
	V        int                  `json:"v"`
	OK       bool                 `json:"ok"`
	Error    string               `json:"error,omitempty"`
	PID      int                  `json:"pid,omitempty"`
	Version  string               `json:"version,omitempty"`
	Origin   string               `json:"origin,omitempty"`
	Snapshot *output.JSONSnapshot `json:"snapshot,omitempty"`
}

type wireEvent struct {
	V int `json:"v"`
	output.JSONEvent
}

func failure(format string, a ...any) response {
	return response{V: ProtocolVersion, Error: fmt.Sprintf(format, a...)}
}

func success() response { return response{V: ProtocolVersion, OK: true} }

// maxExtendSeconds keeps seconds*time.Second inside time.Duration.
const maxExtendSeconds = int64(1<<63-1) / int64(time.Second)

func decodeRequest(line []byte) (request, error) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return req, fmt.Errorf("malformed request: %v", err)
	}
	if req.V != ProtocolVersion {
		return req, fmt.Errorf("unsupported protocol version %d (this instance speaks %d)", req.V, ProtocolVersion)
	}
	switch req.Cmd {
	case cmdStatus, cmdStop, cmdSubscribe:
	case cmdSetActive:
		if req.Active == nil {
			return req, fmt.Errorf(`%s needs "active": true or false`, cmdSetActive)
		}
	case cmdExtend:
		if req.Seconds == nil {
			return req, fmt.Errorf(`%s needs "seconds"`, cmdExtend)
		}
		if s := *req.Seconds; s > maxExtendSeconds || s < -maxExtendSeconds {
			return req, fmt.Errorf("seconds %d out of range", s)
		}
	default:
		return req, fmt.Errorf("unknown command %q", req.Cmd)
	}
	return req, nil
}
