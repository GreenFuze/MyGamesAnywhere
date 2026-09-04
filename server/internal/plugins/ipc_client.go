package plugins

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type Request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// Response carries either a call's outcome or, when Progress is set, an
// interim report for a call that is still running.
type Response struct {
	ID       string          `json:"id"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    *Error          `json:"error,omitempty"`
	Progress *Progress       `json:"progress,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type IpcClient interface {
	Call(ctx context.Context, method string, params any, result any) error
	Close() error
}

// DisconnectFunc is invoked when the plugin stdout reader ends. unexpected is true if Close() was not called (e.g. crash).
type DisconnectFunc func(pluginID string, readErr error, unexpected bool)

type jsonIpcClient struct {
	process        Process
	mu             sync.Mutex
	pending        map[string]chan *Response
	progress       map[string]ProgressFunc
	logger         core.Logger
	pluginID       string
	onDisconnect   DisconnectFunc
	intentionalEnd atomic.Bool
}

func NewIpcClient(process Process, logger core.Logger, pluginID string, onDisconnect DisconnectFunc) IpcClient {
	c := &jsonIpcClient{
		process:      process,
		pending:      make(map[string]chan *Response),
		progress:     make(map[string]ProgressFunc),
		logger:       logger,
		pluginID:     pluginID,
		onDisconnect: onDisconnect,
	}
	go c.listenStdout()
	go c.listenStderr()
	return c
}

func (c *jsonIpcClient) listenStdout() {
	stdout := c.process.Stdout()
	for {
		var length uint32
		err := binary.Read(stdout, binary.BigEndian, &length)
		if err != nil {
			if err != io.EOF {
				c.logger.Error("failed to read from plugin stdout", err, "plugin_id", c.pluginID)
			}
			c.notifyStdoutEnd(err)
			return
		}

		payload := make([]byte, length)
		_, err = io.ReadFull(stdout, payload)
		if err != nil {
			c.logger.Error("failed to read payload from plugin stdout", err, "plugin_id", c.pluginID)
			c.notifyStdoutEnd(err)
			return
		}

		var resp Response
		if err := json.Unmarshal(payload, &resp); err != nil {
			c.logger.Error("failed to unmarshal plugin response", err, "plugin_id", c.pluginID)
			continue
		}

		// A progress frame reports on a call that is still running, so it must
		// not complete it. Unknown ids stay ignored, which is what lets an old
		// host and a new plugin keep working together.
		if resp.Progress != nil {
			c.mu.Lock()
			handler := c.progress[resp.ID]
			c.mu.Unlock()
			if handler != nil {
				handler(*resp.Progress)
			}
			continue
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			ch <- &resp
			delete(c.pending, resp.ID)
		}
		delete(c.progress, resp.ID)
		c.mu.Unlock()
	}
}

func (c *jsonIpcClient) listenStderr() {
	scanner := bufio.NewScanner(c.process.Stderr())
	for scanner.Scan() {
		c.logger.Info(scanner.Text(), "plugin_id", c.pluginID, "source", "stderr")
	}
	if err := scanner.Err(); err != nil {
		c.logger.Error("failed to read from plugin stderr", err, "plugin_id", c.pluginID)
	}
}

func (c *jsonIpcClient) Call(ctx context.Context, method string, params any, result any) error {
	id := uuid.New().String()

	req := Request{
		ID:     id,
		Method: method,
		Params: params,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	c.mu.Lock()
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	if handler := ProgressFromContext(ctx); handler != nil {
		c.progress[id] = handler
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.progress, id)
		c.mu.Unlock()
	}()

	// Write length-prefixed payload
	c.mu.Lock()
	err = binary.Write(c.process.Stdin(), binary.BigEndian, uint32(len(payload)))
	if err == nil {
		_, err = c.process.Stdin().Write(payload)
	}
	c.mu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("plugin error [%s]: %s", resp.Error.Code, resp.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	}
}

func (c *jsonIpcClient) notifyStdoutEnd(readErr error) {
	if c.onDisconnect == nil {
		return
	}
	unexpected := !c.intentionalEnd.Load()
	c.onDisconnect(c.pluginID, readErr, unexpected)
}

func (c *jsonIpcClient) Close() error {
	c.intentionalEnd.Store(true)
	return c.process.Kill()
}
