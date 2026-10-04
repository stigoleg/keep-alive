package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"time"
)

// maxReplySize bounds one reply or event line read by the client.
const maxReplySize = 1 << 20

// Client talks to the running instance; each call opens its own connection.
type Client struct{ path string }

// Dial checks that an instance is listening. It returns ErrNotRunning when
// there is no socket or nothing accepts on it.
func Dial(ctx context.Context) (*Client, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotRunning
		}
		return nil, fmt.Errorf("ipc: %w", err)
	}
	c := &Client{path: path}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	conn.Close()
	return c, nil
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.path)
	if err != nil {
		if notRunning(err) {
			return nil, ErrNotRunning
		}
		return nil, fmt.Errorf("ipc: connect: %w", err)
	}
	return conn, nil
}

// Status returns the instance's pid, version, origin and session snapshot.
func (c *Client) Status(ctx context.Context) (Status, error) {
	resp, raw, err := c.roundTrip(ctx, request{Cmd: cmdStatus})
	if err != nil {
		return Status{}, err
	}
	if resp.Snapshot == nil {
		return Status{}, errors.New("ipc: status reply without a snapshot")
	}
	return Status{PID: resp.PID, Version: resp.Version, Origin: resp.Origin, Snapshot: *resp.Snapshot, Raw: raw}, nil
}

// Stop ends the session (reason ipc).
func (c *Client) Stop(ctx context.Context) error {
	_, _, err := c.roundTrip(ctx, request{Cmd: cmdStop})
	return err
}

// SetActive switches activity simulation on or off.
func (c *Client) SetActive(ctx context.Context, on bool) error {
	_, _, err := c.roundTrip(ctx, request{Cmd: cmdSetActive, Active: &on})
	return err
}

// Extend moves the session's end by d (whole seconds; negative shortens).
func (c *Client) Extend(ctx context.Context, d time.Duration) error {
	secs := int64(d / time.Second)
	_, _, err := c.roundTrip(ctx, request{Cmd: cmdExtend, Seconds: &secs})
	return err
}

// Subscribe streams session events. The channel closes when the session
// ends, the server shuts down, the connection fails or ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context) (<-chan Event, error) {
	conn, br, err := c.request(ctx, request{Cmd: cmdSubscribe})
	if err != nil {
		return nil, err
	}
	if _, _, err := readReply(ctx, conn, br); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	ch := make(chan Event, 16)
	go func() {
		defer close(ch)
		defer conn.Close()
		defer context.AfterFunc(ctx, func() { conn.Close() })()
		for {
			line, err := readLine(br, maxReplySize)
			if err != nil {
				return
			}
			var ev Event
			if err := json.Unmarshal(line, &ev); err != nil {
				slog.Debug("ipc: malformed event", "err", err)
				return
			}
			ev.Raw = json.RawMessage(bytes.TrimSpace(line))
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (c *Client) roundTrip(ctx context.Context, req request) (response, json.RawMessage, error) {
	conn, br, err := c.request(ctx, req)
	if err != nil {
		return response{}, nil, err
	}
	defer conn.Close()
	return readReply(ctx, conn, br)
}

// request connects and sends req; the connection's deadline is the earlier
// of ctx's and requestTimeout, and cancelling ctx unblocks it.
func (c *Client) request(ctx context.Context, req request) (net.Conn, *bufio.Reader, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(requestTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	req.V = ProtocolVersion
	if err := encoder(conn).Encode(req); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("ipc: send %s: %w", req.Cmd, err)
	}
	return conn, bufio.NewReader(conn), nil
}

func readReply(ctx context.Context, conn net.Conn, br *bufio.Reader) (response, json.RawMessage, error) {
	defer context.AfterFunc(ctx, func() { conn.SetDeadline(time.Unix(1, 0)) })()
	line, err := readLine(br, maxReplySize)
	if err != nil {
		if ctx.Err() != nil {
			return response{}, nil, ctx.Err()
		}
		if errors.Is(err, errEmpty) {
			return response{}, nil, errors.New("ipc: connection closed without a reply")
		}
		return response{}, nil, fmt.Errorf("ipc: read reply: %w", err)
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return response{}, nil, fmt.Errorf("ipc: malformed reply: %w", err)
	}
	if resp.V != ProtocolVersion {
		return response{}, nil, fmt.Errorf("ipc: reply has protocol version %d, want %d", resp.V, ProtocolVersion)
	}
	if !resp.OK {
		return response{}, nil, &RemoteError{Message: resp.Error}
	}
	return resp, json.RawMessage(bytes.TrimSpace(line)), nil
}
